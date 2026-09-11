package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStrictConfigValidation(t *testing.T) {
	// Every case but the first two carries a valid environments list so that it
	// still fails for the reason it names, not for a missing declaration.
	for _, data := range []string{
		"", "name: ' '",
		"name: project\nenvironments: [dev]\ntypo: true",
		"name: project\nenvironments: [dev]\n---\nname: another",
		"name: project\nenvironments: [dev]\nuse: {revision: main}",
		"name: project\nenvironments: [dev]\nuse: {languages: [../go]}",
		"name: project\nenvironments: [dev]\nlanguages: {' ': {steps: []}}",
		"name: project\nenvironments: [dev]\ntargets: {' ': {steps: []}}",
		"name: project\nenvironments: [dev]\nlanguages: {go: {steps: [{name: test}]}}",
		"name: project\nenvironments: [dev]\nlanguages: {go: {}}",
		"name: project\nenvironments: [dev]\nlanguages: {go: {steps: null}}",
		"name: project\nenvironments: [dev]\ntargets: {docker: {steps: [{name: ' ', run: 'true'}]}}",
		"name: project\nenvironments: [dev]\nlanguages: {go: {validation: {test: []}}}",
		"name: project\nenvironments: [dev]\nlanguages: {go: {steps: [], detection: {pattern: '['}}}",
		"name: project\nenvironments: [dev]\nignore_dirs: [../outside]",
		// The declaration itself is part of strict validation.
		"name: project",
		"name: project\nenvironments: []",
		"name: project\nenvironments: [dev, dev]",
		"name: project\nenvironments: [Dev]",
	} {
		t.Run(data, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "bear.config.yml")
			if err := os.WriteFile(path, []byte(data), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("invalid config accepted")
			}
		})
	}
}

func TestStrictArtifactAndLibraryValidation(t *testing.T) {
	for _, data := range []string{"", "name: ' '", "name: api\ntarget: ' '", "name: api\ntarget: docker\ntyop: true", "name: api\ntarget: docker\ndepends: [' ']", "name: api\ntarget: docker\n---\nname: other"} {
		path := filepath.Join(t.TempDir(), "bear.artifact.yml")
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadArtifact(path); err == nil {
			t.Errorf("accepted artifact: %s", data)
		}
	}
	for _, data := range []string{"", "name: ' '", "name: common\ntypo: true", "name: common\ndepends: [' ']", "name: common\n---\nname: other"} {
		path := filepath.Join(t.TempDir(), "bear.lib.yml")
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadLibrary(path); err == nil {
			t.Errorf("accepted library: %s", data)
		}
	}
}
