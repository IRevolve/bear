package internal

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLoaderUsesSelectedRevisionCache(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	revision := strings.Repeat("a", 40)
	writeScannerFixture(t, filepath.Join(home, CacheDir, revision, "languages/go.yml"), upstreamGo)
	writeScannerFixture(t, filepath.Join(home, CacheDir, revision, "targets/docker.yml"), upstreamDocker)
	path := filepath.Join(t.TempDir(), "bear.config.yml")
	writeScannerFixture(t, path, "name: project\nuse:\n  revision: "+revision+"\n  languages: [go]\n  targets: [docker]\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Languages["go"].Steps) != 4 || len(cfg.Targets["docker"].Steps) != 2 {
		t.Fatalf("presets not resolved: %+v", cfg)
	}
}

func TestLoaderLocalOverridesPreset(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "bear.config.yml")
	writeScannerFixture(t, path, `name: project
use:
  languages: [not-a-remote-language]
  targets: [not-a-remote-target]
languages:
  not-a-remote-language:
    steps: []
targets:
  not-a-remote-target:
    steps: [{name: Deploy, run: 'true'}]
`)
	if _, err := Load(path); err != nil {
		t.Fatalf("local override fetched remote: %v", err)
	}
}

func TestLoaderPythonCorrectionPreservesLocalOverride(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "bear.config.yml")
	writeScannerFixture(t, path, "name: project\nuse: {languages: [python]}\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Languages["python"].Steps[1].Run != "python -m compileall ." {
		t.Fatal("default import did not use maintained correction")
	}
	writeScannerFixture(t, path, "name: project\nuse: {languages: [python]}\nlanguages:\n  python:\n    steps: [{name: Custom, run: 'custom-command || true'}]\n")
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	steps := cfg.Languages["python"].Steps
	if len(steps) != 1 || steps[0].Run != "custom-command || true" {
		t.Fatalf("local commands were rewritten: %+v", steps)
	}
}

func TestLoaderJavaCorrectionPreservesLocalOverride(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	path := filepath.Join(t.TempDir(), "bear.config.yml")
	writeScannerFixture(t, path, "name: project\nuse: {languages: [java]}\n")
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cfg.Languages["java"].Steps[0].Run, "if [ -f pom.xml ]") {
		t.Fatal("default import did not use maintained correction")
	}
	writeScannerFixture(t, path, "name: project\nuse: {languages: [java]}\nlanguages:\n  java:\n    steps: [{name: Custom, run: 'custom-command || true'}]\n")
	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	steps := cfg.Languages["java"].Steps
	if len(steps) != 1 || steps[0].Run != "custom-command || true" {
		t.Fatalf("local commands were rewritten: %+v", steps)
	}
}
