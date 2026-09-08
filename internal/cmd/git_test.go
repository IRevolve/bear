package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/irevolve/bear/internal/config"
)

func gitFixtureCommand(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func gitFixtureWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func gitFixture(t *testing.T) (string, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	for _, key := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_NAMESPACE", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT", "BRANCH_NAME", "GIT_LOCAL_BRANCH", "GIT_BRANCH", "CHANGE_ID", "CHANGE_BRANCH", "CHANGE_TARGET"} {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("LC_ALL", "C")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_AUTHOR_NAME", "Bear Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "bear@example.invalid")
	t.Setenv("GIT_COMMITTER_NAME", "Bear Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "bear@example.invalid")
	root, remote := t.TempDir(), t.TempDir()
	gitFixtureCommand(t, remote, "init", "--bare")
	gitFixtureCommand(t, root, "init", "-b", "main")
	gitFixtureWrite(t, filepath.Join(root, "bear.lock.yml"), "old lock\n")
	gitFixtureWrite(t, filepath.Join(root, "unrelated"), "original\n")
	gitFixtureCommand(t, root, "add", ".")
	gitFixtureCommand(t, root, "commit", "-m", "initial")
	gitFixtureCommand(t, root, "remote", "add", "origin", remote)
	gitFixtureCommand(t, root, "push", "origin", "HEAD:refs/heads/main")
	return root, remote
}

func gitFixtureStaged(t *testing.T, root string) []byte {
	t.Helper()
	gitFixtureWrite(t, filepath.Join(root, "unrelated"), "staged\n")
	gitFixtureWrite(t, filepath.Join(root, "bear.lock.yml"), "caller staged lock\n")
	gitFixtureCommand(t, root, "add", "unrelated", "bear.lock.yml")
	gitFixtureWrite(t, filepath.Join(root, "unrelated"), "unstaged\n")
	gitFixtureWrite(t, filepath.Join(root, "untracked"), "untracked\n")
	gitFixtureWrite(t, filepath.Join(root, "bear.lock.yml"), "deployed lock\n")
	index, err := os.ReadFile(filepath.Join(root, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	return index
}

func gitFixtureAssertPreserved(t *testing.T, root string, index []byte) {
	t.Helper()
	after, err := os.ReadFile(filepath.Join(root, ".git", "index"))
	if err != nil || !bytes.Equal(after, index) {
		t.Errorf("caller index changed: %v", err)
	}
	for name, want := range map[string]string{"unrelated": "unstaged\n", "untracked": "untracked\n", "bear.lock.yml": "deployed lock\n"} {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(data) != want {
			t.Errorf("%s changed: %q, %v", name, data, err)
		}
	}
}

func TestCommitLockFileIsolation(t *testing.T) {
	for _, detached := range []bool{false, true} {
		t.Run(map[bool]string{false: "branch", true: "detached"}[detached], func(t *testing.T) {
			root, remote := gitFixture(t)
			branch := ""
			if detached {
				gitFixtureCommand(t, root, "checkout", "--detach")
				gitFixtureCommand(t, root, "remote", "rename", "origin", "publish")
				branch = "main"
			}
			index := gitFixtureStaged(t, root)
			beforeConfig, err := os.ReadFile(filepath.Join(root, ".git", "config"))
			if err != nil {
				t.Fatal(err)
			}
			remoteName := ""
			if detached {
				remoteName = "publish"
			}
			if err := commitLockFile(context.Background(), root, filepath.Join(root, "bear.lock.yml"), []config.PlanArtifact{{Name: "api"}}, remoteName, branch); err != nil {
				t.Fatal(err)
			}
			gitFixtureAssertPreserved(t, root, index)
			afterConfig, err := os.ReadFile(filepath.Join(root, ".git", "config"))
			if err != nil || !bytes.Equal(beforeConfig, afterConfig) {
				t.Errorf("Git config changed: %v", err)
			}
			if got := gitFixtureCommand(t, root, "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD"); got != "bear.lock.yml" {
				t.Errorf("committed paths = %q", got)
			}
			if got := gitFixtureCommand(t, remote, "show", "main:bear.lock.yml"); got != "deployed lock" {
				t.Errorf("remote lock = %q", got)
			}
			if got := gitFixtureCommand(t, root, "show", ":bear.lock.yml"); got != "caller staged lock" {
				t.Errorf("staged lock = %q", got)
			}
			if got := gitFixtureCommand(t, remote, "log", "-1", "--format=%B", "main"); !strings.Contains(got, "[skip ci]") || !strings.Contains(got, "Deployed: api") {
				t.Errorf("message = %q", got)
			}
		})
	}
}

func TestCommitLockFileUnsafeTips(t *testing.T) {
	for _, scenario := range []string{"ahead", "behind", "missing"} {
		t.Run(scenario, func(t *testing.T) {
			root, remote := gitFixture(t)
			branch := "main"
			switch scenario {
			case "ahead", "behind":
				gitFixtureCommand(t, root, "commit", "--allow-empty", "-m", "unrelated")
				if scenario == "behind" {
					gitFixtureCommand(t, root, "push", "origin", "HEAD:refs/heads/main")
					gitFixtureCommand(t, root, "checkout", "--detach", "HEAD^")
				}
			case "missing":
				branch = "new-branch"
			}
			before := gitFixtureCommand(t, root, "rev-parse", "HEAD")
			remoteBefore := gitFixtureCommand(t, remote, "rev-parse", "main")
			index := gitFixtureStaged(t, root)
			err := commitLockFile(context.Background(), root, filepath.Join(root, "bear.lock.yml"), nil, "origin", branch)
			if err == nil || !strings.Contains(err.Error(), "unsafe Git push") {
				t.Fatalf("expected unsafe push error, got %v", err)
			}
			if gitFixtureCommand(t, root, "rev-parse", "HEAD") != before || gitFixtureCommand(t, remote, "rev-parse", "main") != remoteBefore {
				t.Error("unsafe tip caused a commit or push")
			}
			gitFixtureAssertPreserved(t, root, index)
		})
	}
}

func TestCommitLockFileFailures(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell hook fixtures require Unix")
	}
	for _, scenario := range []string{"identity", "commit hook", "push rejected", "hook stages unrelated"} {
		t.Run(scenario, func(t *testing.T) {
			root, remote := gitFixture(t)
			index := gitFixtureStaged(t, root)
			before := gitFixtureCommand(t, root, "rev-parse", "HEAD")
			want := ""
			switch scenario {
			case "identity":
				t.Setenv("GIT_AUTHOR_NAME", "")
				t.Setenv("GIT_COMMITTER_NAME", "")
				want = "empty ident name"
			case "commit hook", "push rejected", "hook stages unrelated":
				hook := filepath.Join(root, ".git", "hooks", "pre-commit")
				body := "#!/bin/sh\necho 'policy blocked https://user:supersecret@example.invalid/repo' >&2\nexit 1\n"
				want = "policy blocked"
				if scenario == "push rejected" {
					hook = filepath.Join(remote, "hooks", "pre-receive")
				}
				if scenario == "hook stages unrelated" {
					body = "#!/bin/sh\ngit add unrelated\n"
					want = "refusing to push"
				}
				gitFixtureWrite(t, hook, body)
				if err := os.Chmod(hook, 0700); err != nil {
					t.Fatal(err)
				}
			}
			err := commitLockFile(context.Background(), root, filepath.Join(root, "bear.lock.yml"), nil, "", "")
			if err == nil || !strings.Contains(err.Error(), want) || strings.Contains(err.Error(), "supersecret") {
				t.Fatalf("expected sanitized actionable %q error, got %v", want, err)
			}
			gitFixtureAssertPreserved(t, root, index)
			if gitFixtureCommand(t, remote, "rev-parse", "main") != before {
				t.Error("failure published a commit")
			}
			after := gitFixtureCommand(t, root, "rev-parse", "HEAD")
			committed := scenario == "push rejected" || scenario == "hook stages unrelated"
			if (after != before) != committed {
				t.Errorf("local commit exists = %v, want %v", after != before, committed)
			}
		})
	}
}

func TestCommitLockFileJenkinsBranches(t *testing.T) {
	for _, scenario := range []string{"consistent", "prefixed", "PR", "change", "ambiguous", "unknown remote", "invalid", "absent"} {
		t.Run(scenario, func(t *testing.T) {
			root, _ := gitFixture(t)
			gitFixtureCommand(t, root, "checkout", "--detach")
			gitFixtureWrite(t, filepath.Join(root, "bear.lock.yml"), "new lock\n")
			switch scenario {
			case "consistent":
				t.Setenv("BRANCH_NAME", "main")
				t.Setenv("GIT_LOCAL_BRANCH", "main")
				t.Setenv("GIT_BRANCH", "origin/main")
			case "prefixed":
				t.Setenv("GIT_BRANCH", "origin/main")
			case "PR":
				t.Setenv("BRANCH_NAME", "PR-123")
			case "change":
				t.Setenv("BRANCH_NAME", "main")
				t.Setenv("CHANGE_ID", "123")
			case "ambiguous":
				t.Setenv("BRANCH_NAME", "main")
				t.Setenv("GIT_BRANCH", "origin/other")
			case "unknown remote":
				t.Setenv("GIT_BRANCH", "upstream/main")
			case "invalid":
				t.Setenv("BRANCH_NAME", "main:evil")
			}
			err := commitLockFile(context.Background(), root, filepath.Join(root, "bear.lock.yml"), nil, "", "")
			if scenario == "consistent" || scenario == "prefixed" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "--git-branch") {
				t.Fatalf("expected actionable branch rejection, got %v", err)
			}
		})
	}
}

func TestCommitLockFileCancellation(t *testing.T) {
	root, _ := gitFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := commitLockFile(ctx, root, filepath.Join(root, "bear.lock.yml"), nil, "", "main")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	if runtime.GOOS == "windows" {
		return
	}
	index := gitFixtureStaged(t, root)
	hook := filepath.Join(root, ".git", "hooks", "pre-commit")
	gitFixtureWrite(t, hook, "#!/bin/sh\nexec sleep 10\n")
	if err := os.Chmod(hook, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := time.Now()
	err = commitLockFile(ctx, root, filepath.Join(root, "bear.lock.yml"), nil, "", "main")
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 4*time.Second {
		t.Fatalf("cancellation did not return promptly: %v (%s)", err, time.Since(start))
	}
	gitFixtureAssertPreserved(t, root, index)
}

func TestSanitizeGitError(t *testing.T) {
	remote := "https://token-user:secret-password@example.invalid/repo?access_token=query-secret"
	text := "authentication failed for " + remote + "; user token-user password secret-password query-secret; also ssh://other:other-secret@host/repo"
	got := sanitizeGitError(text, remote)
	for _, secret := range []string{"token-user", "secret-password", "query-secret", "other-secret"} {
		if strings.Contains(got, secret) {
			t.Errorf("secret %q leaked: %s", secret, got)
		}
	}
	if !strings.Contains(got, "authentication failed") {
		t.Errorf("actionable diagnostic lost: %s", got)
	}
}

func TestCommitLockFileNewAndUnchangedLock(t *testing.T) {
	for _, newLock := range []bool{false, true} {
		t.Run(map[bool]string{false: "unchanged", true: "new"}[newLock], func(t *testing.T) {
			root, _ := gitFixture(t)
			lock := filepath.Join(root, "bear.lock.yml")
			if newLock {
				lock = filepath.Join(root, "new.lock.yml")
				gitFixtureWrite(t, lock, "new lock\n")
			}
			before := gitFixtureCommand(t, root, "rev-parse", "HEAD")
			if err := commitLockFile(context.Background(), root, lock, nil, "", ""); err != nil {
				t.Fatal(err)
			}
			if after := gitFixtureCommand(t, root, "rev-parse", "HEAD"); (after != before) != newLock {
				t.Errorf("commit created = %v, want %v", after != before, newLock)
			}
		})
	}
}

func TestCommitLockFileIgnoresRepositoryEnvironment(t *testing.T) {
	root, _ := gitFixture(t)
	index := gitFixtureStaged(t, root)
	other := t.TempDir()
	otherIndex := filepath.Join(other, "index")
	gitFixtureWrite(t, otherIndex, "caller-owned index")
	for key, value := range map[string]string{
		"GIT_DIR": other, "GIT_WORK_TREE": other, "GIT_COMMON_DIR": other,
		"GIT_INDEX_FILE": otherIndex, "GIT_NAMESPACE": "unrelated",
		"GIT_OBJECT_DIRECTORY": other, "GIT_ALTERNATE_OBJECT_DIRECTORIES": other,
	} {
		t.Setenv(key, value)
	}
	if err := commitLockFile(context.Background(), root, filepath.Join(root, "bear.lock.yml"), nil, "", "main"); err != nil {
		t.Fatal(err)
	}
	gitFixtureAssertPreserved(t, root, index)
	got, err := os.ReadFile(otherIndex)
	if err != nil || string(got) != "caller-owned index" {
		t.Fatalf("inherited index modified: %q, %v", got, err)
	}
}

func TestCommitLockFileRejectsMultiplePushURLs(t *testing.T) {
	root, remote := gitFixture(t)
	// Config writes are fixture setup only; the helper must never write config.
	gitFixtureCommand(t, root, "config", "--add", "remote.origin.pushurl", remote)
	gitFixtureCommand(t, root, "config", "--add", "remote.origin.pushurl", t.TempDir())
	index := gitFixtureStaged(t, root)
	err := commitLockFile(context.Background(), root, filepath.Join(root, "bear.lock.yml"), nil, "", "main")
	if err == nil || !strings.Contains(err.Error(), "exactly one push URL") {
		t.Fatalf("expected multiple push URL rejection, got %v", err)
	}
	gitFixtureAssertPreserved(t, root, index)
}

func TestCommitLockFileConcurrentHEAD(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Git interception fixture requires a Unix shell")
	}
	for _, phase := range []string{"before commit", "after commit", "before push"} {
		t.Run(phase, func(t *testing.T) {
			root, remote := gitFixture(t)
			index := gitFixtureStaged(t, root)
			before := gitFixtureCommand(t, root, "rev-parse", "HEAD")
			// Build an unrelated commit without changing HEAD or the caller's index.
			blob := gitFixtureCommand(t, root, "rev-parse", ":unrelated")
			treeCmd := exec.Command("git", "mktree")
			treeCmd.Dir = root
			treeCmd.Stdin = strings.NewReader("100644 blob " + blob + "\tunrelated\n")
			treeOut, err := treeCmd.Output()
			if err != nil {
				t.Fatal(err)
			}
			tree := strings.TrimSpace(string(treeOut))
			unverified := gitFixtureCommand(t, root, "commit-tree", tree, "-p", before, "-m", "concurrent unrelated change")
			realGit, err := exec.LookPath("git")
			if err != nil {
				t.Fatal(err)
			}
			bin := t.TempDir()
			wrapper := filepath.Join(bin, "git")
			gitFixtureWrite(t, wrapper, `#!/bin/sh
set -eu
case "$BEAR_TEST_GIT_PHASE:$1" in
  'before commit:write-tree'|'after commit:commit')
    "$BEAR_TEST_REAL_GIT" "$@"
    "$BEAR_TEST_REAL_GIT" update-ref HEAD "$BEAR_TEST_UNVERIFIED"
    exit 0
    ;;
  'before push:push')
    for arg in "$@"; do
      printf '%s\n' "$arg" >> "$BEAR_TEST_PUSH_ARGS"
    done
    "$BEAR_TEST_REAL_GIT" update-ref HEAD "$BEAR_TEST_UNVERIFIED"
    ;;
esac
exec "$BEAR_TEST_REAL_GIT" "$@"
`)
			if err := os.Chmod(wrapper, 0700); err != nil {
				t.Fatal(err)
			}
			argsPath := filepath.Join(bin, "push-args")
			t.Setenv("BEAR_TEST_REAL_GIT", realGit)
			t.Setenv("BEAR_TEST_GIT_PHASE", phase)
			t.Setenv("BEAR_TEST_UNVERIFIED", unverified)
			t.Setenv("BEAR_TEST_PUSH_ARGS", argsPath)
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			release, err := acquireRepositoryLock(context.Background(), root)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			err = commitLockFile(context.Background(), root, filepath.Join(root, "bear.lock.yml"), nil, "", "main")
			gitFixtureAssertPreserved(t, root, index)
			published := gitFixtureCommand(t, remote, "rev-parse", "main")
			if phase != "before push" {
				if err == nil || !strings.Contains(err.Error(), "refusing to") {
					t.Fatalf("expected concurrent HEAD rejection, got %v", err)
				}
				if published != before {
					t.Fatalf("unverified commit published: %s", published)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if published == before || published == unverified {
				t.Fatalf("did not publish the verified lock commit: %s", published)
			}
			if head := gitFixtureCommand(t, root, "rev-parse", "HEAD"); head != unverified {
				t.Fatalf("concurrent HEAD update did not occur: %s", head)
			}
			args, err := os.ReadFile(argsPath)
			if err != nil || !strings.Contains(string(args), published+":refs/heads/main\n") || strings.Contains(string(args), "HEAD:") {
				t.Fatalf("push did not use explicit verified SHA: %s, %v", args, err)
			}
			if paths := gitFixtureCommand(t, remote, "diff-tree", "--no-commit-id", "--name-only", "-r", "main"); paths != "bear.lock.yml" {
				t.Fatalf("published unrelated paths: %s", paths)
			}
		})
	}
}
