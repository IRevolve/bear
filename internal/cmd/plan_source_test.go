package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/irevolve/bear/internal/config"
)

func planSourceConfig(step string) string {
	return fmt.Sprintf(`name: source-plan
languages:
  test:
    detection:
      files: [bear.artifact.yml]
    vars:
      CHOICE: language
      LANGUAGE_REF: language-${ENVIRONMENT}
    steps:
      - name: inspect-source
        run: %q
targets:
  local:
    vars:
      CHOICE: target
    steps:
      - name: deploy
        run: test -f generated
`, step)
}

func planSourceFixture(t *testing.T, step string) (repo, root, path, commit string) {
	t.Helper()
	repo = t.TempDir()
	root = filepath.Join(repo, "workspace")
	path = filepath.Join(root, "bear.config.yml")
	sourceTestGit(t, repo, "init")
	sourceTestWrite(t, root, "bear.config.yml", planSourceConfig(step))
	sourceTestWrite(t, root, "app/bear.artifact.yml", "name: app\ntarget: local\nenvironments: [int]\nvars:\n  CHOICE: artifact\n  ENVIRONMENT: prd\n")
	sourceTestWrite(t, root, "app/source.txt", "A\n")
	commit = sourceTestCommit(t, repo)
	return
}

func TestPlanSourceNormalSnapshot(t *testing.T) {
	repo, root, path, commit := planSourceFixture(t, `test "$ENVIRONMENT|$LANGUAGE_REF|$CHOICE" = 'int|language-int|artifact' && cat source.txt > generated`)
	_, before, _ := sourceTestState(t, root)
	// Bear's own tracked and untracked state never makes source dirty.
	sourceTestWrite(t, root, ".bear/state", "local state\n")
	sourceTestWrite(t, root, "bear.lock.yml", "artifacts:\n  legacy:\n    commit: old\n")
	output, err := captureEnvironmentOutput(t, func() error {
		return PlanWithOptions(path, Options{Environment: "int", Concurrency: 1})
	})
	if err != nil {
		t.Fatalf("plan: %v\n%s", err, output)
	}
	if !strings.Contains(output, "legacy") || !strings.Contains(output, "will not be migrated") {
		t.Fatalf("missing legacy warning: %s", output)
	}
	plan, err := config.ReadPlan(root)
	if err != nil {
		t.Fatal(err)
	}
	head, fingerprint, dirty := sourceTestState(t, root)
	if plan.Commit != commit || head != commit || plan.SourceFingerprint != fingerprint || before == fingerprint || !dirty || plan.Pinned {
		t.Fatalf("incorrect source snapshot: %+v; HEAD=%s fingerprint=%s dirty=%v", plan, head, fingerprint, dirty)
	}
	if len(plan.Artifacts) != 1 || len(plan.Validations) != 1 || plan.Validated != 1 || plan.ToDeploy != 1 {
		t.Fatalf("incorrect actions: %+v", plan)
	}
	artifact, validation := plan.Artifacts[0], plan.Validations[0]
	if artifact.Path != "app" || validation.Path != "app" || artifact.Completed || artifact.Pinned || artifact.Vars["VERSION"] != commit[:7] {
		t.Fatalf("incorrect artifact snapshot: %+v; validation: %+v", artifact, validation)
	}
	if validation.Name != "app" || validation.Vars["ENVIRONMENT"] != "int" || validation.Vars["CHOICE"] != "artifact" || validation.Vars["LANGUAGE_REF"] != "language-${ENVIRONMENT}" || len(validation.Steps) != 1 {
		t.Fatalf("incorrect replay snapshot: %+v", validation)
	}
	data, err := os.ReadFile(config.PlanFilePath(root))
	if err != nil || strings.Contains(string(data), repo) || strings.Contains(string(data), "bear-source-") {
		t.Fatalf("nonportable plan: %s; %v", data, err)
	}
}

func TestPlanSourcePinUsesCurrentPolicyAndPinnedFiles(t *testing.T) {
	repo, root, path, _ := planSourceFixture(t, "exit 91")
	// A's validation is deliberately unusable, and its policy allows deployment.
	// B supplies the actual validation and denies deployment for a second artifact.
	sourceTestWrite(t, root, "blocked/bear.artifact.yml", "name: blocked\ntarget: local\nenvironments: [int]\nvars:\n  CHOICE: artifact\n")
	sourceTestWrite(t, root, "blocked/source.txt", "A\n")
	a := sourceTestCommit(t, repo)
	sourceTestGit(t, repo, "tag", "execution-a", a)
	sourceTestWrite(t, root, "app/source.txt", "B\n")
	sourceTestWrite(t, root, "blocked/bear.artifact.yml", "name: blocked\ntarget: local\nenvironments: [dev]\nvars:\n  CHOICE: artifact\n")
	sourceTestWrite(t, root, "bear.config.yml", planSourceConfig(`test "$(cat source.txt)" = A && test "$ENVIRONMENT|$CHOICE" = 'int|artifact' && cat source.txt > generated`))
	b := sourceTestCommit(t, repo)
	sourceTestWrite(t, root, "app/source.txt", "dirty B\n")
	beforeStatus := sourceTestGit(t, repo, "status", "--porcelain", "--untracked-files=no")
	if err := PlanWithOptions(path, Options{Environment: "int", PinCommit: "execution-a", Concurrency: 2}); err != nil {
		t.Fatal(err)
	}
	plan, err := config.ReadPlan(root)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Commit != a || !plan.Pinned || plan.Validated != 2 || len(plan.Validations) != 2 || len(plan.Artifacts) != 1 || len(plan.Skipped) != 1 || plan.Skipped[0].Name != "blocked" {
		t.Fatalf("incorrect pinned snapshot: %+v", plan)
	}
	artifact := plan.Artifacts[0]
	if artifact.Name != "app" || artifact.Path != "app" || artifact.PinCommit != a || !artifact.Pinned || artifact.Vars["VERSION"] != a[:7] || !reflect.DeepEqual(artifact.Environments, []string{"int"}) {
		t.Fatalf("incorrect pinned deployment: %+v", artifact)
	}
	if sourceTestGit(t, repo, "rev-parse", "HEAD") != b || sourceTestGit(t, repo, "status", "--porcelain", "--untracked-files=no") != beforeStatus {
		t.Fatal("pin modified caller's HEAD or tracked files")
	}
	for _, name := range []string{"app", "blocked"} {
		if _, err := os.Stat(filepath.Join(root, name, "generated")); !os.IsNotExist(err) {
			t.Fatalf("validation leaked to current workspace: %s: %v", name, err)
		}
	}
	if count := strings.Count(sourceTestGit(t, repo, "worktree", "list", "--porcelain"), "worktree "); count != 1 {
		t.Fatalf("plan leaked private worktree: %d worktrees", count)
	}
	// Replay the saved validation on a fresh checkout, exactly as apply will.
	snapshot, _, cleanup, err := prepareSource(context.Background(), root, plan.Commit)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Error(err)
		}
	})
	for _, validation := range plan.Validations {
		if validation.Path != validation.Name || validation.Vars["ENVIRONMENT"] != "int" {
			t.Fatalf("nonportable validation: %+v", validation)
		}
		dir, err := safeArtifactPath(snapshot, validation.Path)
		if err != nil {
			t.Fatal(err)
		}
		for _, step := range validation.Steps {
			if err := ExecuteStep(context.Background(), step.Run, dir, validation.Vars, io.Discard, io.Discard); err != nil {
				t.Fatal(err)
			}
		}
	}
	_, fingerprint, _ := sourceTestState(t, snapshot)
	if fingerprint != plan.SourceFingerprint {
		t.Fatalf("replay fingerprint %s differs from planned %s", fingerprint, plan.SourceFingerprint)
	}
}

func TestPlanSourceRejectsUnsafeChanges(t *testing.T) {
	for _, scenario := range []string{"dirty", "tracked validation edit", "HEAD validation change", "no git", "no git empty selection"} {
		t.Run(scenario, func(t *testing.T) {
			step := "touch generated"
			if scenario == "tracked validation edit" {
				step = "printf changed > source.txt"
			}
			if scenario == "HEAD validation change" {
				step = "git -c user.name=Test -c user.email=test@example.invalid -c commit.gpgsign=false commit --allow-empty -m changed"
			}
			repo, root, path, _ := planSourceFixture(t, step)
			opts := Options{Environment: "int", Concurrency: 1}
			if scenario == "dirty" {
				sourceTestWrite(t, root, "app/source.txt", "dirty\n")
			}
			if strings.HasPrefix(scenario, "no git") {
				if err := os.RemoveAll(filepath.Join(repo, ".git")); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "no git empty selection" {
				opts.Artifacts = []string{"missing"}
			}
			if err := config.WritePlan(root, config.NewPlanFile("stale")); err != nil {
				t.Fatal(err)
			}
			err := PlanWithOptions(path, opts)
			if err == nil || config.PlanExists(root) {
				t.Fatalf("unsafe source accepted or stale plan survived: %v", err)
			}
			if scenario == "dirty" || strings.HasPrefix(scenario, "no git") {
				if _, err := os.Stat(filepath.Join(root, "app/generated")); !os.IsNotExist(err) {
					t.Fatalf("validation ran before rejecting source: %v", err)
				}
			}
		})
	}
}

func TestPlanSourceValidationOnlyAllowsInitialDirty(t *testing.T) {
	_, root, path, commit := planSourceFixture(t, "cat source.txt > generated")
	sourceTestWrite(t, root, "app/bear.artifact.yml", "name: app\ntarget: local\nenvironments: [dev]\n")
	sourceTestWrite(t, root, "app/source.txt", "dirty\n")
	if err := PlanWithOptions(path, Options{Environment: "int"}); err != nil {
		t.Fatal(err)
	}
	plan, err := config.ReadPlan(root)
	if err != nil {
		t.Fatal(err)
	}
	_, fingerprint, _ := sourceTestState(t, root)
	if plan.Commit != commit || plan.SourceFingerprint != fingerprint || plan.ToDeploy != 0 || len(plan.Validations) != 1 || plan.Validations[0].Path != "app" {
		t.Fatalf("incorrect validation-only plan: %+v", plan)
	}
}

func TestPlanSourceLockBeforeRemovingPlan(t *testing.T) {
	_, root, path, _ := planSourceFixture(t, "touch generated")
	if err := config.WritePlan(root, config.NewPlanFile("stale")); err != nil {
		t.Fatal(err)
	}
	release, err := acquireWorkspaceLock(root)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := PlanWithOptions(path, Options{Environment: "int"}); err == nil || !config.PlanExists(root) {
		t.Fatalf("lock did not protect saved plan: %v", err)
	}
}

func TestPlanSourceContextAndStepErrors(t *testing.T) {
	t.Run("canceled context", func(t *testing.T) {
		_, root, path, _ := planSourceFixture(t, "touch generated")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := PlanWithOptions(path, Options{Environment: "int", Context: ctx})
		if !errors.Is(err, context.Canceled) || config.PlanExists(root) {
			t.Fatalf("cancellation not propagated: %v", err)
		}
	})
	t.Run("verbose failure", func(t *testing.T) {
		_, root, path, _ := planSourceFixture(t, "printf stdout; printf stderr >&2; exit 7")
		output, err := captureEnvironmentOutput(t, func() error {
			return PlanWithOptions(path, Options{Environment: "int", Verbose: true})
		})
		if err == nil || !strings.Contains(err.Error(), "inspect-source") || config.PlanExists(root) {
			t.Fatalf("missing step error: %v", err)
		}
		for _, text := range []string{"app | inspect-source |", "stdout", "stderr"} {
			if !strings.Contains(output, text) {
				t.Errorf("missing %q in streamed output: %s", text, output)
			}
		}
	})
}
