package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/irevolve/bear/internal"
	"github.com/irevolve/bear/internal/config"
)

func TestGenerateConfigSafeYAML(t *testing.T) {
	for _, name := range []string{"project", "project: name", "project # comment", "true", "[project]", "line\nbreak", "it's a project", "*alias"} {
		t.Run(name, func(t *testing.T) {
			data, err := generateConfig(name, []string{"go", "typescript"}, []string{"docker"})
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
		})
	}
}

func TestInitWritesSafeProjectName(t *testing.T) {
	oldDir, oldLanguages, oldTargets, oldForce := workDir, initLanguages, initTargets, initForce
	t.Cleanup(func() { workDir, initLanguages, initTargets, initForce = oldDir, oldLanguages, oldTargets, oldForce })
	workDir = filepath.Join(t.TempDir(), "project: name # comment")
	if err := os.Mkdir(workDir, 0755); err != nil {
		t.Fatal(err)
	}
	initLanguages, initTargets, initForce = nil, nil, false
	if err := initCmd.RunE(initCmd, nil); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(filepath.Join(workDir, "bear.config.yml"))
	if err != nil || cfg.Name != filepath.Base(workDir) {
		t.Fatalf("generated config: %+v, %v", cfg, err)
	}
	if err := initCmd.RunE(initCmd, nil); err == nil {
		t.Fatal("init overwrote existing config without force")
	}
}
