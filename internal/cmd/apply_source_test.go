package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/irevolve/bear/internal/config"
)

func applySourceFixture(t *testing.T, commands ...string) (string, *config.PlanFile) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("lifecycle fixtures use POSIX shell commands")
	}
	root := t.TempDir()
	sourceTestGit(t, root, "init")
	sourceTestWrite(t, root, ".gitignore", ".bear/\nnode_modules/\n")
	sourceTestWrite(t, root, "app/source", "approved\n")
	commit := sourceTestCommit(t, root)
	_, fingerprint, _ := sourceTestState(t, root)
	plan := config.NewPlanFile(commit)
	plan.Environment = "int"
	plan.SourceFingerprint = fingerprint
	for i, command := range commands {
		name := string(rune('a' + i))
		plan.Artifacts = append(plan.Artifacts, config.PlanArtifact{
			Name: name, Path: ".", Action: "deploy", Target: "saved-target",
			Environments: []string{"int"}, Vars: map[string]string{"ENVIRONMENT": "int", "NAME": name},
			Steps: []config.Step{{Name: "deploy", Run: command}},
		})
	}
	plan.ToDeploy = len(plan.Artifacts)
	return root, plan
}

func applySourceSave(t *testing.T, root string, plan *config.PlanFile) {
	t.Helper()
	if err := config.WritePlan(root, plan); err != nil {
		t.Fatal(err)
	}
}

func applySourceRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func applySourceAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("unexpected path %s: %v", path, err)
	}
}

func TestApplySourceRejectsDriftBeforeDeployment(t *testing.T) {
	for _, scenario := range []string{"missing fingerprint", "missing commit", "tracked", "untracked", "generated", "head only", "staged", "absolute path", "traversal", "symlink", "validation path"} {
		t.Run(scenario, func(t *testing.T) {
			root, plan := applySourceFixture(t, "touch .bear/deployed", "touch .bear/second")
			switch scenario {
			case "missing fingerprint":
				plan.SourceFingerprint = ""
			case "missing commit":
				plan.Commit = ""
			case "tracked", "staged":
				sourceTestWrite(t, root, "app/source", "changed")
				if scenario == "staged" {
					sourceTestGit(t, root, "add", "app/source")
				}
			case "untracked", "generated":
				sourceTestWrite(t, root, "app/"+scenario, "changed")
			case "head only":
				sourceTestWrite(t, root, "bear.lock.yml", "environments: {}\n")
				sourceTestCommit(t, root)
			case "absolute path":
				plan.Artifacts[1].Path = root
			case "traversal":
				plan.Artifacts[1].Path = "app/../../outside"
			case "symlink":
				if err := os.Symlink(t.TempDir(), filepath.Join(root, "escape")); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
				plan.Artifacts[1].Path = "escape"
			case "validation path":
				plan.Validations = []config.PlanValidation{{Name: "invalid", Path: "../outside", Vars: map[string]string{"ENVIRONMENT": "int"}}}
			}
			applySourceSave(t, root, plan)
			before := applySourceRead(t, config.PlanFilePath(root))
			err := ApplyWithOptions(filepath.Join(root, "bear.config.yml"), Options{NoCommit: true, Concurrency: 1})
			if err == nil || !strings.Contains(err.Error(), "run 'bear plan <environment>' again") {
				t.Fatalf("expected replan error: %v", err)
			}
			applySourceAbsent(t, filepath.Join(root, ".bear/deployed"))
			applySourceAbsent(t, filepath.Join(root, ".bear/second"))
			if got := applySourceRead(t, config.PlanFilePath(root)); got != before {
				t.Fatal("rejected plan was changed")
			}
		})
	}
}

func TestApplySourceUsesSnapshotAndEnvironmentHistory(t *testing.T) {
	root, plan := applySourceFixture(t, `test "$ENVIRONMENT" = int && test "$NAME" = a && printf deployed > .bear/deployed`)
	// Current config is deliberately not loadable. Only its fingerprint matters.
	sourceTestWrite(t, root, "bear.config.yml", "not: [valid YAML")
	_, plan.SourceFingerprint, _ = sourceTestState(t, root)
	sourceTestWrite(t, root, "node_modules/dependency", "ignored after approval")
	lock := &config.LockFile{Artifacts: map[string]config.LockEntry{"legacy": {Commit: "old", Pinned: true}}}
	lock.UpdateArtifactPinned("prd", "a", "production", "prod-target", "prod")
	if err := lock.Save(filepath.Join(root, "bear.lock.yml")); err != nil {
		t.Fatal(err)
	}
	applySourceSave(t, root, plan)
	beforeIndex := sourceTestGit(t, root, "ls-files", "--stage")
	if err := ApplyWithOptions(filepath.Join(root, "bear.config.yml"), Options{NoCommit: true, Environment: "prd"}); err != nil {
		t.Fatal(err)
	}
	applySourceAbsent(t, config.PlanFilePath(root))
	if got := applySourceRead(t, filepath.Join(root, ".bear/deployed")); got != "deployed" {
		t.Fatalf("deployment output = %q", got)
	}
	got, err := config.LoadLock(filepath.Join(root, "bear.lock.yml"))
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := got.GetArtifact("int", "a")
	if !ok || entry.Commit != plan.Commit || entry.Target != "saved-target" || entry.Version != plan.Commit[:7] || entry.Pinned {
		t.Fatalf("wrong deployment history: %+v", entry)
	}
	if prod, _ := got.GetArtifact("prd", "a"); prod.Commit != "production" || !prod.Pinned || got.Artifacts["legacy"].Commit != "old" {
		t.Fatalf("unrelated history changed: %+v", got)
	}
	if sourceTestGit(t, root, "rev-parse", "HEAD") != plan.Commit || sourceTestGit(t, root, "ls-files", "--stage") != beforeIndex {
		t.Fatal("NoCommit modified HEAD or index")
	}
}

func TestApplySourcePartialCheckpointAndRetry(t *testing.T) {
	root, plan := applySourceFixture(t, "printf a >> .bear/deploy-a", "test -f .bear/allow-b && printf b >> .bear/deploy-b")
	applySourceSave(t, root, plan)
	opts := Options{NoCommit: true, Concurrency: 1}
	if err := ApplyWithOptions(filepath.Join(root, "bear.config.yml"), opts); err == nil {
		t.Fatal("expected deployment failure")
	}
	saved, err := config.ReadPlan(root)
	if err != nil || !saved.Artifacts[0].Completed || saved.Artifacts[1].Completed {
		t.Fatalf("incorrect checkpoints: %+v, %v", saved, err)
	}
	lock, err := config.LoadLock(filepath.Join(root, "bear.lock.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := lock.GetArtifact("int", "a"); !ok {
		t.Fatal("successful deployment not durable")
	}
	if _, ok := lock.GetArtifact("int", "b"); ok {
		t.Fatal("failed deployment recorded as successful")
	}
	if sourceTestGit(t, root, "rev-parse", "HEAD") != plan.Commit {
		t.Fatal("partial deployment changed HEAD")
	}
	sourceTestWrite(t, root, ".bear/allow-b", "")
	if err := ApplyWithOptions(filepath.Join(root, "bear.config.yml"), opts); err != nil {
		t.Fatal(err)
	}
	if applySourceRead(t, filepath.Join(root, ".bear/deploy-a")) != "a" || applySourceRead(t, filepath.Join(root, ".bear/deploy-b")) != "b" {
		t.Fatal("retry redeployed completed artifact")
	}
	applySourceAbsent(t, config.PlanFilePath(root))
}

func TestApplySourceCheckpointIsImmediate(t *testing.T) {
	root, plan := applySourceFixture(t, "printf a > .bear/deploy-a", "touch .bear/running; while test ! -f .bear/release; do sleep 0.05; done; exit 1")
	applySourceSave(t, root, plan)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- ApplyWithOptions(filepath.Join(root, "bear.config.yml"), Options{Context: ctx, NoCommit: true, Concurrency: 2})
	}()
	defer func() { cancel(); <-result }()
	for {
		saved, err := config.ReadPlan(root)
		_, runningErr := os.Stat(filepath.Join(root, ".bear/running"))
		if err == nil && saved.Artifacts[0].Completed && runningErr == nil {
			break
		}
		if ctx.Err() != nil {
			t.Fatal("first artifact was not checkpointed while another worker was running")
		}
		time.Sleep(10 * time.Millisecond)
	}
	lock, err := config.LoadLock(filepath.Join(root, "bear.lock.yml"))
	if err != nil || lock.GetLastDeployedCommit("int", "a") != plan.Commit {
		t.Fatalf("checkpoint preceded durable history: %+v, %v", lock, err)
	}
	sourceTestWrite(t, root, ".bear/release", "")
}

func TestApplySourceRetryRejectsDeploymentSourceChanges(t *testing.T) {
	root, plan := applySourceFixture(t, "printf changed > app/source", "exit 1")
	applySourceSave(t, root, plan)
	for attempt := 0; attempt < 2; attempt++ {
		err := ApplyWithOptions(filepath.Join(root, "bear.config.yml"), Options{NoCommit: true, Concurrency: 1})
		if err == nil || attempt == 1 && !strings.Contains(err.Error(), "fingerprint does not match") {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
	}
	saved, err := config.ReadPlan(root)
	if err != nil || saved.SourceFingerprint != plan.SourceFingerprint || !saved.Artifacts[0].Completed {
		t.Fatalf("approved fingerprint or checkpoint changed: %+v, %v", saved, err)
	}
}

func TestApplySourceRejectsBogusCompletedCheckpoint(t *testing.T) {
	for _, mismatch := range []string{"missing", "environment", "commit", "target", "pin", "version", "timestamp"} {
		t.Run(mismatch, func(t *testing.T) {
			root, plan := applySourceFixture(t, "touch .bear/deployed", "touch .bear/second")
			plan.Artifacts[0].Completed = true
			lock := &config.LockFile{}
			lock.UpdateArtifact("int", "a", plan.Commit, "saved-target", plan.Commit[:7])
			entry, _ := lock.GetArtifact("int", "a")
			switch mismatch {
			case "missing":
				delete(lock.Environments["int"], "a")
			case "environment":
				lock.Environments["prd"] = lock.Environments["int"]
				delete(lock.Environments, "int")
			case "commit":
				entry.Commit = "wrong"
			case "target":
				entry.Target = "wrong"
			case "pin":
				entry.Pinned = true
			case "version":
				entry.Version = "wrong"
			case "timestamp":
				entry.Timestamp = ""
			}
			if mismatch != "missing" && mismatch != "environment" {
				lock.Environments["int"]["a"] = entry
			}
			if err := lock.Save(filepath.Join(root, "bear.lock.yml")); err != nil {
				t.Fatal(err)
			}
			applySourceSave(t, root, plan)
			err := ApplyWithOptions(filepath.Join(root, "bear.config.yml"), Options{NoCommit: true})
			if err == nil || !strings.Contains(err.Error(), "does not match saved lock history") {
				t.Fatalf("bogus completed entry accepted: %v", err)
			}
			applySourceAbsent(t, filepath.Join(root, ".bear/deployed"))
			applySourceAbsent(t, filepath.Join(root, ".bear/second"))
		})
	}
}

func TestApplySourcePersistenceFailureCancelsJobs(t *testing.T) {
	for _, checkpoint := range []string{"lock", "plan"} {
		t.Run(checkpoint, func(t *testing.T) {
			breakSave := "mkdir bear.lock.yml"
			if checkpoint == "plan" {
				breakSave = "mv .bear/plan.yml .bear/approved; mkdir .bear/plan.yml"
			}
			root, plan := applySourceFixture(t,
				"while test ! -f .bear/running; do sleep 0.05; done; "+breakSave,
				"touch .bear/running; sleep 30; touch .bear/should-not-finish",
				"touch .bear/should-not-start")
			applySourceSave(t, root, plan)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			err := ApplyWithOptions(filepath.Join(root, "bear.config.yml"), Options{Context: ctx, NoCommit: true, Concurrency: 2})
			if err == nil || !strings.Contains(err.Error(), "deployment status uncertain") || ctx.Err() != nil {
				t.Fatalf("persistence failure not propagated promptly: %v (context %v)", err, ctx.Err())
			}
			applySourceAbsent(t, filepath.Join(root, ".bear/should-not-finish"))
			applySourceAbsent(t, filepath.Join(root, ".bear/should-not-start"))
			if checkpoint == "plan" {
				lock, err := config.LoadLock(filepath.Join(root, "bear.lock.yml"))
				if err != nil || lock.GetLastDeployedCommit("int", "a") != plan.Commit {
					t.Fatalf("successful history rolled back: %+v, %v", lock, err)
				}
			} else {
				saved, err := config.ReadPlan(root)
				if err != nil || saved.Artifacts[0].Completed || saved.Artifacts[1].Completed || saved.Artifacts[2].Completed {
					t.Fatalf("failed save marked completed: %+v, %v", saved, err)
				}
			}
		})
	}
}

func TestApplySourceContextAndWorkspaceLock(t *testing.T) {
	root, plan := applySourceFixture(t, "touch .bear/deployed")
	applySourceSave(t, root, plan)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ApplyWithOptions(filepath.Join(root, "bear.config.yml"), Options{Context: ctx, NoCommit: true}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context ignored: %v", err)
	}
	release, err := acquireWorkspaceLock(root)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	// An unreadable plan proves lock acquisition precedes ReadPlan.
	sourceTestWrite(t, root, ".bear/plan.yml", "invalid: [yaml")
	err = ApplyWithOptions(filepath.Join(root, "bear.config.yml"), Options{NoCommit: true})
	if err == nil || !strings.Contains(err.Error(), "cannot lock workspace") {
		t.Fatalf("apply did not acquire workspace lock first: %v", err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
	applySourceSave(t, root, plan)
	if err := ApplyWithOptions(filepath.Join(root, "bear.config.yml"), Options{NoCommit: true}); err != nil {
		t.Fatal(err)
	}
}

func TestApplySourcePinnedRebuildsAllSavedValidations(t *testing.T) {
	for _, scenario := range []string{"success", "validation failure", "nondeterministic output", "commit mismatch", "deploy failure"} {
		t.Run(scenario, func(t *testing.T) {
			root, plan := applySourceFixture(t, `test "$(cat app/source)" = approved && test "$(cat app/generated)" = rebuilt && test "$(cat node_modules/dependency)" = installed && cat app/generated >> "$RECEIPT"`)
			receipt := filepath.Join(t.TempDir(), "receipt")
			validationReceipt := filepath.Join(t.TempDir(), "validations")
			plan.Pinned = true
			plan.Artifacts[0].Pinned = true
			plan.Artifacts[0].PinCommit = plan.Commit
			plan.Artifacts[0].Vars["RECEIPT"] = receipt
			plan.Validations = []config.PlanValidation{
				{Name: "dependency-only", Path: ".", Vars: map[string]string{"ENVIRONMENT": "int", "VALIDATIONS": validationReceipt}, Steps: []config.Step{{Name: "setup", Run: `test ! -e node_modules/dependency && mkdir -p node_modules && printf installed > node_modules/dependency && printf dependency >> "$VALIDATIONS"`}}},
				{Name: "a", Path: "app", Vars: map[string]string{"ENVIRONMENT": "int", "VALUE": "rebuilt", "VALIDATIONS": validationReceipt}, Steps: []config.Step{{Name: "build", Run: `test "$ENVIRONMENT" = int && printf "%s" "$VALUE" > generated && printf app >> "$VALIDATIONS"`}}},
			}
			// Planning approves the deterministic post-validation source, not the
			// pristine checkout. Apply must regenerate this file in its own tree.
			sourceTestWrite(t, root, "app/generated", "rebuilt")
			_, plan.SourceFingerprint, _ = sourceTestState(t, root)
			if err := os.Remove(filepath.Join(root, "app/generated")); err != nil {
				t.Fatal(err)
			}
			sourceTestWrite(t, root, "app/source", "new HEAD\n")
			callerCommit := sourceTestCommit(t, root)
			sourceTestWrite(t, root, "app/source", "dirty caller\n")
			sourceTestGit(t, root, "add", "app/source")
			sourceTestWrite(t, root, "app/generated", "caller output")
			sourceTestWrite(t, root, "node_modules/dependency", "caller ignored dependency")
			beforeIndex := sourceTestGit(t, root, "ls-files", "--stage")
			beforeConfig := applySourceRead(t, filepath.Join(root, ".git/config"))
			switch scenario {
			case "validation failure":
				plan.Validations[0].Steps[0].Run = "exit 1"
			case "nondeterministic output":
				plan.Validations[1].Vars["VALUE"] = "different output"
			case "commit mismatch":
				plan.Commit = "HEAD"
				plan.Artifacts[0].PinCommit = "HEAD"
			case "deploy failure":
				plan.Artifacts[0].Steps[0].Run = "exit 1"
			}
			applySourceSave(t, root, plan)
			err := ApplyWithOptions(filepath.Join(root, "bear.config.yml"), Options{NoCommit: true, Concurrency: 1, Verbose: true})
			if scenario == "success" {
				if err != nil {
					t.Fatal(err)
				}
				if got := applySourceRead(t, receipt); got != "rebuilt" {
					t.Fatalf("deployed wrong source: %q", got)
				}
				if got := applySourceRead(t, validationReceipt); got != "dependencyapp" {
					t.Fatalf("did not rerun all validations: %q", got)
				}
				lock, err := config.LoadLock(filepath.Join(root, "bear.lock.yml"))
				if err != nil {
					t.Fatal(err)
				}
				entry, _ := lock.GetArtifact("int", "a")
				if !entry.Pinned || entry.Commit != plan.Commit || entry.Version != plan.Commit[:7] {
					t.Fatalf("wrong pin history: %+v", entry)
				}
			} else {
				if err == nil {
					t.Fatal("unsafe pinned apply succeeded")
				}
				applySourceAbsent(t, receipt)
				applySourceAbsent(t, filepath.Join(root, "bear.lock.yml"))
				if !config.PlanExists(root) {
					t.Fatal("failed plan removed")
				}
			}
			if sourceTestGit(t, root, "rev-parse", "HEAD") != callerCommit || sourceTestGit(t, root, "ls-files", "--stage") != beforeIndex {
				t.Fatal("pinned apply changed caller HEAD/index")
			}
			for path, want := range map[string]string{"app/source": "dirty caller\n", "app/generated": "caller output", "node_modules/dependency": "caller ignored dependency", ".git/config": beforeConfig} {
				if got := applySourceRead(t, filepath.Join(root, path)); got != want {
					t.Errorf("caller %s changed: %q", path, got)
				}
			}
			if listing := sourceTestGit(t, root, "worktree", "list", "--porcelain"); strings.Contains(listing, "bear-source-") {
				t.Fatalf("pinned worktree leaked: %s", listing)
			}
		})
	}
}

func TestApplySourceGitFailureKeepsCompletedPlan(t *testing.T) {
	for _, failure := range []string{"commit", "push"} {
		t.Run(failure, func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("Git hooks use POSIX shell")
			}
			root, remote := gitFixture(t)
			sourceTestWrite(t, root, "bear.lock.yml", "environments: {}\n")
			commit, fingerprint, _ := sourceTestState(t, root)
			plan := config.NewPlanFile(commit)
			plan.Environment = "int"
			plan.SourceFingerprint = fingerprint
			plan.Artifacts = []config.PlanArtifact{{Name: "a", Path: ".", Target: "saved", Environments: []string{"int"}, Vars: map[string]string{"ENVIRONMENT": "int"}, Steps: []config.Step{{Name: "deploy", Run: "printf a >> .bear/deployed"}}}}
			applySourceSave(t, root, plan)
			hook := filepath.Join(root, ".git/hooks/pre-commit")
			if failure == "push" {
				hook = filepath.Join(remote, "hooks/pre-receive")
			}
			if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0755); err != nil {
				t.Fatal(err)
			}
			opts := Options{GitRemote: "origin", GitBranch: "main"}
			err := ApplyWithOptions(filepath.Join(root, "bear.config.yml"), opts)
			if err == nil || !strings.Contains(err.Error(), "Git publication failed") {
				t.Fatalf("Git failure swallowed: %v", err)
			}
			saved, err := config.ReadPlan(root)
			if err != nil || !saved.Artifacts[0].Completed {
				t.Fatalf("Git failure lost completion: %+v, %v", saved, err)
			}
			if err := os.Remove(hook); err != nil {
				t.Fatal(err)
			}
			if failure == "push" {
				// A locally created lock commit intentionally needs manual push
				// recovery: the helper must not publish an arbitrary ahead HEAD.
				err := ApplyWithOptions(filepath.Join(root, "bear.config.yml"), opts)
				if err == nil || !strings.Contains(err.Error(), "unsafe Git push") {
					t.Fatalf("unpublished commit silently pushed: %v", err)
				}
				sourceTestGit(t, root, "push", "origin", "HEAD:refs/heads/main")
			}
			// Source may have changed after completed deployments. A Git-only
			// retry must neither revalidate nor redeploy that completed snapshot.
			sourceTestWrite(t, root, "unrelated", "changed after deployment")
			if err := ApplyWithOptions(filepath.Join(root, "bear.config.yml"), opts); err != nil {
				t.Fatal(err)
			}
			if applySourceRead(t, filepath.Join(root, ".bear/deployed")) != "a" {
				t.Fatal("Git retry redeployed artifact")
			}
			applySourceAbsent(t, config.PlanFilePath(root))
			if got := sourceTestGit(t, remote, "rev-parse", "main"); got != sourceTestGit(t, root, "rev-parse", "HEAD") {
				t.Fatal("lock commit not published")
			}
			if got := sourceTestGit(t, remote, "show", "main:unrelated"); got != "original" {
				t.Fatal("Git-only retry committed source changes")
			}
		})
	}
}

func TestApplySourceRemovePlanErrorPreservesCompletion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Git hooks use POSIX shell")
	}
	root, _ := gitFixture(t)
	sourceTestWrite(t, root, "bear.lock.yml", "environments: {}\n")
	commit, fingerprint, _ := sourceTestState(t, root)
	plan := config.NewPlanFile(commit)
	plan.Environment, plan.SourceFingerprint = "int", fingerprint
	plan.Artifacts = []config.PlanArtifact{{Name: "a", Path: ".", Target: "saved", Environments: []string{"int"}, Vars: map[string]string{"ENVIRONMENT": "int"}}}
	applySourceSave(t, root, plan)
	// Make removal fail only after the completed checkpoint and Git commit.
	sourceTestWrite(t, root, ".git/hooks/post-commit", "#!/bin/sh\nmv .bear/plan.yml .bear/completed\nmkdir .bear/plan.yml\ntouch .bear/plan.yml/keep\n")
	if err := os.Chmod(filepath.Join(root, ".git/hooks/post-commit"), 0755); err != nil {
		t.Fatal(err)
	}
	err := ApplyWithOptions(filepath.Join(root, "bear.config.yml"), Options{GitBranch: "main"})
	if err == nil || !strings.Contains(err.Error(), "error removing completed plan") {
		t.Fatalf("RemovePlan error swallowed: %v", err)
	}
	if !strings.Contains(applySourceRead(t, filepath.Join(root, ".bear/completed")), "completed: true") {
		t.Fatal("completed checkpoint not written before cleanup")
	}
}

func TestApplySourceCancellationPreservesPendingJobs(t *testing.T) {
	for _, pinned := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal", true: "pinned"}[pinned], func(t *testing.T) {
			root, plan := applySourceFixture(t, `touch "$RUNNING"; sleep 30; touch "$FINISHED"`, "exit 0")
			markers := t.TempDir()
			plan.Pinned = pinned
			for i := range plan.Artifacts {
				plan.Artifacts[i].Pinned = pinned
				plan.Artifacts[i].Vars["RUNNING"] = filepath.Join(markers, "running")
				plan.Artifacts[i].Vars["FINISHED"] = filepath.Join(markers, "finished")
			}
			applySourceSave(t, root, plan)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			result := make(chan error, 1)
			go func() {
				result <- ApplyWithOptions(filepath.Join(root, "bear.config.yml"), Options{Context: ctx, NoCommit: true, Concurrency: 1})
			}()
			for {
				if _, err := os.Stat(filepath.Join(markers, "running")); err == nil || ctx.Err() != nil {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			cancel()
			if err := <-result; !errors.Is(err, context.Canceled) {
				t.Fatalf("running deployment did not return cancellation: %v", err)
			}
			applySourceAbsent(t, filepath.Join(markers, "finished"))
			applySourceAbsent(t, filepath.Join(root, "bear.lock.yml"))
			saved, err := config.ReadPlan(root)
			if err != nil || saved.Artifacts[0].Completed || saved.Artifacts[1].Completed {
				t.Fatalf("canceled/unscheduled jobs marked successful: %+v, %v", saved, err)
			}
			if listing := sourceTestGit(t, root, "worktree", "list", "--porcelain"); strings.Contains(listing, "bear-source-") {
				t.Fatal("cancellation leaked pinned worktree")
			}
			release, err := acquireWorkspaceLock(root)
			if err != nil {
				t.Fatalf("cancellation leaked workspace lock: %v", err)
			}
			if err := release(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestApplySourceVerboseStreamsWithoutDuplicateCapture(t *testing.T) {
	root, plan := applySourceFixture(t, "printf 'unique-step-output\\n'; exit 1")
	applySourceSave(t, root, plan)
	output, err := captureEnvironmentOutput(t, func() error {
		return ApplyWithOptions(filepath.Join(root, "bear.config.yml"), Options{NoCommit: true, Verbose: true})
	})
	if err == nil {
		t.Fatal("expected step failure")
	}
	if strings.Count(output, "unique-step-output") != 1 || !strings.Contains(output, "a -> saved-target | deploy | unique-step-output") {
		t.Fatalf("verbose output not streamed exactly once: %s", output)
	}
	if !strings.Contains(output, "deploy / deploy (1/1)") || !strings.Contains(output, "failed") {
		t.Fatalf("missing phase/failure tracking: %s", output)
	}
}
