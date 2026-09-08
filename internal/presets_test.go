package internal

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Fixtures match bear-presets at DefaultPresetsRevision.
const upstreamGo = `name: go
detection:
  files: [go.mod]
validation:
  setup:
    - name: Download modules
      run: go mod download
  lint:
    - name: Vet
      run: go vet ./...
  test:
    - name: Test
      run: go test -race ./...
  build:
    - name: Build
      run: go build -o dist/app .
`

const upstreamDocker = `name: docker
defaults:
  REGISTRY: docker.io
deploy:
  - name: Build image
    run: docker build -t $REGISTRY/$NAME:$VERSION .
  - name: Push image
    run: docker push $REGISTRY/$NAME:$VERSION
`

func TestPresetUpstreamSchemas(t *testing.T) {
	language, err := parseLanguage([]byte(upstreamGo), "go")
	if err != nil {
		t.Fatal(err)
	}
	if len(language.Steps) != 4 || language.Steps[0].Name != "Download modules" || language.Steps[1].Name != "Vet" || language.Steps[2].Name != "Test" || language.Steps[3].Name != "Build" {
		t.Fatalf("incorrect validation order: %+v", language.Steps)
	}
	target, err := parseTarget([]byte(upstreamDocker), "docker")
	if err != nil || len(target.Steps) != 2 || target.Vars["REGISTRY"] != "docker.io" {
		t.Fatalf("target = %+v, error = %v", target, err)
	}
	if _, err := parseLanguage([]byte("steps: []\n"), "go"); err != nil {
		t.Fatalf("explicit empty local language steps: %v", err)
	}
	if target, err := parseTarget([]byte("vars: {REGISTRY: local}\nsteps: [{name: deploy, run: 'true'}]\n"), "docker"); err != nil || target.Vars["REGISTRY"] != "local" {
		t.Fatalf("local target = %+v, error = %v", target, err)
	}
}

func TestPresetStrictSchemas(t *testing.T) {
	for _, data := range []string{
		upstreamGo + "steps: []\n", upstreamGo + "vars: {}\n", upstreamGo + "typo: true\n",
		strings.Replace(upstreamGo, "name: go", "name: node", 1),
		"validation: {unknown: []}", "validation: {}", "steps: null", "detection: {}",
		"steps: [{name: '', run: test}]", "steps: [{name: test, run: ' '}]",
		"steps: []\n---\nsteps: []", "steps: []\nsteps: []", "name: go\nsteps: []\nvars: null",
		"<<: {validation: {test: [{name: test, run: test}]}}\nsteps: []",
	} {
		if _, err := parseLanguage([]byte(data), "go"); err == nil {
			t.Errorf("accepted invalid language: %s", data)
		}
	}
	for _, data := range []string{
		upstreamDocker + "steps: []\n", upstreamDocker + "vars: {}\n", upstreamDocker + "typo: true\n",
		strings.Replace(upstreamDocker, "name: docker", "name: other", 1),
		"deploy: []", "steps: []", "defaults: {}", "steps: [{name: deploy}]",
	} {
		if _, err := parseTarget([]byte(data), "docker"); err == nil {
			t.Errorf("accepted invalid target: %s", data)
		}
	}
}

func TestPresetCacheWriteFailurePreservesGoodFiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.yml":
			fmt.Fprint(w, "version: 1\nlanguages: [go]\ntargets: [docker]")
		case "/languages/go.yml":
			fmt.Fprint(w, upstreamGo)
		case "/targets/docker.yml":
			fmt.Fprint(w, upstreamDocker)
		}
	}))
	defer server.Close()
	m := &Manager{repoURL: server.URL, cacheDir: t.TempDir(), revision: DefaultPresetsRevision}
	path := filepath.Join(m.cacheDir, m.revision, "languages/go.yml")
	good := []byte("steps: [{name: Test, run: go test ./...}]\n")
	if err := m.writeCache(path, good); err != nil {
		t.Fatal(err)
	}
	// A regular file in place of a directory makes publication fail even as root.
	if err := os.WriteFile(filepath.Join(m.cacheDir, m.revision, "targets"), []byte("blocked"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.Update(); err == nil {
		t.Fatal("expected cache publication error")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != string(good) {
		t.Fatalf("good cache changed on failed update: %q, %v", data, err)
	}
}

func TestPresetCacheFailureAndOffline(t *testing.T) {
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.yml":
			fmt.Fprint(w, "version: 1\nlanguages: [go]\ntargets: [docker]\n")
		case "/languages/go.yml":
			fmt.Fprint(w, upstreamGo)
		case "/targets/docker.yml":
			if fail.Load() {
				fmt.Fprint(w, "deploy: []")
			} else {
				fmt.Fprint(w, upstreamDocker)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	m := &Manager{repoURL: server.URL, cacheDir: t.TempDir(), revision: DefaultPresetsRevision}
	if err := m.Update(); err != nil {
		t.Fatal(err)
	}
	fail.Store(true)
	if err := m.Update(); err == nil {
		t.Fatal("invalid update succeeded")
	}
	path := filepath.Join(m.cacheDir, m.revision, "targets/docker.yml")
	data, err := os.ReadFile(path)
	if err != nil || string(data) != upstreamDocker {
		t.Fatalf("good cache was replaced: %s, %v", data, err)
	}
	old := time.Now().Add(-365 * 24 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	server.Close()
	if err := m.Update(); err == nil {
		t.Fatal("offline update succeeded")
	}
	if _, err := m.GetTarget("docker"); err != nil {
		t.Fatalf("offline immutable cache: %v", err)
	}
	other := *m
	other.revision = strings.Repeat("a", 40)
	if _, err := other.GetTarget("docker"); err == nil {
		t.Fatal("different revision reused stale cache")
	}
}

func TestPresetCacheValidationAndConcurrency(t *testing.T) {
	var bad atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if bad.Load() {
			fmt.Fprint(w, "validation: {typo: []}")
		} else {
			fmt.Fprint(w, upstreamGo)
		}
	}))
	defer server.Close()
	m := &Manager{repoURL: server.URL, cacheDir: t.TempDir(), revision: DefaultPresetsRevision}
	bad.Store(true)
	if _, err := m.GetLanguage("go"); err == nil {
		t.Fatal("invalid download accepted")
	}
	path := filepath.Join(m.cacheDir, m.revision, "languages/go.yml")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("invalid download published: %v", err)
	}
	bad.Store(false)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := m.GetLanguage("go"); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if err := os.WriteFile(path, []byte("steps: [{name: broken}]"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.GetLanguage("go"); err != nil {
		t.Fatalf("did not repair corrupt cache: %v", err)
	}
	server.Close()
	if err := os.WriteFile(path, []byte("steps: [{name: broken}]"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.GetLanguage("go"); err == nil {
		t.Fatal("invalid offline cache accepted")
	}
}

func TestPresetTraversal(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.NotFound(w, r)
	}))
	defer server.Close()
	m := &Manager{repoURL: server.URL, cacheDir: t.TempDir(), revision: DefaultPresetsRevision}
	for _, name := range []string{"", "../go", "/go", `..\go`, "go/../../outside", "go?raw=1", "go#fragment", "%2e%2e"} {
		if _, err := m.GetLanguage(name); err == nil {
			t.Errorf("accepted language %q", name)
		}
		if _, err := m.GetTarget(name); err == nil {
			t.Errorf("accepted target %q", name)
		}
	}
	for _, revision := range []string{"main", "../outside", "abc"} {
		m.revision = revision
		if _, err := m.GetIndex(); err == nil {
			t.Errorf("accepted revision %q", revision)
		}
	}
	for _, path := range []string{"../index.yml", "/index.yml", "languages/../../escape.yml", "unknown/go.yml"} {
		if _, err := m.fetchFile(path); err == nil {
			t.Errorf("accepted path %q", path)
		}
	}
	if requests.Load() != 0 {
		t.Fatal("unsafe names reached network")
	}
	if _, err := parseIndex([]byte("version: 1\nlanguages: [../escape]")); err == nil {
		t.Fatal("unsafe index accepted")
	}
}
