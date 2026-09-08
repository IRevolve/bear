package internal

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const upstreamJava = `name: java
detection:
  files: [pom.xml, build.gradle, build.gradle.kts]
validation:
  setup:
    - name: Download
      run: mvn dependency:go-offline 2>/dev/null || gradle dependencies 2>/dev/null || true
  lint:
    - name: Check
      run: mvn checkstyle:check 2>/dev/null || gradle check 2>/dev/null || true
  test:
    - name: Test
      run: mvn test 2>/dev/null || gradle test 2>/dev/null
  build:
    - name: Build
      run: mvn package -DskipTests 2>/dev/null || gradle build -x test 2>/dev/null
`

func TestPresetJavaCorrectionScope(t *testing.T) {
	upstream, err := parseLanguage([]byte(upstreamJava), "java")
	if err != nil {
		t.Fatal(err)
	}
	for _, revision := range []string{correctedPresetsRevision, strings.Repeat("a", 40)} {
		m := &Manager{cacheDir: t.TempDir(), revision: revision}
		writeScannerFixture(t, filepath.Join(m.cacheDir, revision, "languages/java.yml"), upstreamJava)
		language, err := m.GetLanguage("java")
		if err != nil {
			t.Fatal(err)
		}
		if revision == correctedPresetsRevision {
			if reflect.DeepEqual(language.Steps, upstream.Steps) || !reflect.DeepEqual(language.Detection, upstream.Detection) {
				t.Fatalf("incorrect correction: %+v", language)
			}
		} else if !reflect.DeepEqual(language, upstream) {
			t.Fatal("arbitrary revision was rewritten")
		}
	}
}

func TestPresetJavaFailuresPropagate(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh is required for preset execution")
	}
	language, err := NewManager(correctedPresetsRevision).GetLanguage("java")
	if err != nil {
		t.Fatal(err)
	}
	for _, buildFile := range []string{"pom.xml", "build.gradle", "build.gradle.kts"} {
		for _, step := range language.Steps {
			t.Run(buildFile+"/"+step.Name, func(t *testing.T) {
				root, bin := t.TempDir(), t.TempDir()
				writeScannerFixture(t, filepath.Join(root, buildFile), "")
				selected := "gradle"
				if buildFile == "pom.xml" {
					selected = "mvn"
				}
				for _, tool := range []string{"mvn", "gradle"} {
					script := "#!/bin/sh\nprintf '%s\\n' 'unexpected fallback'\nexit 0\n"
					if tool == selected {
						script = "#!/bin/sh\nprintf '%s\\n' 'validation failure sentinel' >&2\nexit 7\n"
					}
					path := filepath.Join(bin, tool)
					writeScannerFixture(t, path, script)
					if err := os.Chmod(path, 0700); err != nil {
						t.Fatal(err)
					}
				}
				t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
				cmd := exec.Command("sh", "-c", step.Run)
				cmd.Dir = root
				output, err := cmd.CombinedOutput()
				if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 7 {
					t.Fatalf("expected selected tool failure, got %v\n%s", err, output)
				}
				if !strings.Contains(string(output), "validation failure sentinel") || strings.Contains(string(output), "unexpected fallback") {
					t.Fatalf("failure hidden or retried: %s", output)
				}
			})
		}
	}
}
