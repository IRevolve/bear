package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/irevolve/bear/internal"
	"github.com/irevolve/bear/internal/config"
)

func TestPresetShowEmitsLocalYAML(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	oldRevision := presetRevision
	presetRevision = internal.DefaultPresetsRevision
	t.Cleanup(func() { presetRevision = oldRevision; presetShowCmd.SetOut(nil) })
	for _, tc := range []struct{ kind, category, name, data string }{
		{"language", "languages", "go", "name: go\nvalidation:\n  test:\n    - name: Test\n      run: 'printf \"a: b\"'\n"},
		{"target", "targets", "docker", "name: docker\ndefaults: {KEY: 'a: b'}\ndeploy: [{name: Deploy, run: 'true'}]\n"},
	} {
		path := filepath.Join(home, internal.CacheDir, presetRevision, tc.category, tc.name+".yml")
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(tc.data), 0600); err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		presetShowCmd.SetOut(&output)
		if err := presetShowCmd.RunE(presetShowCmd, []string{tc.kind, tc.name}); err != nil {
			t.Fatal(err)
		}
		if tc.kind == "language" {
			var language config.Language
			if err := config.DecodeStrict(output.Bytes(), &language); err != nil || len(language.Steps) != 1 {
				t.Fatalf("invalid language output: %v\n%s", err, output.String())
			}
		} else {
			var target config.Target
			if err := config.DecodeStrict(output.Bytes(), &target); err != nil || len(target.Steps) != 1 || target.Vars["KEY"] != "a: b" {
				t.Fatalf("invalid target output: %v\n%s", err, output.String())
			}
		}
	}
}
