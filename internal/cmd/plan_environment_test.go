package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/irevolve/bear/internal"
	"github.com/irevolve/bear/internal/config"
)

func writeEnvironmentFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func environmentGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = root
	out, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func commitEnvironmentFixture(t *testing.T, root string) string {
	t.Helper()
	environmentGit(t, root, "add", ".")
	environmentGit(t, root, "-c", "user.name=Bear Test", "-c", "user.email=bear@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "fixture")
	return environmentGit(t, root, "rev-parse", "HEAD")
}

func pinEnvironmentFixture(t *testing.T, root string) string {
	t.Helper()
	commit := environmentGit(t, root, "rev-parse", "HEAD")
	writeEnvironmentFixture(t, filepath.Join(root, "revision.txt"), "newer source\n")
	commitEnvironmentFixture(t, root)
	return commit
}

func environmentFixture(t *testing.T) (string, string) {
	t.Helper()
	return environmentFixtureWith(t, []string{"dev", "int", "prd"})
}

// environmentFixtureWith builds the shared fixture with an explicit set of
// declared environments, so a test can prove that nothing hardcodes dev/int/prd.
func environmentFixtureWith(t *testing.T, environments []string) (string, string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "bear.config.yml")
	writeEnvironmentFixture(t, path, environmentConfig(environments))
	writeEnvironmentFixture(t, filepath.Join(root, "disabled", "bear.artifact.yml"), "name: disabled\ntarget: local\nenvironments: ["+environments[0]+"]\n")
	writeEnvironmentFixture(t, filepath.Join(root, "allowed", "bear.artifact.yml"), "name: allowed\ntarget: local\nenvironments: ["+strings.Join(environments, ", ")+"]\n")
	writeEnvironmentFixture(t, filepath.Join(root, ".gitignore"), ".bear/\nbear.lock.yml\nvalidated\ndeployed\n")
	t.Setenv("BEAR_TEST_OUTPUT", root)
	environmentGit(t, root, "init")
	commitEnvironmentFixture(t, root)
	return root, path
}

// environmentConfig renders the fixture project config for a declared set.
func environmentConfig(environments []string) string {
	return `name: environment-test
environments: [` + strings.Join(environments, ", ") + `]
languages:
  test:
    detection:
      files: [bear.artifact.yml]
    steps:
      - name: validate
        run: touch "$BEAR_TEST_OUTPUT/$(basename "$PWD")/validated"
targets:
  local:
    steps:
      - name: deploy
        run: touch "$BEAR_TEST_OUTPUT/$(basename "$PWD")/deployed"
`
}

func captureEnvironmentOutput(t *testing.T, run func() error) (string, error) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	previous := os.Stdout
	os.Stdout = f
	defer func() { os.Stdout = previous }()
	runErr := run()
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	return string(data), runErr
}

func TestEnvironmentPlanApply(t *testing.T) {
	for _, mode := range []string{"normal", "pin", "force", "all disabled"} {
		t.Run(mode, func(t *testing.T) {
			root, path := environmentFixture(t)
			previousCommit := environmentGit(t, root, "rev-parse", "HEAD")
			writeEnvironmentFixture(t, filepath.Join(root, "disabled", "source.txt"), "changed source\n")
			commitEnvironmentFixture(t, root)
			opts := Options{Environment: "int", NoCommit: true, Concurrency: 1}
			if mode == "pin" {
				opts.PinCommit = pinEnvironmentFixture(t, root)
			}
			if mode == "force" {
				opts.Force = true
			}
			if mode == "all disabled" {
				opts.Artifacts = []string{"disabled"}
			}
			lockPath := filepath.Join(root, "bear.lock.yml")
			original := config.LockEntry{Commit: previousCommit, Timestamp: "old-time", Target: "local", Pinned: mode == "force"}
			lock := &config.LockFile{Environments: map[string]map[string]config.LockEntry{"int": {"disabled": original}}}
			if err := lock.Save(lockPath); err != nil {
				t.Fatal(err)
			}
			// Every deployment shares one source, so the plan states it once as a
			// header fact aligned under "Environment:", not per artifact.
			sourceLabel, sourceCommit := "Commit", environmentGit(t, root, "rev-parse", "HEAD")
			if mode == "pin" {
				sourceLabel, sourceCommit = "Pinned", opts.PinCommit
			}
			output, err := captureEnvironmentOutput(t, func() error { return PlanWithOptions(path, opts) })
			if err != nil {
				t.Fatal(err)
			}
			wantDeploys, wantChanged := 1, 2
			if mode == "all disabled" {
				wantDeploys, wantChanged = 0, 1
			}
			// Plan brands itself like apply, reports each job live, then closes
			// with a rule, aligned facts, counted sections and one sentence.
			wantText := []string{
				"Bear Plan",
				"Environment: int",
				fmt.Sprintf("%s:      %s", sourceLabel, sourceCommit[:7]),
				"skip (1):",
				"- disabled (disabled): deployment not enabled for environment int",
				fmt.Sprintf("Plan complete: %d changed, %d to deploy, 1 skipped", wantChanged, wantDeploys),
				// "disabled" always changed but never deploys, so it always shows
				// up for review even when it is also skipped.
				"changed (1):",
				"- disabled (disabled): ",
			}
			if wantDeploys > 0 {
				wantText = append(wantText, "deploy (1):", "- allowed (allowed): ", "Run 'bear apply' to execute this plan.")
			}
			for _, text := range wantText {
				if !strings.Contains(output, text) {
					t.Errorf("plan output missing %q: %s", text, output)
				}
			}
			// Plan never executes anything: no validation phase is reported and
			// no step runs, for any artifact, deploying or not.
			for _, gone := range []string{"Validating", "Validation complete", "disabled: Deploying", "allowed: Deploying"} {
				if strings.Contains(output, gone) {
					t.Errorf("plan executed a command (%q): %s", gone, output)
				}
			}
			// The per-artifact "<commit> => <target>" line is gone: an entry is
			// one line, and the source belongs to the header.
			for _, gone := range []string{"=> local", previousCommit[:7] + " =>"} {
				if strings.Contains(output, gone) {
					t.Errorf("plan repeated the source under each artifact (%q): %s", gone, output)
				}
			}
			plan, err := config.ReadPlan(root)
			if err != nil {
				t.Fatal(err)
			}
			if plan.Environment != "int" || plan.ToDeploy != wantDeploys || len(plan.Artifacts) != wantDeploys || plan.Changed != wantChanged || plan.TotalSkips != 1 {
				t.Fatalf("unexpected saved plan: %+v", plan)
			}
			if len(plan.Commit) != 40 || len(plan.SourceFingerprint) != 64 || plan.Pinned != (mode == "pin") {
				t.Fatalf("missing saved source evidence: %+v", plan)
			}
			if mode != "pin" {
				commit, fingerprint, dirty, err := sourceState(context.Background(), root)
				if err != nil || dirty || plan.Commit != commit || plan.SourceFingerprint != fingerprint {
					t.Fatalf("plan does not match clean fixture source: %s %s %v, %v", commit, fingerprint, dirty, err)
				}
			} else if plan.Commit != opts.PinCommit {
				t.Fatalf("plan does not identify pinned source: %+v", plan)
			}
			for _, artifact := range plan.Artifacts {
				if artifact.Path != artifact.Name || artifact.Vars["ENVIRONMENT"] != "int" || len(artifact.BuildSteps) != 1 || len(artifact.Steps) != 1 {
					t.Fatalf("invalid deployment snapshot: %+v", artifact)
				}
			}
			if len(plan.Skipped) != 1 || plan.Skipped[0].Name != "disabled" || plan.Skipped[0].Reason != "deployment not enabled for environment int" {
				t.Fatalf("missing saved skip reason: %+v", plan.Skipped)
			}
			// Plan is a pure decision: it never runs a language or target step.
			if _, err := os.Stat(filepath.Join(root, "disabled", "validated")); !os.IsNotExist(err) {
				t.Fatalf("plan executed the disabled artifact's language step: %v", err)
			}
			if wantDeploys > 0 {
				if _, err := os.Stat(filepath.Join(root, "allowed", "validated")); !os.IsNotExist(err) {
					t.Fatalf("plan executed the deploying artifact's language step: %v", err)
				}
			}
			// Pins consume the old snapshot; normal plans must reject changed source.
			writeEnvironmentFixture(t, filepath.Join(root, "disabled", "bear.artifact.yml"), "name: disabled\ntarget: local\nenvironments: [int]\n")
			writeEnvironmentFixture(t, filepath.Join(root, "allowed", "bear.artifact.yml"), "name: allowed\ntarget: local\nenvironments: []\n")
			if mode != "pin" {
				if wantDeploys > 0 {
					if err := ApplyWithOptions(path, opts); err == nil || !strings.Contains(err.Error(), "fingerprint") {
						t.Fatalf("expected changed-source rejection, got %v", err)
					}
					if _, err := os.Stat(filepath.Join(root, "allowed", "deployed")); !os.IsNotExist(err) {
						t.Fatalf("changed source deployed: %v", err)
					}
				}
				writeEnvironmentFixture(t, filepath.Join(root, "disabled", "bear.artifact.yml"), "name: disabled\ntarget: local\nenvironments: [dev]\n")
				writeEnvironmentFixture(t, filepath.Join(root, "allowed", "bear.artifact.yml"), "name: allowed\ntarget: local\nenvironments: [dev, int, prd]\n")
			}
			if wantDeploys > 0 && !reflect.DeepEqual(plan.Artifacts[0].Environments, []string{"dev", "int", "prd"}) {
				t.Fatalf("allowlist not snapshotted: %+v", plan.Artifacts[0])
			}
			output, err = captureEnvironmentOutput(t, func() error {
				return ApplyWithOptions(path, Options{Environment: "dev", NoCommit: true, Concurrency: 1})
			})
			if err != nil {
				t.Fatal(err)
			}
			// Apply acts on the approved snapshot, not on the current (now
			// permissive) configuration. The proof is the workspace and the lock
			// file rather than a recap: the disabled artifact is never run and its
			// history is untouched.
			if _, err := os.Stat(filepath.Join(root, "disabled", "deployed")); !os.IsNotExist(err) {
				t.Fatalf("disabled deployment executed: %v", err)
			}
			lock, err = config.LoadLock(lockPath)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(lock.Environments["int"]["disabled"], original) {
				t.Errorf("disabled lock entry changed: %+v", lock.Environments["int"]["disabled"])
			}
			if len(lock.Environments) != 1 || len(lock.Environments["int"]) != 1+wantDeploys {
				t.Errorf("apply wrote history for a skipped artifact: %+v", lock.Environments)
			}
			// The approved plan already listed every deployment and skip, so a
			// successful apply repeats none of it.
			for _, recap := range []string{summaryRule, "deploy (", "skip (", "failed (", "Environment: int"} {
				if strings.Contains(output, recap) {
					t.Errorf("successful apply recapped the plan with %q: %s", recap, output)
				}
			}
			if wantDeploys > 0 {
				for _, text := range []string{
					"Bear Apply",
					// The heading names the environment being deployed to.
					"Deploying 1 artifact to int",
					// Apply runs the language's build step, then the target's
					// deploy step, as one continuous numbered sequence.
					"allowed: Deploying... [1/2 validate]",
					"allowed: Deploying... [2/2 deploy]",
					"allowed: Deployment complete after ",
					"Apply complete: 1 deployed, 1 skipped in ",
				} {
					if !strings.Contains(output, text) {
						t.Errorf("apply output missing %q: %s", text, output)
					}
				}
				if _, err := os.Stat(filepath.Join(root, "allowed", "deployed")); err != nil {
					t.Fatalf("allowed deployment did not execute: %v", err)
				}
				// The language's build step ran too, before the deploy step.
				buildInfo, buildErr := os.Stat(filepath.Join(root, "allowed", "validated"))
				deployInfo, deployErr := os.Stat(filepath.Join(root, "allowed", "deployed"))
				if buildErr != nil || deployErr != nil {
					t.Fatalf("apply did not run both build and deploy steps: %v, %v", buildErr, deployErr)
				}
				if buildInfo.ModTime().After(deployInfo.ModTime()) {
					t.Errorf("build step ran after deploy step: build=%v deploy=%v", buildInfo.ModTime(), deployInfo.ModTime())
				}
				entry, ok := lock.GetArtifact("int", "allowed")
				if !ok || entry.Pinned != (mode == "pin") || entry.Commit != plan.Commit {
					t.Errorf("allowed deployment not recorded correctly: %+v", entry)
				}
			} else if output != "Plan for int contains no artifacts to deploy.\n" {
				// An empty plan says so, names its environment and stops there.
				t.Errorf("empty-plan apply printed %q", output)
			}
			if config.PlanExists(root) {
				t.Error("apply did not consume plan")
			}
		})
	}
}

func TestPlanningClearsStalePlan(t *testing.T) {
	for _, scenario := range []struct{ name, wantError string }{
		{name: "missing environment", wantError: `invalid environment name ""`},
		{name: "malformed environment", wantError: `invalid environment name "PRD"`},
		{name: "undeclared environment", wantError: `unknown environment "production": bear.config.yml declares dev, int, prd`},
		{name: "undeclared policy", wantError: `artifact "disabled" allows undeclared environment "production"; bear.config.yml declares dev, int, prd`},
		{name: "legacy policy"},
		{name: "invalid config"},
		{name: "missing config"},
		{name: "empty selection", wantError: `unknown artifact "nonexistent"`},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root, path := environmentFixture(t)
			opts := Options{Environment: "int", NoCommit: true, Concurrency: 1}
			switch scenario.name {
			case "missing environment":
				opts.Environment = ""
				t.Setenv("ENVIRONMENT", "int")
				t.Setenv("BEAR_ENVIRONMENT", "int")
			case "malformed environment":
				opts.Environment = "PRD"
			case "undeclared environment":
				opts.Environment = "production"
			case "undeclared policy":
				writeEnvironmentFixture(t, filepath.Join(root, "disabled", "bear.artifact.yml"), "name: disabled\ntarget: local\nenvironments: [production]\n")
			case "legacy policy":
				writeEnvironmentFixture(t, filepath.Join(root, "disabled", "bear.artifact.yml"), "name: disabled\ntarget: local\ndisabled_environments: []\n")
			case "invalid config":
				writeEnvironmentFixture(t, path, "invalid: [")
			case "missing config":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "empty selection":
				opts.Artifacts = []string{"nonexistent"}
			}
			switch scenario.name {
			case "undeclared policy", "legacy policy", "invalid config", "missing config":
				commitEnvironmentFixture(t, root)
			}
			stale := config.NewPlanFile("stale")
			stale.Artifacts = []config.PlanArtifact{{Name: "stale", Path: ".", Steps: []config.Step{{Name: "deploy", Run: "touch stale-deployed"}}}}
			if err := config.WritePlan(root, stale); err != nil {
				t.Fatal(err)
			}
			_, err := captureEnvironmentOutput(t, func() error { return PlanWithOptions(path, opts) })
			if err == nil {
				t.Fatal("expected planning error")
			}
			if scenario.wantError != "" && !strings.Contains(err.Error(), scenario.wantError) {
				t.Fatalf("expected %q, got %v", scenario.wantError, err)
			}
			if config.PlanExists(root) {
				t.Fatal("stale plan survived")
			}
			if err := ApplyWithOptions(path, Options{NoCommit: true}); err == nil {
				t.Fatal("apply accepted a stale plan")
			}
			if _, err := os.Stat(filepath.Join(root, "stale-deployed")); !os.IsNotExist(err) {
				t.Fatalf("stale deployment executed: %v", err)
			}
		})
	}
}

func TestPlanningFailsWhenStalePlanCannotBeRemoved(t *testing.T) {
	root, path := environmentFixture(t)
	// A nonempty directory is reliably unremovable even when tests run as root.
	writeEnvironmentFixture(t, filepath.Join(config.PlanFilePath(root), "blocked"), "blocked")
	err := PlanWithOptions(path, Options{Environment: "int", NoCommit: true})
	if err == nil || !strings.Contains(err.Error(), "error removing previous plan") {
		t.Fatalf("expected cleanup error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "disabled", "validated")); !os.IsNotExist(err) {
		t.Fatalf("validation ran after cleanup failed: %v", err)
	}
}

// TestPlanRejectsUnknownArtifactWithoutWritingPlan asserts that a positional
// filter matching nothing fails plan the same way it fails validate, in every
// mode, and leaves no plan file behind (there was none to begin with).
func TestPlanRejectsUnknownArtifactWithoutWritingPlan(t *testing.T) {
	for _, tt := range []struct {
		name      string
		artifacts []string
		pin       bool
		force     bool
		wantError string
	}{
		{name: "unmatched name alone", artifacts: []string{"missing"}, wantError: `unknown artifact "missing"`},
		{name: "unmatched name mixed with a valid one", artifacts: []string{"allowed", "missing"}, wantError: `unknown artifact "missing"`},
		{name: "unmatched name with pin", artifacts: []string{"missing"}, pin: true, wantError: `unknown artifact "missing"`},
		{name: "unmatched name with force", artifacts: []string{"missing"}, force: true, wantError: `unknown artifact "missing"`},
		{name: "multiple unmatched names are sorted and deduplicated", artifacts: []string{"zeta", "alpha", "zeta"}, wantError: `unknown artifact "alpha", "zeta"`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root, path := environmentFixture(t)
			opts := Options{Environment: "int", Artifacts: tt.artifacts, Force: tt.force, NoCommit: true, Concurrency: 1}
			if tt.pin {
				opts.PinCommit = pinEnvironmentFixture(t, root)
			}
			if config.PlanExists(root) {
				t.Fatal("fixture unexpectedly starts with a plan")
			}
			err := PlanWithOptions(path, opts)
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("expected %q, got %v", tt.wantError, err)
			}
			if config.PlanExists(root) {
				t.Fatal("rejecting an unknown artifact wrote a plan file")
			}
		})
	}
}

func TestEnvironmentDependentPlanApply(t *testing.T) {
	root, path := environmentFixture(t)
	previousCommit := environmentGit(t, root, "rev-parse", "HEAD")
	writeEnvironmentFixture(t, filepath.Join(root, "source", "bear.artifact.yml"), "name: source\ntarget: local\nenvironments: [dev]\n")
	writeEnvironmentFixture(t, filepath.Join(root, "disabled", "bear.artifact.yml"), "name: disabled\ntarget: local\ndepends: [source]\nenvironments: [dev]\n")
	writeEnvironmentFixture(t, filepath.Join(root, "allowed", "bear.artifact.yml"), "name: allowed\ntarget: local\ndepends: [disabled]\nenvironments: [int]\n")
	commit := commitEnvironmentFixture(t, root)
	if commit == "" {
		t.Fatal("fixture has no commit")
	}
	lock := &config.LockFile{Environments: map[string]map[string]config.LockEntry{"int": {
		"disabled": {Commit: previousCommit, Target: "local"},
		"allowed":  {Commit: previousCommit, Target: "local"},
	}}}
	if err := lock.Save(filepath.Join(root, "bear.lock.yml")); err != nil {
		t.Fatal(err)
	}
	opts := Options{Environment: "int", NoCommit: true, Concurrency: 1}
	if _, err := captureEnvironmentOutput(t, func() error { return PlanWithOptions(path, opts) }); err != nil {
		t.Fatal(err)
	}
	plan, err := config.ReadPlan(root)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Changed != 3 || plan.ToDeploy != 1 || plan.TotalSkips != 2 || len(plan.Artifacts) != 1 || plan.Artifacts[0].Name != "allowed" || !strings.Contains(plan.Artifacts[0].Reason, "dependency") {
		t.Fatalf("transitive environment plan: %+v", plan)
	}
	if _, err := captureEnvironmentOutput(t, func() error { return ApplyWithOptions(path, opts) }); err != nil {
		t.Fatal(err)
	}
	// Only "allowed" deploys. Its own language build step runs, but nothing
	// ever runs for its dependencies: a changed dependency selects "allowed"
	// for deployment, it does not run "source" or "disabled" commands.
	for _, name := range []string{"source", "disabled", "allowed"} {
		_, err := os.Stat(filepath.Join(root, name, "validated"))
		if name == "allowed" && err != nil || name != "allowed" && !os.IsNotExist(err) {
			t.Errorf("unexpected build step state for %s: %v", name, err)
		}
		_, err = os.Stat(filepath.Join(root, name, "deployed"))
		if name == "allowed" && err != nil || name != "allowed" && !os.IsNotExist(err) {
			t.Errorf("unexpected deployment state for %s: %v", name, err)
		}
	}
	updated, err := config.LoadLock(filepath.Join(root, "bear.lock.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := updated.GetArtifact("int", "source"); ok {
		t.Error("disabled new source recorded in lock")
	}
	if updated.Environments["int"]["disabled"] != lock.Environments["int"]["disabled"] {
		t.Error("disabled dependent lock entry changed")
	}
	if internal.GetCurrentCommit(root) != commit {
		t.Error("NoCommit changed fixture HEAD")
	}
	// With every artifact marked deployed at HEAD, an empty plan removes the saved plan.
	updated.UpdateArtifact("int", "source", commit, "local", "")
	updated.UpdateArtifact("int", "disabled", commit, "local", "")
	if err := updated.Save(filepath.Join(root, "bear.lock.yml")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "allowed", "validated")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "allowed", "deployed")); err != nil {
		t.Fatal(err)
	}
	if err := config.WritePlan(root, plan); err != nil {
		t.Fatal(err)
	}
	if err := PlanWithOptions(path, Options{NoCommit: true}); err == nil || !strings.Contains(err.Error(), "invalid environment") || config.PlanExists(root) {
		t.Fatalf("unchanged policy must still require an environment and clear stale plan: %v", err)
	}
	if err := config.WritePlan(root, plan); err != nil {
		t.Fatal(err)
	}
	output, err := captureEnvironmentOutput(t, func() error { return PlanWithOptions(path, opts) })
	if err != nil || config.PlanExists(root) {
		t.Fatalf("unchanged plan retained stale file: %v\n%s", err, output)
	}
	// A plan with nothing to do still names its environment and still says, per
	// artifact and with its path, why nothing happens.
	for _, text := range []string{"Environment: int", "skip (3):",
		"  - allowed (allowed): no changes detected",
		"  - disabled (disabled): no changes detected",
		"  - source (source): no changes detected",
	} {
		if !strings.Contains(output, text) {
			t.Errorf("empty plan hid %q: %s", text, output)
		}
	}
}

func TestSelectionWithoutPolicy(t *testing.T) {
	root, path := environmentFixture(t)
	writeEnvironmentFixture(t, filepath.Join(root, "allowed", "bear.artifact.yml"), "name: allowed\ntarget: local\n")
	commitEnvironmentFixture(t, root)
	if _, err := captureEnvironmentOutput(t, func() error {
		return PlanWithOptions(path, Options{Environment: "int", Artifacts: []string{"allowed"}, NoCommit: true, Concurrency: 1})
	}); err != nil {
		t.Fatal(err)
	}
	plan, err := config.ReadPlan(root)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Environment != "int" || len(plan.Artifacts) != 0 || plan.Changed != 1 || plan.ToDeploy != 0 || len(plan.Skipped) != 1 || plan.Skipped[0].Reason != "no deployment environments configured" {
		t.Fatalf("selection without policy regressed: %+v", plan)
	}
}

func TestDefaultDenyPlanApply(t *testing.T) {
	for _, policy := range []string{"", "environments: []\n"} {
		for _, environment := range []string{"dev", "int", "prd"} {
			for _, mode := range []string{"normal", "pin", "force", "force pin"} {
				t.Run(policy+"/"+environment+"/"+mode, func(t *testing.T) {
					root, path := environmentFixture(t)
					previousCommit := environmentGit(t, root, "rev-parse", "HEAD")
					writeEnvironmentFixture(t, filepath.Join(root, "disabled", "bear.artifact.yml"), "name: disabled\ntarget: local\n"+policy)
					commitEnvironmentFixture(t, root)
					opts := Options{Environment: environment, Artifacts: []string{"disabled"}, NoCommit: true, Concurrency: 1}
					if strings.Contains(mode, "pin") {
						opts.PinCommit = pinEnvironmentFixture(t, root)
					}
					opts.Force = strings.Contains(mode, "force")
					lockPath := filepath.Join(root, "bear.lock.yml")
					lock := &config.LockFile{Environments: map[string]map[string]config.LockEntry{environment: {"disabled": {Commit: previousCommit, Pinned: opts.Force}}}}
					if err := lock.Save(lockPath); err != nil {
						t.Fatal(err)
					}
					if _, err := captureEnvironmentOutput(t, func() error { return PlanWithOptions(path, opts) }); err != nil {
						t.Fatal(err)
					}
					plan, err := config.ReadPlan(root)
					if err != nil {
						t.Fatal(err)
					}
					if plan.Environment != environment || plan.Changed != 1 || plan.ToDeploy != 0 || len(plan.Artifacts) != 0 || plan.TotalSkips != 1 || len(plan.Skipped) != 1 || plan.Skipped[0].Reason != "no deployment environments configured" {
						t.Fatalf("default deny plan: %+v", plan)
					}
					if _, err := os.Stat(filepath.Join(root, "disabled", "validated")); !os.IsNotExist(err) {
						t.Fatalf("plan executed a language step: %v", err)
					}
					if err := ApplyWithOptions(path, opts); err != nil {
						t.Fatal(err)
					}
					if _, err := os.Stat(filepath.Join(root, "disabled", "deployed")); !os.IsNotExist(err) {
						t.Fatalf("default deny deployed: %v", err)
					}
					if _, err := os.Stat(filepath.Join(root, "disabled", "validated")); !os.IsNotExist(err) {
						t.Fatalf("apply ran a build step for an artifact with nothing to deploy: %v", err)
					}
					updated, err := config.LoadLock(lockPath)
					if err != nil || !reflect.DeepEqual(updated, lock) {
						t.Fatalf("default deny modified lock: %+v, %v", updated, err)
					}
				})
			}
		}
	}
}

func TestEnvironmentJobVarsPlanApply(t *testing.T) {
	for _, selection := range []struct{ environment, policy string }{
		{environment: "int", policy: "environments: [int]\n"},
		{environment: "int"},
		{environment: "int", policy: "environments: []\n"},
	} {
		selected := selection.environment
		for _, source := range []struct {
			name, languageVars, targetVars, artifactVars string
		}{
			{name: "OS"},
			{name: "language", languageVars: "      ENVIRONMENT: prd\n"},
			{name: "target", languageVars: "      ENVIRONMENT: prd\n", targetVars: "      ENVIRONMENT: dev\n"},
			{name: "artifact", languageVars: "      ENVIRONMENT: prd\n", targetVars: "      ENVIRONMENT: dev\n", artifactVars: "  ENVIRONMENT: prd\n"},
		} {
			t.Run("selected="+selected+"/"+selection.policy+"/"+source.name, func(t *testing.T) {
				root, path := environmentFixture(t)
				t.Setenv("ENVIRONMENT", "dev")
				cfg := fmt.Sprintf(`name: environment-test
environments: [dev, int, prd]
languages:
  test:
    detection:
      files: [bear.artifact.yml]
    vars:
%s      LANGUAGE_REF: language-${ENVIRONMENT}
    steps:
      - name: validate
        run: printf '%%s\n' "$ENVIRONMENT|$LANGUAGE_REF|$TARGET_REF|$ARTIFACT_REF|$NAME|$VERSION" > "$BEAR_TEST_OUTPUT/$(basename "$PWD")/validated"
targets:
  local:
    vars:
%s      TARGET_REF: target-${ENVIRONMENT}
    steps:
      - name: deploy
        run: printf '%%s\n' "$ENVIRONMENT|$LANGUAGE_REF|$TARGET_REF|$ARTIFACT_REF|$NAME|$VERSION" > "$BEAR_TEST_OUTPUT/$(basename "$PWD")/deployed"
`, source.languageVars, source.targetVars)
				writeEnvironmentFixture(t, path, cfg)
				for _, name := range []string{"allowed", "disabled"} {
					artifact := "name: " + name + "\ntarget: local\nvars:\n" + source.artifactVars + "  ARTIFACT_REF: artifact-${ENVIRONMENT}\n  NAME: configured-name\n  VERSION: configured-version\n"
					if name == "disabled" {
						artifact += "environments: [dev]\n"
					} else {
						artifact += selection.policy
					}
					writeEnvironmentFixture(t, filepath.Join(root, name, "bear.artifact.yml"), artifact)
				}
				commitEnvironmentFixture(t, root)
				opts := Options{Environment: selected, PinCommit: pinEnvironmentFixture(t, root), NoCommit: true, Concurrency: 1}
				// Neither inherited nor configured job variables can select an environment,
				// even when the selected artifact has no deployment policy.
				if err := config.WritePlan(root, config.NewPlanFile("stale")); err != nil {
					t.Fatal(err)
				}
				missing := opts
				missing.Environment = ""
				missing.Artifacts = []string{"allowed"}
				if err := PlanWithOptions(path, missing); err == nil || !strings.Contains(err.Error(), "invalid environment") || config.PlanExists(root) {
					t.Fatalf("configured/inherited ENVIRONMENT selected environment: %v", err)
				}
				if output, err := captureEnvironmentOutput(t, func() error { return PlanWithOptions(path, opts) }); err != nil {
					t.Fatalf("plan: %v\n%s", err, output)
				}
				wantRefs := fmt.Sprintf("%s|language-%s|target-%s|artifact-%s|", selected, selected, selected, selected)
				// Plan never runs a command: neither artifact has a build-step
				// marker yet, whether or not it will ultimately deploy.
				for _, name := range []string{"allowed", "disabled"} {
					if _, err := os.Stat(filepath.Join(root, name, "validated")); !os.IsNotExist(err) {
						t.Errorf("plan executed %s's build step: %v", name, err)
					}
				}
				plan, err := config.ReadPlan(root)
				if err != nil {
					t.Fatal(err)
				}
				if selection.policy != "environments: [int]\n" {
					if plan.Environment != selected || len(plan.Artifacts) != 0 || plan.Changed != 2 || plan.TotalSkips != 2 {
						t.Fatalf("expected validation-only plan: %+v", plan)
					}
					if err := ApplyWithOptions(path, Options{NoCommit: true}); err != nil {
						t.Fatal(err)
					}
					for _, name := range []string{"allowed", "disabled"} {
						if _, err := os.Stat(filepath.Join(root, name, "validated")); !os.IsNotExist(err) {
							t.Errorf("apply ran %s's build step though nothing deploys: %v", name, err)
						}
					}
					if _, err := os.Stat(filepath.Join(root, "allowed", "deployed")); !os.IsNotExist(err) {
						t.Fatalf("validation-only plan deployed: %v", err)
					}
					return
				}
				if plan.Environment != selected || len(plan.Artifacts) != 1 || plan.Artifacts[0].Name != "allowed" || !reflect.DeepEqual(plan.Artifacts[0].Environments, []string{"int"}) {
					t.Fatalf("unexpected plan: %+v", plan)
				}
				value, present := plan.Artifacts[0].Vars["ENVIRONMENT"]
				if !present || value != selected {
					t.Fatalf("saved ENVIRONMENT = %q, want %q", value, selected)
				}
				if len(plan.Artifacts[0].BuildSteps) != 1 {
					t.Fatalf("build steps not saved: %+v", plan.Artifacts[0])
				}
				t.Setenv("ENVIRONMENT", "prd")
				writeEnvironmentFixture(t, path, `name: environment-test
environments: [dev, int, prd]
languages:
  test:
    steps: []
    vars:
      ENVIRONMENT: prd
targets:
  local:
    vars:
      ENVIRONMENT: prd
    steps:
      - name: changed-deploy
        run: exit 1
`)
				writeEnvironmentFixture(t, filepath.Join(root, "allowed", "bear.artifact.yml"), "name: allowed\ntarget: local\nvars:\n  ENVIRONMENT: prd\n")
				if output, err := captureEnvironmentOutput(t, func() error {
					return ApplyWithOptions(path, Options{Environment: "prd", NoCommit: true, Concurrency: 1})
				}); err != nil {
					t.Fatalf("apply: %v\n%s", err, output)
				}
				data, err := os.ReadFile(filepath.Join(root, "allowed", "deployed"))
				if err != nil || string(data) != wantRefs+"allowed|"+opts.PinCommit[:7]+"\n" {
					t.Errorf("deployment vars: %q, error: %v", data, err)
				}
				// The saved build step ran too, using the same approved vars, even
				// though the language's current (reconfigured) steps say nothing.
				buildData, err := os.ReadFile(filepath.Join(root, "allowed", "validated"))
				if err != nil || string(buildData) != wantRefs+"allowed|"+opts.PinCommit[:7]+"\n" {
					t.Errorf("build step vars: %q, error: %v", buildData, err)
				}
				if _, err := os.Stat(filepath.Join(root, "disabled", "deployed")); !os.IsNotExist(err) {
					t.Errorf("disabled deployment executed: %v", err)
				}
				if _, err := os.Stat(filepath.Join(root, "disabled", "validated")); !os.IsNotExist(err) {
					t.Errorf("disabled build step executed: %v", err)
				}
			})
		}
	}
}
