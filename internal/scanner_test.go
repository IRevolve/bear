package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/irevolve/bear/internal/config"
)

func writeScannerFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestScannerLanguageOverlap(t *testing.T) {
	root := t.TempDir()
	writeScannerFixture(t, filepath.Join(root, "package.json"), "{}")
	writeScannerFixture(t, filepath.Join(root, "tsconfig.json"), "{}")
	cfg := &config.Config{Languages: map[string]config.Language{
		"node":       {Detection: config.Detection{Files: []string{"package.json"}}},
		"typescript": {Detection: config.Detection{Files: []string{"tsconfig.json"}}},
	}}
	path := filepath.Join(root, "bear.artifact.yml")
	writeScannerFixture(t, path, "name: web\ntarget: docker\n")
	for i := 0; i < 20; i++ {
		if _, err := ScanArtifacts(root, cfg); err == nil || !strings.Contains(err.Error(), "node, typescript") || !strings.Contains(err.Error(), "set language explicitly") {
			t.Fatalf("expected actionable deterministic ambiguity, got %v", err)
		}
	}
	writeScannerFixture(t, path, "name: web\ntarget: docker\nlanguage: typescript\n")
	artifacts, err := ScanArtifacts(root, cfg)
	if err != nil || len(artifacts) != 1 || artifacts[0].Language != "typescript" {
		t.Fatalf("explicit language: %+v, %v", artifacts, err)
	}
	writeScannerFixture(t, path, "name: web\ntarget: docker\nlanguage: typo\n")
	if _, err := ScanArtifacts(root, cfg); err == nil {
		t.Fatal("unknown explicit language accepted")
	}
}

func TestScannerIgnoredDirectories(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{".git", ".bear", "node_modules", "vendor", "generated", "dist", "build", "custom", "nested/cache"} {
		writeScannerFixture(t, filepath.Join(root, dir, "bear.artifact.yml"), "invalid: [")
	}
	writeScannerFixture(t, filepath.Join(root, "src", "bear.lib.yml"), "name: common\nlanguage: go\n")
	cfg := &config.Config{IgnoreDirs: []string{"custom", "nested/cache"}, Languages: map[string]config.Language{"go": {}}}
	artifacts, err := ScanArtifacts(root, cfg)
	if err != nil || len(artifacts) != 1 || artifacts[0].Artifact.Name != "common" || artifacts[0].Language != "go" {
		t.Fatalf("scan result: %+v, %v", artifacts, err)
	}
}

func TestLoaderEmptyDeploymentTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bear.config.yml")
	writeScannerFixture(t, path, "name: project\nlanguages: {go: {steps: []}}\ntargets: {docker: {steps: []}}\n")
	if _, err := config.Load(path); err != nil {
		t.Fatalf("syntax-level config load rejected empty steps: %v", err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "no deployment steps") {
		t.Fatalf("effective config accepted empty deployment: %v", err)
	}
	writeScannerFixture(t, path, "name: project\nlanguages: {go: {steps: []}}\n")
	if _, err := Load(path); err != nil {
		t.Fatalf("validation-only config rejected: %v", err)
	}
}
