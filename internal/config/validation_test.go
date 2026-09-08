package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStrictConfigValidation(t *testing.T) {
	for _, data := range []string{
		"", "name: ' '", "name: project\ntypo: true", "name: project\n---\nname: another",
		"name: project\nuse: {revision: main}", "name: project\nuse: {languages: [../go]}",
		"name: project\nlanguages: {' ': {steps: []}}", "name: project\ntargets: {' ': {steps: []}}",
		"name: project\nlanguages: {go: {steps: [{name: test}]}}",
		"name: project\nlanguages: {go: {}}", "name: project\nlanguages: {go: {steps: null}}",
		"name: project\ntargets: {docker: {steps: [{name: ' ', run: 'true'}]}}",
		"name: project\nlanguages: {go: {validation: {test: []}}}",
		"name: project\nlanguages: {go: {steps: [], detection: {pattern: '['}}}",
		"name: project\nignore_dirs: [../outside]",
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
