package commands

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/irevolve/bear/internal"
	"github.com/irevolve/bear/internal/config"
)

func TestGenerateConfigSafeYAML(t *testing.T) {
	for _, name := range []string{"project", "project: name", "project # comment", "true", "[project]", "line\nbreak", "it's a project", "*alias"} {
		t.Run(name, func(t *testing.T) {
			data, err := generateConfig(name, []string{"preprd", "prd"}, []string{"go", "typescript"}, []string{"docker"})
			if err != nil {
				t.Fatal(err)
			}
			var cfg config.Config
			if err := config.DecodeStrict(data, &cfg); err != nil {
				t.Fatalf("invalid YAML: %v\n%s", err, data)
			}
			if cfg.Name != name || cfg.Use.Revision != internal.DefaultPresetsRevision || len(cfg.Use.Languages) != 2 || len(cfg.Use.Targets) != 1 {
				t.Fatalf("round-trip mismatch: %+v", cfg)
			}
			if !reflect.DeepEqual(cfg.Environments, []string{"preprd", "prd"}) {
				t.Fatalf("environments not encoded: %+v", cfg.Environments)
			}
		})
	}
}

// initFlags isolates the package-level flag variables the init command reads.
func initFlags(t *testing.T, dir string, environments []string) {
	t.Helper()
	oldDir, oldLanguages, oldTargets, oldEnvironments, oldForce := workDir, initLanguages, initTargets, initEnvironments, initForce
	t.Cleanup(func() {
		workDir, initLanguages, initTargets, initEnvironments, initForce = oldDir, oldLanguages, oldTargets, oldEnvironments, oldForce
	})
	workDir, initLanguages, initTargets, initEnvironments, initForce = dir, nil, nil, environments, false
}

func TestInitWritesSafeProjectName(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "project: name # comment")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	initFlags(t, dir, defaultInitEnvironments)
	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(filepath.Join(dir, "bear.config.yml"))
	if err != nil || cfg.Name != filepath.Base(dir) {
		t.Fatalf("generated config: %+v, %v", cfg, err)
	}
	if !reflect.DeepEqual(cfg.Environments, []string{"dev", "int", "prd"}) {
		t.Fatalf("default environments: %v", cfg.Environments)
	}
	if err := initCmd.RunE(initCmd, nil); err == nil {
		t.Fatal("init overwrote existing config without force")
	}
}

// A generated config that cannot be loaded is worse than no config, so init
// both writes a valid environments list and refuses an invalid one.
func TestInitEnvironments(t *testing.T) {
	for _, tt := range []struct {
		name         string
		environments []string
		want         []string
		wantError    string
	}{
		{name: "default", environments: defaultInitEnvironments, want: []string{"dev", "int", "prd"}},
		{name: "custom", environments: []string{"preprd", "prd"}, want: []string{"preprd", "prd"}},
		{name: "single", environments: []string{"prd"}, want: []string{"prd"}},
		{name: "hyphens and digits", environments: []string{"eu-west-1"}, want: []string{"eu-west-1"}},
		{name: "empty", environments: nil, wantError: "--environments must list at least one deployment environment"},
		{name: "blank value", environments: []string{""}, wantError: `--environments: invalid environment name ""`},
		{name: "uppercase", environments: []string{"PRD"}, wantError: `--environments: invalid environment name "PRD"`},
		{name: "underscore", environments: []string{"pre_prd"}, wantError: `--environments: invalid environment name "pre_prd"`},
		{name: "spaced", environments: []string{"dev", " prd"}, wantError: `--environments: invalid environment name " prd"`},
		{name: "duplicate", environments: []string{"prd", "prd"}, wantError: `--environments: duplicate environment "prd"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			initFlags(t, dir, tt.environments)
			err := initCmd.RunE(initCmd, nil)
			configPath := filepath.Join(dir, "bear.config.yml")
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("expected %q, got %v", tt.wantError, err)
				}
				// A rejected flag writes nothing at all.
				if _, err := os.Stat(configPath); !os.IsNotExist(err) {
					t.Fatalf("invalid environments still wrote a config: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			cfg, err := config.Load(configPath)
			if err != nil {
				t.Fatalf("generated config does not load: %v", err)
			}
			if !reflect.DeepEqual(cfg.Environments, tt.want) {
				t.Fatalf("environments = %v, want %v", cfg.Environments, tt.want)
			}
		})
	}
}
