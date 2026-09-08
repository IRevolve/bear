package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/irevolve/bear/internal"
	"github.com/irevolve/bear/internal/config"
)

func writeTreeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestTreeRejectsCycles(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "bear.config.yml")
	writeTreeFixture(t, path, "name: project\n")
	writeTreeFixture(t, filepath.Join(root, "a", "bear.lib.yml"), "name: a\ndepends: [b]\n")
	writeTreeFixture(t, filepath.Join(root, "b", "bear.lib.yml"), "name: b\ndepends: [a]\n")
	for _, filter := range [][]string{nil, {"a"}} {
		if err := Tree(path, filter); err == nil || !strings.Contains(err.Error(), "a -> b -> a") {
			t.Fatalf("expected cycle error, got %v", err)
		}
	}
}

func TestListAndTreeSurfaceLockErrors(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "bear.config.yml")
	writeTreeFixture(t, path, "name: project\n")
	writeTreeFixture(t, filepath.Join(root, "bear.lock.yml"), "environments: [broken")
	for _, run := range []func() error{func() error { return Tree(path, nil) }, func() error { return List(path) }} {
		if err := run(); err == nil || !strings.Contains(err.Error(), "loading lock") {
			t.Fatalf("lock error was hidden: %v", err)
		}
	}
}

func TestTreeEnvironmentStatus(t *testing.T) {
	p := NewPrinterWithWriter(&bytes.Buffer{})
	a := internal.DiscoveredArtifact{Artifact: &config.Artifact{Name: "api"}}
	lock := &config.LockFile{Environments: map[string]map[string]config.LockEntry{
		"prd": {"api": {Version: "v1", Pinned: true}},
		"dev": {"api": {Version: "v2"}},
		"int": {"other": {Version: "v3"}},
	}}
	if status := getStatus(p, a, lock); status != " [dev: v2; prd: v1 (pinned)]" {
		t.Fatalf("status = %q", status)
	}
}

func TestFindCyclesRestoresTraversalState(t *testing.T) {
	var artifacts []internal.DiscoveredArtifact
	for _, a := range []config.Artifact{
		{Name: "a", Depends: []string{"b", "c"}},
		{Name: "b", Depends: []string{"a"}},
		{Name: "c", Depends: []string{"d"}},
		{Name: "d", Depends: []string{"c"}},
		{Name: "e", Depends: []string{"b"}},
	} {
		artifacts = append(artifacts, internal.DiscoveredArtifact{Artifact: &a})
	}
	cycles := findCycles(artifacts)
	if len(cycles) != 2 || strings.Join(cycles[0], ",") != "a,b,a" || strings.Join(cycles[1], ",") != "c,d,c" {
		t.Fatalf("cycles = %v", cycles)
	}
}

func TestCheckRejectsEmptyDeployment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bear.config.yml")
	writeTreeFixture(t, path, "name: project\ntargets: {local: {steps: []}}\n")
	if err := Check(path); err == nil {
		t.Fatal("check accepted empty deployment target")
	}
}
