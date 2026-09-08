package internal

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Original upstream payload at correctedPresetsRevision, including unsafe fallbacks.
const upstreamPython = `name: python
detection:
  files:
    - requirements.txt
    - pyproject.toml
    - setup.py
validation:
  setup:
    - name: Install
      run: pip install -r requirements.txt 2>/dev/null || pip install -e .
  lint:
    - name: Lint
      run: ruff check . 2>/dev/null || pylint **/*.py 2>/dev/null || true
  test:
    - name: Test
      run: pytest 2>/dev/null || python -m unittest discover 2>/dev/null || true
  build:
    - name: Build
      run: python -m build 2>/dev/null || echo 'No build step'
`

func TestPresetPythonCorrectionScope(t *testing.T) {
	upstream, err := parseLanguage([]byte(upstreamPython), "python")
	if err != nil {
		t.Fatal(err)
	}
	for _, revision := range []string{correctedPresetsRevision, strings.Repeat("a", 40)} {
		m := &Manager{cacheDir: t.TempDir(), revision: revision}
		path := filepath.Join(m.cacheDir, revision, "languages/python.yml")
		writeScannerFixture(t, path, upstreamPython)
		language, err := m.GetLanguage("python")
		if err != nil {
			t.Fatal(err)
		}
		if revision == correctedPresetsRevision {
			if reflect.DeepEqual(language.Steps, upstream.Steps) || !reflect.DeepEqual(language.Detection, upstream.Detection) {
				t.Fatalf("correction should replace steps, preserving detection: %+v", language)
			}
			for _, step := range language.Steps {
				if strings.Contains(step.Run, "|| true") || strings.Contains(step.Run, "2>/dev/null") {
					t.Fatalf("failure masking remains: %+v", step)
				}
			}
		} else if !reflect.DeepEqual(language, upstream) {
			t.Fatal("arbitrary revision was rewritten")
		}
		data, err := os.ReadFile(path)
		if err != nil || string(data) != upstreamPython {
			t.Fatalf("upstream cache should remain unmodified: %v", err)
		}
	}
	// The bundled default also works offline without a populated cache.
	m := &Manager{cacheDir: t.TempDir(), revision: correctedPresetsRevision}
	if _, err := m.GetLanguage("python"); err != nil {
		t.Fatal(err)
	}
}

func TestPresetPythonFailuresPropagate(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is required for execution regression tests")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is required for preset execution")
	}
	language, err := NewManager(correctedPresetsRevision).GetLanguage("python")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		step  int
		files map[string]string
		want  string
		code  int
	}{
		{"pytest", 2, map[string]string{"test_failure.py": "def test_failure():\n    assert False, 'pytest failure sentinel'\n"}, "pytest failure sentinel", 1},
		{"broken pytest", 2, map[string]string{"pytest.py": "raise RuntimeError('broken pytest sentinel')\n"}, "broken pytest sentinel", 1},
		{"unittest", 2, map[string]string{"test_failure.py": "import unittest\nclass Failure(unittest.TestCase):\n    def test_failure(self):\n        self.fail('unittest failure sentinel')\n"}, "unittest failure sentinel", 1},
		{"syntax", 1, map[string]string{"invalid.py": "def invalid(:\n"}, "SyntaxError", 1},
		{"requirements install", 0, map[string]string{"requirements.txt": "", "pip.py": "import sys\nprint('pip failure sentinel', sys.argv)\nsys.exit(7)\n"}, "pip failure sentinel", 7},
		{"editable install", 0, map[string]string{"setup.py": "", "pip.py": "import sys\nprint('editable failure sentinel', sys.argv)\nsys.exit(8)\n"}, "editable failure sentinel", 8},
		{"package build", 3, map[string]string{"pyproject.toml": "", "build.py": "import sys\nprint('build failure sentinel')\nsys.exit(9)\n"}, "build failure sentinel", 9},
		{"script only build", 3, nil, "skipping package build", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			interpreter := python
			if tc.name == "pytest" {
				if err := exec.Command(python, "-c", "import pytest").Run(); err != nil {
					t.Skip("pytest must be installed in python3 for the pytest regression test")
				}
			}
			if tc.name == "unittest" {
				venv := filepath.Join(t.TempDir(), "venv")
				if out, err := exec.Command(python, "-m", "venv", "--without-pip", venv).CombinedOutput(); err != nil {
					t.Fatalf("create isolated interpreter without pytest: %v\n%s", err, out)
				}
				interpreter = filepath.Join(venv, "bin", "python")
			}
			root := t.TempDir()
			for name, data := range tc.files {
				writeScannerFixture(t, filepath.Join(root, name), data)
			}
			bin := t.TempDir()
			wrapper := filepath.Join(bin, "python")
			writeScannerFixture(t, wrapper, "#!/bin/sh\nexec '"+strings.ReplaceAll(interpreter, "'", "'\\''")+"' \"$@\"\n")
			if err := os.Chmod(wrapper, 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("PYTHONPATH", "")
			t.Setenv("PYTEST_DISABLE_PLUGIN_AUTOLOAD", "1")
			cmd := exec.Command("sh", "-c", language.Steps[tc.step].Run)
			cmd.Dir = root
			output, err := cmd.CombinedOutput()
			if tc.code == 0 {
				if err != nil {
					t.Fatalf("unexpected error: %v\n%s", err, output)
				}
			} else if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != tc.code {
				t.Fatalf("expected exit %d, got %v\n%s", tc.code, err, output)
			}
			if !strings.Contains(string(output), tc.want) {
				t.Fatalf("diagnostic was hidden: expected %q\n%s", tc.want, output)
			}
		})
	}
}
