package cmd

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func sourceTestGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func sourceTestWrite(t *testing.T, root, path, content string) {
	t.Helper()
	path = filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func sourceTestCommit(t *testing.T, root string) string {
	t.Helper()
	sourceTestGit(t, root, "add", ".")
	sourceTestGit(t, root, "-c", "user.name=Source Test", "-c", "user.email=source@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "source fixture")
	return sourceTestGit(t, root, "rev-parse", "HEAD")
}

func sourceTestRepo(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	sourceTestGit(t, root, "init")
	sourceTestWrite(t, root, ".gitignore", "node_modules/\nignored-output\n")
	sourceTestWrite(t, root, "workspace/app/source.txt", "A\n")
	sourceTestWrite(t, root, "shared.txt", "shared\n")
	sourceTestWrite(t, root, "bear.lock.yml", "lock\n")
	sourceTestWrite(t, root, "workspace/bear.lock.yml", "nested lock\n")
	sourceTestWrite(t, root, ".bear/tracked", "state\n")
	return root, sourceTestCommit(t, root)
}

func sourceTestState(t *testing.T, root string) (string, string, bool) {
	t.Helper()
	commit, fingerprint, dirty, err := sourceState(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(commit) < 40 || len(fingerprint) != 64 {
		t.Fatalf("invalid source state: %q %q", commit, fingerprint)
	}
	return commit, fingerprint, dirty
}

func TestPrepareSourcePinnedNestedWorkspace(t *testing.T) {
	root, a := sourceTestRepo(t)
	sourceTestGit(t, root, "tag", "source-a", a)
	sourceTestWrite(t, root, "workspace/app/source.txt", "B\n")
	b := sourceTestCommit(t, root)
	sourceTestWrite(t, root, "workspace/app/source.txt", "dirty B\n")
	sourceTestWrite(t, root, "workspace/fixture", "untracked\n")
	sourceTestWrite(t, root, "workspace/node_modules/dependency", "ignored\n")
	sourceTestGit(t, root, "add", "workspace/app/source.txt")
	// A normal checkout would run this hook. Source preparation must not run it
	// or change hooks configuration, the caller's index, branch, or files.
	sourceTestWrite(t, root, ".git/hooks/post-checkout", "#!/bin/sh\nprintf hook > hook-ran\nexit 1\n")
	if err := os.Chmod(filepath.Join(root, ".git/hooks/post-checkout"), 0755); err != nil {
		t.Fatal(err)
	}
	beforeStatus := sourceTestGit(t, root, "status", "--porcelain=v1", "-z")
	beforeIndex := sourceTestGit(t, root, "ls-files", "--stage", "-z")
	beforeBranch := sourceTestGit(t, root, "symbolic-ref", "HEAD")
	beforeConfig, err := os.ReadFile(filepath.Join(root, ".git/config"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	snapshot, commit, cleanup, err := prepareSource(ctx, filepath.Join(root, "workspace"), "source-a")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Error(err)
		}
	})
	if commit != a || filepath.Base(snapshot) != "workspace" {
		t.Fatalf("snapshot = %q at %s, want workspace at %s", snapshot, commit, a)
	}
	data, err := os.ReadFile(filepath.Join(snapshot, "app/source.txt"))
	if err != nil || string(data) != "A\n" {
		t.Fatalf("pin did not read A: %q, %v", data, err)
	}
	if got := sourceTestGit(t, snapshot, "rev-parse", "HEAD"); got != a {
		t.Fatalf("worktree HEAD = %s, want %s", got, a)
	}
	for _, path := range []string{"fixture", "node_modules/dependency", "../hook-ran"} {
		if _, err := os.Stat(filepath.Join(snapshot, path)); !os.IsNotExist(err) {
			t.Errorf("unexpected snapshot file %s: %v", path, err)
		}
	}
	if got := sourceTestGit(t, root, "rev-parse", "HEAD"); got != b {
		t.Errorf("caller HEAD changed: %s", got)
	}
	if got := sourceTestGit(t, root, "symbolic-ref", "HEAD"); got != beforeBranch {
		t.Errorf("caller branch changed: %s", got)
	}
	if got := sourceTestGit(t, root, "status", "--porcelain=v1", "-z"); got != beforeStatus {
		t.Errorf("caller working tree changed: %q vs %q", got, beforeStatus)
	}
	if got := sourceTestGit(t, root, "ls-files", "--stage", "-z"); got != beforeIndex {
		t.Error("caller index changed")
	}
	afterConfig, err := os.ReadFile(filepath.Join(root, ".git/config"))
	if err != nil || string(afterConfig) != string(beforeConfig) {
		t.Fatalf("Git config changed: %v", err)
	}
	sourceTestWrite(t, snapshot, "app/source.txt", "validation changed tracked file")
	sourceTestWrite(t, snapshot, "generated/output", "validation output")
	cancel()
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup is not idempotent: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(snapshot)); !os.IsNotExist(err) {
		t.Fatalf("snapshot survived cleanup: %v", err)
	}
	if listing := sourceTestGit(t, root, "worktree", "list", "--porcelain"); strings.Contains(listing, "bear-source-") {
		t.Fatalf("worktree registration survived cleanup: %s", listing)
	}
}

func TestPrepareSourceRejectsInvalidRevisionAndMissingWorkspace(t *testing.T) {
	root, a := sourceTestRepo(t)
	sourceTestWrite(t, root, "later/source", "new workspace")
	sourceTestCommit(t, root)
	for _, revision := range []string{"does-not-exist", "--help", "HEAD:shared.txt", ""} {
		if path, commit, cleanup, err := prepareSource(context.Background(), root, revision); err == nil || path != "" || commit != "" || cleanup != nil {
			t.Fatalf("accepted revision %q: %q %q %v", revision, path, commit, err)
		}
	}
	if _, _, _, err := prepareSource(context.Background(), filepath.Join(root, "later"), a); err == nil {
		t.Fatal("accepted missing pinned workspace")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, _, err := prepareSource(ctx, root, "HEAD"); err == nil {
		t.Fatal("accepted canceled preparation")
	}
	if listing := sourceTestGit(t, root, "worktree", "list", "--porcelain"); strings.Contains(listing, "bear-source-") {
		t.Fatalf("failed preparation leaked worktree: %s", listing)
	}
}

func TestSourceStateTracksHeadAndRelocates(t *testing.T) {
	root, a := sourceTestRepo(t)
	commit, fingerprint, dirty := sourceTestState(t, root)
	if commit != a || dirty {
		t.Fatalf("clean A state: %s, dirty=%v", commit, dirty)
	}
	_, nestedFingerprint, _ := sourceTestState(t, filepath.Join(root, "workspace"))
	if nestedFingerprint != fingerprint {
		t.Fatal("nested workspace did not fingerprint entire repository")
	}
	snapshot, _, cleanup, err := prepareSource(context.Background(), root, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cleanup(); err != nil {
			t.Error(err)
		}
	}()
	_, relocated, dirty := sourceTestState(t, snapshot)
	if relocated != fingerprint || dirty {
		t.Fatalf("relocation changed fingerprint: %s vs %s, dirty=%v", relocated, fingerprint, dirty)
	}
	// A state-only commit moves HEAD without changing the source digest.
	sourceTestWrite(t, root, "bear.lock.yml", "updated lock")
	b := sourceTestCommit(t, root)
	commit, sameTree, dirty := sourceTestState(t, root)
	if commit != b || commit == a || sameTree != fingerprint || dirty {
		t.Fatalf("moved HEAD state: %s %s dirty=%v", commit, sameTree, dirty)
	}
}

func TestSourceFingerprintChanges(t *testing.T) {
	for _, scenario := range []string{"tracked", "untracked output", "staged", "deleted", "deleted directory", "rename", "mode", "symlink", "outside workspace", "newline name", "ignored tracked"} {
		t.Run(scenario, func(t *testing.T) {
			root, _ := sourceTestRepo(t)
			if scenario == "ignored tracked" {
				sourceTestWrite(t, root, "ignored-output", "tracked despite ignore")
				sourceTestGit(t, root, "add", "--force", "ignored-output")
				sourceTestCommit(t, root)
			}
			_, before, _ := sourceTestState(t, root)
			path := filepath.Join(root, "workspace/app/source.txt")
			switch scenario {
			case "tracked", "staged":
				sourceTestWrite(t, root, "workspace/app/source.txt", "changed")
				if scenario == "staged" {
					sourceTestGit(t, root, "add", "workspace/app/source.txt")
				}
			case "untracked output":
				sourceTestWrite(t, root, "workspace/generated/output", "validated output")
			case "deleted":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			case "deleted directory":
				if err := os.RemoveAll(filepath.Dir(path)); err != nil {
					t.Fatal(err)
				}
			case "rename":
				if err := os.Rename(path, path+".renamed"); err != nil {
					t.Fatal(err)
				}
			case "mode":
				if runtime.GOOS == "windows" {
					t.Skip("Windows does not preserve executable mode")
				}
				if err := os.Chmod(path, 0755); err != nil {
					t.Fatal(err)
				}
			case "symlink":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("../../shared.txt", path); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			case "outside workspace":
				sourceTestWrite(t, root, "shared.txt", "changed shared dependency")
			case "newline name":
				sourceTestWrite(t, root, "workspace/line\nbreak\tfile", "output")
			case "ignored tracked":
				sourceTestWrite(t, root, "ignored-output", "changed tracked ignored file")
			}
			_, after, dirty := sourceTestState(t, filepath.Join(root, "workspace"))
			if before == after || !dirty {
				t.Fatalf("missed %s change: %s vs %s, dirty=%v", scenario, before, after, dirty)
			}
		})
	}
}

func TestSourceFingerprintExcludesOnlyStateAndIgnoredUntracked(t *testing.T) {
	root, _ := sourceTestRepo(t)
	_, before, _ := sourceTestState(t, root)
	for _, path := range []string{"bear.lock.yml", "workspace/bear.lock.yml", ".bear/tracked", ".bear/plan.json", "workspace/.bear/plan.json", "workspace/node_modules/dependency", "ignored-output"} {
		sourceTestWrite(t, root, path, "changed")
	}
	sourceTestGit(t, root, "add", "bear.lock.yml", "workspace/bear.lock.yml", ".bear")
	_, after, dirty := sourceTestState(t, root)
	if before != after || dirty {
		t.Fatalf("state/ignored changes affected source: %s vs %s, dirty=%v", before, after, dirty)
	}
	sourceTestWrite(t, root, "workspace/bear.lock.yml.backup", "not excluded")
	_, after, dirty = sourceTestState(t, root)
	if before == after || !dirty {
		t.Fatal("excluded ordinary source")
	}
}

func TestSourceFingerprintSymlinkTargetAndContainment(t *testing.T) {
	root, _ := sourceTestRepo(t)
	link := filepath.Join(root, "link")
	if err := os.Symlink("shared.txt", link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, before, _ := sourceTestState(t, root)
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing-target", link); err != nil {
		t.Fatal(err)
	}
	_, after, _ := sourceTestState(t, root)
	if before == after {
		t.Fatal("symlink target change not fingerprinted")
	}
	if err := os.RemoveAll(filepath.Join(root, "workspace/app")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "workspace/app")); err != nil {
		t.Fatal(err)
	}
	if _, err := sourceFingerprint(context.Background(), root); err == nil {
		t.Fatal("fingerprinted through escaped parent symlink")
	}
}

func TestSourceStateFailsClosed(t *testing.T) {
	if _, _, _, err := sourceState(context.Background(), t.TempDir()); err == nil {
		t.Fatal("accepted non-repository")
	}
	empty := t.TempDir()
	sourceTestGit(t, empty, "init")
	if _, _, _, err := sourceState(context.Background(), empty); err == nil {
		t.Fatal("accepted unborn HEAD")
	}
	root, commit := sourceTestRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, _, err := sourceState(ctx, root); err == nil {
		t.Fatal("accepted canceled state check")
	}
	sourceTestGit(t, root, "update-index", "--add", "--cacheinfo", "160000,"+commit+",submodule")
	if _, _, _, err := sourceState(context.Background(), root); err == nil || !strings.Contains(err.Error(), "submodule") {
		t.Fatalf("submodule must fail closed: %v", err)
	}
}

func TestSourceStateRejectsHiddenIndexChanges(t *testing.T) {
	for _, flag := range []string{"--assume-unchanged", "--skip-worktree"} {
		t.Run(flag, func(t *testing.T) {
			root, _ := sourceTestRepo(t)
			sourceTestGit(t, root, "update-index", flag, "shared.txt")
			sourceTestWrite(t, root, "shared.txt", "hidden change")
			if _, _, _, err := sourceState(context.Background(), root); err == nil {
				t.Fatal("accepted index flag that hides dirty source")
			}
		})
	}
}

func TestSourceStateReportsIndexOnlyChanges(t *testing.T) {
	root, _ := sourceTestRepo(t)
	_, before, _ := sourceTestState(t, root)
	sourceTestWrite(t, root, "shared.txt", "staged content")
	sourceTestGit(t, root, "add", "shared.txt")
	sourceTestWrite(t, root, "shared.txt", "shared\n")
	_, after, dirty := sourceTestState(t, root)
	if before != after || !dirty {
		t.Fatalf("index-only changes: %s vs %s, dirty=%v", before, after, dirty)
	}
}

func TestSourceHelpersIgnoreRepositoryEnvironment(t *testing.T) {
	root, commit := sourceTestRepo(t)
	other, _ := sourceTestRepo(t)
	sourceTestWrite(t, other, "shared.txt", "other repository")
	sourceTestCommit(t, other)
	t.Setenv("GIT_DIR", filepath.Join(other, ".git"))
	t.Setenv("GIT_WORK_TREE", other)
	t.Setenv("GIT_INDEX_FILE", filepath.Join(other, ".git/index"))
	got, _, dirty := sourceTestState(t, root)
	if got != commit || dirty {
		t.Fatalf("environment redirected source state: %s, dirty=%v", got, dirty)
	}
	snapshot, got, cleanup, err := prepareSource(context.Background(), root, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := cleanup(); err != nil {
			t.Error(err)
		}
	}()
	data, err := os.ReadFile(filepath.Join(snapshot, "shared.txt"))
	if err != nil || got != commit || string(data) != "shared\n" {
		t.Fatalf("environment redirected checkout: %s %q, %v", got, data, err)
	}
}

func TestSafeArtifactPath(t *testing.T) {
	root := t.TempDir()
	sourceTestWrite(t, root, "app/source", "source")
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{".", "app", "app/source", "app/generated/output", "missing/output"} {
		path, err := safeArtifactPath(root, rel)
		if err != nil || path != filepath.Join(canonical, rel) {
			t.Errorf("safe path %q: %q, %v", rel, path, err)
		}
	}
	for _, rel := range []string{"", "..", "../outside", "app/../../outside", "app/../source", root, "/absolute", `C:\outside`, `C:outside`, `\\server\share`, `..\outside`, "app\x00bad"} {
		if path, err := safeArtifactPath(root, rel); err == nil {
			t.Errorf("accepted unsafe path %q as %q", rel, path)
		}
	}
	if err := os.Symlink("app", filepath.Join(root, "inside")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if path, err := safeArtifactPath(root, "inside/generated/output"); err != nil || path != filepath.Join(canonical, "app/generated/output") {
		t.Fatalf("internal symlink: %q, %v", path, err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "outside")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(root, "dangling")); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"outside", "outside/missing/output", "dangling/output"} {
		if path, err := safeArtifactPath(root, rel); err == nil {
			t.Errorf("accepted unsafe symlink %q as %q", rel, path)
		}
	}
}
