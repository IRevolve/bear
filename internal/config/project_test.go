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
	configContent := `name = "test-project"

[languages.go]
detection = { files = ["go.mod"] }
steps = [
  { name = "Download", run = "go mod download" },
  { name = "Vet", run = "go vet ./..." },
  { name = "Test", run = "go test ./..." },
  { name = "Build", run = "go build ." },
]

[targets.docker]
vars = { REGISTRY = "ghcr.io" }
steps = [
  { name = "Build", run = "docker build ." },
]
`
	configPath := filepath.Join(tmpDir, "bear.config.toml")
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
	_, err := Load("/nonexistent/path/bear.config.toml")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestLoad_InvalidTOML(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "bear-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write invalid TOML
	configPath := filepath.Join(tmpDir, "bear.config.toml")
	if err := os.WriteFile(configPath, []byte("invalid = [broken"), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err = Load(configPath)
	if err == nil {
		t.Error("expected error for invalid TOML")
	}
}
