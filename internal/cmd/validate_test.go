package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/irevolve/bear/internal/config"
)

// validateRecord records the ENVIRONMENT each artifact ran with, which proves
// both which artifacts ran and what they were handed.
const validateRecord = `printf '%s' "${ENVIRONMENT-unset}" > "$BEAR_TEST_OUTPUT/$(basename "$PWD").validated"`

// validatePass is a step that succeeds without recording anything.
const validatePass = `exit 0`

// validateFixture writes a project with two services, a library and an
// artifact whose language configures no steps. It is deliberately not a Git
// repository: validation must work in a shallow clone or a plain export.
// testRun and buildRun are the two validation steps of the "test" language.
func validateFixture(t *testing.T, testRun, buildRun string) (root, configPath, markers string) {
	t.Helper()
	root, markers = t.TempDir(), t.TempDir()
	t.Setenv("BEAR_TEST_OUTPUT", markers)
	// A merge-request job inherits an environment; validation must not silently
	// pass it off as an injected one.
	t.Setenv("ENVIRONMENT", "ambient")
	configPath = filepath.Join(root, "bear.config.yml")
	writeEnvironmentFixture(t, configPath, fmt.Sprintf(`name: validate-test
environments: [dev, int, prd]
languages:
  test:
    detection:
      files: [bear.artifact.yml, bear.lib.yml]
    steps:
      - name: Test
        run: |
          %s
      - name: Build
        run: |
          %s
  none:
    detection:
      files: [none.marker]
    steps: []
targets:
  local:
    steps:
      - name: deploy
        run: 'true'
`, testRun, buildRun))
	// checkout-api depends on shared to prove that naming it does not drag its
	// dependencies in. web has no environments allowlist to prove that
	// validation ignores deployment permissions entirely.
	writeEnvironmentFixture(t, filepath.Join(root, "services", "checkout-api", "bear.artifact.yml"), "name: checkout-api\ntarget: local\ndepends: [shared]\nenvironments: [dev, int, prd]\n")
	writeEnvironmentFixture(t, filepath.Join(root, "services", "web", "bear.artifact.yml"), "name: web\ntarget: local\n")
	writeEnvironmentFixture(t, filepath.Join(root, "libs", "shared", "bear.lib.yml"), "name: shared\n")
	writeEnvironmentFixture(t, filepath.Join(root, "tools", "generator", "bear.artifact.yml"), "name: generator\nlanguage: none\ntarget: local\n")
	return root, configPath, markers
}

// validateMarkers maps each artifact that ran to the ENVIRONMENT it saw.
func validateMarkers(t *testing.T, markers string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(markers)
	if err != nil {
		t.Fatal(err)
	}
	recorded := make(map[string]string, len(entries))
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(markers, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		recorded[strings.TrimSuffix(entry.Name(), ".validated")] = string(data)
	}
	return recorded
}

// validateNoDeploymentState asserts that validation left no deployment
// artifacts behind and did not disturb a plan another command may own.
func validateNoDeploymentState(t *testing.T, root string, plan []byte) {
	t.Helper()
	saved, err := os.ReadFile(config.PlanFilePath(root))
	if plan == nil {
		if !os.IsNotExist(err) {
			t.Fatalf("validate wrote a plan file: %v", err)
		}
	} else if err != nil || !bytes.Equal(saved, plan) {
		t.Fatalf("validate rewrote the saved plan: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "bear.lock.yml")); !os.IsNotExist(err) {
		t.Fatalf("validate touched the lock file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); !os.IsNotExist(err) {
		t.Fatalf("fixture became a repository: %v", err)
	}
}

func TestValidateSuccess(t *testing.T) {
	root, path, markers := validateFixture(t, validateRecord, validatePass)
	// An unrelated saved plan belongs to plan/apply. Validation neither reads,
	// rewrites nor removes it.
	if err := config.WritePlan(root, config.NewPlanFile("stale")); err != nil {
		t.Fatal(err)
	}
	plan, err := os.ReadFile(config.PlanFilePath(root))
	if err != nil {
		t.Fatal(err)
	}
	output, err := captureEnvironmentOutput(t, func() error {
		return ValidateWithOptions(path, Options{Concurrency: 2})
	})
	if err != nil {
		t.Fatalf("%v\n%s", err, output)
	}
	// Validation brands itself like plan and apply, reports each job live, and
	// closes with one sentence. Everything else is deployment reporting.
	for _, text := range []string{
		"Bear Validate",
		"Validating 4 artifacts",
		"checkout-api: Validating... [1/2 Test]",
		"checkout-api: Validating... [2/2 Build]",
		"checkout-api: Validation complete after ",
		"Validating... [no validation steps]",
		"Validation complete: 4 artifacts in ",
	} {
		if !strings.Contains(output, text) {
			t.Errorf("validate output missing %q: %s", text, output)
		}
	}
	for _, gone := range []string{summaryRule, "Environment:", "deploy (", "skip (", "failed (", "bear apply"} {
		if strings.Contains(output, gone) {
			t.Errorf("validate reported deployment state %q: %s", gone, output)
		}
	}
	// The library validates exactly like an artifact; the artifact whose
	// language defines no steps runs nothing and is not a failure.
	want := map[string]string{"checkout-api": "ambient", "web": "ambient", "shared": "ambient"}
	if recorded := validateMarkers(t, markers); !reflect.DeepEqual(recorded, want) {
		t.Errorf("validated %v, want %v", recorded, want)
	}
	validateNoDeploymentState(t, root, plan)
}

// A merge-request job may run in a shallow clone or an export that is not a
// repository at all, so validation must never shell out to Git.
func TestValidateWithoutGit(t *testing.T) {
	if isWindows() {
		t.Skip("shell stub requires a POSIX shell")
	}
	root, path, markers := validateFixture(t, validateRecord, validatePass)
	stub := t.TempDir()
	attempted := filepath.Join(stub, "invoked")
	writeEnvironmentFixture(t, filepath.Join(stub, "git"), "#!/bin/sh\ntouch "+attempted+"\nexit 1\n")
	if err := os.Chmod(filepath.Join(stub, "git"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", stub+string(os.PathListSeparator)+os.Getenv("PATH"))
	output, err := captureEnvironmentOutput(t, func() error {
		return ValidateWithOptions(path, Options{Concurrency: 2})
	})
	if err != nil {
		t.Fatalf("%v\n%s", err, output)
	}
	if _, err := os.Stat(attempted); !os.IsNotExist(err) {
		t.Fatalf("validate ran Git outside a repository: %v", err)
	}
	if len(validateMarkers(t, markers)) != 3 {
		t.Fatalf("validation did not run without Git: %s", output)
	}
	validateNoDeploymentState(t, root, nil)
}

func TestValidateFailureReportsFailedArtifacts(t *testing.T) {
	root, path, _ := validateFixture(t,
		`test "$(basename "$PWD")" != checkout-api || exit 1`,
		`test "$(basename "$PWD")" != shared || exit 2`)
	output, err := captureEnvironmentOutput(t, func() error {
		return ValidateWithOptions(path, Options{Concurrency: 4})
	})
	// A non-nil error is what makes the CI job red.
	if err == nil {
		t.Fatalf("failed validation returned success: %s", output)
	}
	for _, text := range []string{"exit status 1", "exit status 2"} {
		if !strings.Contains(err.Error(), text) {
			t.Errorf("returned error missing %q: %v", text, err)
		}
	}
	// A failing run reopens the summary with the failures only: the rule, one
	// counted section naming every artifact that failed and why, then the
	// closing sentence. There is no environment and no other section.
	block := strings.Join([]string{
		summaryRule,
		"",
		"failed (2):",
		"  - checkout-api (services/checkout-api): Test: exit status 1",
		"  - shared (libs/shared): Build: exit status 2",
		"",
	}, "\n")
	if !strings.Contains(output, block) {
		t.Fatalf("failure summary does not match:\n%s", output)
	}
	for _, text := range []string{
		"Bear Validate",
		"Validating 4 artifacts",
		"checkout-api: Validation failed after ",
		"Validation failed: 2 passed, 2 failed in ",
	} {
		if !strings.Contains(output, text) {
			t.Errorf("validate output missing %q: %s", text, output)
		}
	}
	for _, gone := range []string{"Environment:", "deploy (", "skip (", "Validation complete: "} {
		if strings.Contains(output, gone) {
			t.Errorf("failed validation reported deployment state %q: %s", gone, output)
		}
	}
	validateNoDeploymentState(t, root, nil)
}

func TestValidateVerboseStreamsWithoutDuplicateCapture(t *testing.T) {
	_, path, _ := validateFixture(t,
		`if [ "$(basename "$PWD")" = web ]; then printf 'unique-step-output\n'; exit 1; fi`,
		validatePass)
	output, err := captureEnvironmentOutput(t, func() error {
		return ValidateWithOptions(path, Options{Concurrency: 1, Verbose: true})
	})
	if err == nil {
		t.Fatalf("expected step failure: %s", output)
	}
	if strings.Count(output, "unique-step-output") != 1 || !strings.Contains(output, "web | Test | unique-step-output") {
		t.Fatalf("verbose output not streamed exactly once: %s", output)
	}
	if !strings.Contains(output, "Validation failed: 3 passed, 1 failed in ") {
		t.Fatalf("missing closing sentence: %s", output)
	}
}

// The environment names the variable injected into the steps and nothing else:
// it selects no policy, prints no verdict, and is optional.
func TestValidateEnvironment(t *testing.T) {
	for _, tt := range []struct {
		name        string
		environment string
		want        string
		wantError   string
	}{
		{name: "absent inherits the job environment", want: "ambient"},
		{name: "dev", environment: "dev", want: "dev"},
		{name: "int", environment: "int", want: "int"},
		{name: "prd", environment: "prd", want: "prd"},
		{name: "undeclared", environment: "production", wantError: `unknown environment "production": bear.config.yml declares dev, int, prd`},
		{name: "malformed", environment: "PRD", wantError: `invalid environment name "PRD"`},
		{name: "artifact is not an environment", environment: "checkout-api", wantError: `unknown environment "checkout-api": bear.config.yml declares dev, int, prd`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root, path, markers := validateFixture(t, validateRecord, validatePass)
			output, err := captureEnvironmentOutput(t, func() error {
				return ValidateWithOptions(path, Options{Environment: tt.environment, Concurrency: 2})
			})
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("expected %q, got %v\n%s", tt.wantError, err, output)
				}
				if recorded := validateMarkers(t, markers); len(recorded) > 0 {
					t.Fatalf("invalid environment still ran steps: %v", recorded)
				}
				// A rejected flag must not even create the workspace directory.
				if _, err := os.Stat(filepath.Join(root, ".bear")); !os.IsNotExist(err) {
					t.Fatalf("rejected environment touched the workspace: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("%v\n%s", err, output)
			}
			want := map[string]string{"checkout-api": tt.want, "web": tt.want, "shared": tt.want}
			if recorded := validateMarkers(t, markers); !reflect.DeepEqual(recorded, want) {
				t.Fatalf("validated %v, want %v", recorded, want)
			}
			// An environment never turns validation into a deployment verdict.
			for _, gone := range []string{"Environment:", "deploy (", "skip ("} {
				if strings.Contains(output, gone) {
					t.Errorf("environment produced deployment output %q: %s", gone, output)
				}
			}
		})
	}
}

func TestValidateArtifactSelection(t *testing.T) {
	for _, tt := range []struct {
		name      string
		artifacts []string
		want      []string
		wantError string
	}{
		{name: "all by default", want: []string{"checkout-api", "shared", "web"}},
		{name: "one artifact without its dependencies", artifacts: []string{"checkout-api"}, want: []string{"checkout-api"}},
		{name: "a library", artifacts: []string{"shared"}, want: []string{"shared"}},
		{name: "several artifacts", artifacts: []string{"web", "shared"}, want: []string{"shared", "web"}},
		{name: "a repeated name runs once", artifacts: []string{"web", "web"}, want: []string{"web"}},
		{name: "unknown name", artifacts: []string{"typo"}, wantError: `unknown artifact "typo"`},
		{name: "unknown name beside a known one", artifacts: []string{"web", "typo"}, wantError: `unknown artifact "typo"`},
		{name: "every unknown name is reported", artifacts: []string{"typo", "web", "other"}, wantError: `unknown artifact "other", "typo"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, path, markers := validateFixture(t, validateRecord, validatePass)
			output, err := captureEnvironmentOutput(t, func() error {
				return ValidateWithOptions(path, Options{Artifacts: tt.artifacts, Concurrency: 2})
			})
			if tt.wantError != "" {
				// A typo must fail the job, not silently validate nothing.
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("expected %q, got %v\n%s", tt.wantError, err, output)
				}
				if recorded := validateMarkers(t, markers); len(recorded) > 0 {
					t.Fatalf("unknown name still ran steps: %v", recorded)
				}
				return
			}
			if err != nil {
				t.Fatalf("%v\n%s", err, output)
			}
			var ran []string
			for name := range validateMarkers(t, markers) {
				ran = append(ran, name)
			}
			slices.Sort(ran)
			if !reflect.DeepEqual(ran, tt.want) {
				t.Fatalf("validated %v, want %v\n%s", ran, tt.want, output)
			}
		})
	}
}

// An artifact whose language configures nothing to run is reported, not hidden
// and not failed.
func TestValidateWithoutSteps(t *testing.T) {
	root, path, markers := validateFixture(t, validateRecord, validatePass)
	output, err := captureEnvironmentOutput(t, func() error {
		return ValidateWithOptions(path, Options{Artifacts: []string{"generator"}, Concurrency: 1})
	})
	if err != nil {
		t.Fatalf("an artifact without steps failed validation: %v\n%s", err, output)
	}
	for _, text := range []string{
		"Validating 1 artifact",
		"  generator: Validating... [no validation steps]",
		"  generator: Validation complete after ",
		"Validation complete: 1 artifact in ",
	} {
		if !strings.Contains(output, text) {
			t.Errorf("validate output missing %q: %s", text, output)
		}
	}
	if recorded := validateMarkers(t, markers); len(recorded) > 0 {
		t.Fatalf("an artifact without steps ran something: %v", recorded)
	}
	validateNoDeploymentState(t, root, nil)
}

// Validation has no deployment semantics, but it still loads the graph, so an
// artifact whose allowlist names an environment the project does not declare is
// a configuration error it must surface rather than silently ignore.
func TestValidateRejectsUndeclaredArtifactEnvironment(t *testing.T) {
	root, path, markers := validateFixture(t, validateRecord, validatePass)
	writeEnvironmentFixture(t, filepath.Join(root, "services", "web", "bear.artifact.yml"), "name: web\ntarget: local\nenvironments: [dev, preprd]\n")
	output, err := captureEnvironmentOutput(t, func() error {
		return ValidateWithOptions(path, Options{Concurrency: 1})
	})
	want := `artifact "web" allows undeclared environment "preprd"; bear.config.yml declares dev, int, prd`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("expected %q, got %v\n%s", want, err, output)
	}
	// Selecting other artifacts does not excuse a broken graph.
	if _, err := captureEnvironmentOutput(t, func() error {
		return ValidateWithOptions(path, Options{Artifacts: []string{"shared"}, Concurrency: 1})
	}); err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("selection hid the graph error: %v", err)
	}
	if recorded := validateMarkers(t, markers); len(recorded) > 0 {
		t.Fatalf("a broken graph still ran steps: %v", recorded)
	}
	validateNoDeploymentState(t, root, nil)
}

// Validation reads the working tree, so it must not run while a plan or apply
// is rewriting the same workspace.
func TestValidateWorkspaceLockContention(t *testing.T) {
	root, path, markers := validateFixture(t, validateRecord, validatePass)
	release, err := acquireWorkspaceLock(root)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	output, runErr := captureEnvironmentOutput(t, func() error {
		return ValidateWithOptions(path, Options{Concurrency: 1})
	})
	if runErr == nil || !strings.Contains(runErr.Error(), "cannot lock workspace") {
		t.Fatalf("validation ran inside a locked workspace: %v\n%s", runErr, output)
	}
	if recorded := validateMarkers(t, markers); len(recorded) > 0 {
		t.Fatalf("locked workspace still ran steps: %v", recorded)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	// Releasing the contended lock leaves validation able to acquire it.
	if _, err := captureEnvironmentOutput(t, func() error {
		return ValidateWithOptions(path, Options{Concurrency: 1})
	}); err != nil {
		t.Fatalf("validation could not reacquire the workspace: %v", err)
	}
}

func TestValidateCanceledContext(t *testing.T) {
	root, path, markers := validateFixture(t, validateRecord, validatePass)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := captureEnvironmentOutput(t, func() error {
		return ValidateWithOptions(path, Options{Context: ctx, Concurrency: 1})
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation not propagated: %v", err)
	}
	if recorded := validateMarkers(t, markers); len(recorded) > 0 {
		t.Fatalf("canceled validation ran steps: %v", recorded)
	}
	validateNoDeploymentState(t, root, nil)
}
