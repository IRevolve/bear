package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "bear-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write test config
	configContent := `name: test-project

environments: [dev, int, prd]

languages:
  go:
    detection:
      files: [go.mod]
    steps:
      - name: Download
        run: go mod download
      - name: Vet
        run: go vet ./...
      - name: Test
        run: go test ./...
      - name: Build
        run: go build .

targets:
  docker:
    vars:
      REGISTRY: ghcr.io
    steps:
      - name: Build
        run: docker build .
`
	configPath := filepath.Join(tmpDir, "bear.config.yml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Test Load
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify
	if cfg.Name != "test-project" {
		t.Errorf("expected name 'test-project', got '%s'", cfg.Name)
	}
	if !reflect.DeepEqual(cfg.Environments, []string{"dev", "int", "prd"}) {
		t.Errorf("expected environments [dev int prd], got %v", cfg.Environments)
	}
	if len(cfg.Languages) != 1 {
		t.Errorf("expected 1 language, got %d", len(cfg.Languages))
	}
	goLang, ok := cfg.Languages["go"]
	if !ok {
		t.Fatal("expected language 'go' to exist")
	}
	if goLang.Name != "go" {
		t.Errorf("expected language name 'go', got '%s'", goLang.Name)
	}
	if len(goLang.Detection.Files) != 1 {
		t.Errorf("expected 1 detection file, got %d", len(goLang.Detection.Files))
	}
	if len(goLang.Steps) != 4 {
		t.Errorf("expected 4 steps, got %d", len(goLang.Steps))
	}
	if len(cfg.Targets) != 1 {
		t.Errorf("expected 1 target, got %d", len(cfg.Targets))
	}
	dockerTarget, ok := cfg.Targets["docker"]
	if !ok {
		t.Fatal("expected target 'docker' to exist")
	}
	if dockerTarget.Vars["REGISTRY"] != "ghcr.io" {
		t.Errorf("expected REGISTRY 'ghcr.io', got '%s'", dockerTarget.Vars["REGISTRY"])
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/path/bear.config.yml")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "bear-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write invalid YAML
	configPath := filepath.Join(tmpDir, "bear.config.yml")
	if err := os.WriteFile(configPath, []byte("invalid: [broken"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err = Load(configPath)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

// environments is required: without it nothing can be planned, applied or keyed
// in the lock file, so the failure belongs at load time with a usable message.
func TestLoadEnvironments(t *testing.T) {
	for _, tt := range []struct {
		name      string
		data      string
		want      []string
		wantError string
	}{
		{name: "custom set", data: "name: p\nenvironments: [preprd, prd]\n", want: []string{"preprd", "prd"}},
		{name: "single", data: "name: p\nenvironments: [prd]\n", want: []string{"prd"}},
		{name: "digits and hyphens", data: "name: p\nenvironments: [eu-west-1, prd2]\n", want: []string{"eu-west-1", "prd2"}},
		{
			name:      "missing",
			data:      "name: p\n",
			wantError: "environments must list at least one deployment environment, for example [dev, int, prd]",
		},
		{
			name:      "empty",
			data:      "name: p\nenvironments: []\n",
			wantError: "environments must list at least one deployment environment, for example [dev, int, prd]",
		},
		{
			name:      "null",
			data:      "name: p\nenvironments:\n",
			wantError: "environments must list at least one deployment environment, for example [dev, int, prd]",
		},
		{name: "duplicate", data: "name: p\nenvironments: [dev, prd, dev]\n", wantError: `environments: duplicate environment "dev"`},
		{name: "uppercase", data: "name: p\nenvironments: [Prd]\n", wantError: `environments: invalid environment name "Prd"`},
		{name: "underscore", data: "name: p\nenvironments: [pre_prd]\n", wantError: `environments: invalid environment name "pre_prd"`},
		{name: "leading digit", data: "name: p\nenvironments: [1prd]\n", wantError: `environments: invalid environment name "1prd"`},
		{name: "blank", data: "name: p\nenvironments: ['']\n", wantError: `environments: invalid environment name ""`},
		{name: "whitespace", data: "name: p\nenvironments: [' prd']\n", wantError: `environments: invalid environment name " prd"`},
		{name: "too long", data: "name: p\nenvironments: [" + strings.Repeat("a", 33) + "]\n", wantError: "environments: invalid environment name"},
		// Strict decoding is unchanged: a scalar or a map is not a list.
		{name: "scalar", data: "name: p\nenvironments: prd\n", wantError: "cannot unmarshal"},
		{name: "map", data: "name: p\nenvironments: {prd: true}\n", wantError: "cannot unmarshal"},
		{name: "unknown key still rejected", data: "name: p\nenvironments: [prd]\nenvironment: prd\n", wantError: "field environment not found"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "bear.config.yml")
			if err := os.WriteFile(path, []byte(tt.data), 0600); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("expected %q, got %v", tt.wantError, err)
				}
				// Every environments failure names the file it came from.
				if !strings.Contains(err.Error(), path) {
					t.Errorf("error does not name the config file: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(cfg.Environments, tt.want) {
				t.Fatalf("environments = %v, want %v", cfg.Environments, tt.want)
			}
		})
	}
}
