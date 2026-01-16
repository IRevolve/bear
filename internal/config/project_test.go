package config

import (
	"os"
	"path/filepath"
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
languages:
  - name: go
    detection:
      files: [go.mod]
    validation:
      setup:
        - name: Download
          run: go mod download
      lint:
        - name: Vet
          run: go vet ./...
      test:
        - name: Test
          run: go test ./...
      build:
        - name: Build
          run: go build .
targets:
  - name: docker
    defaults:
      REGISTRY: ghcr.io
    deploy:
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
	if len(cfg.Languages) != 1 {
		t.Errorf("expected 1 language, got %d", len(cfg.Languages))
	}
	if cfg.Languages[0].Name != "go" {
		t.Errorf("expected language 'go', got '%s'", cfg.Languages[0].Name)
	}
	if len(cfg.Languages[0].Detection.Files) != 1 {
		t.Errorf("expected 1 detection file, got %d", len(cfg.Languages[0].Detection.Files))
	}
	if len(cfg.Languages[0].Validation.Lint) != 1 {
		t.Errorf("expected 1 lint step, got %d", len(cfg.Languages[0].Validation.Lint))
	}
	if len(cfg.Targets) != 1 {
		t.Errorf("expected 1 target, got %d", len(cfg.Targets))
	}
	if cfg.Targets[0].Defaults["REGISTRY"] != "ghcr.io" {
		t.Errorf("expected REGISTRY 'ghcr.io', got '%s'", cfg.Targets[0].Defaults["REGISTRY"])
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
	if err := os.WriteFile(configPath, []byte("invalid: yaml: content:"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err = Load(configPath)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}
