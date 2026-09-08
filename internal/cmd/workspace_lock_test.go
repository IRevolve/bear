package cmd

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceLock(t *testing.T) {
	root := t.TempDir()
	release, err := acquireWorkspaceLock(root)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if other, err := acquireWorkspaceLock(root); err == nil {
		other()
		t.Fatal("second acquisition succeeded")
	}
	// Separate project roots within a repository have independent lock scopes.
	nested, err := acquireWorkspaceLock(filepath.Join(root, "nested"))
	if err != nil {
		t.Fatal(err)
	}
	if err := nested(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := release(); err != nil {
			t.Fatal(err)
		}
	}
	release, err = acquireWorkspaceLock(root)
	if err != nil {
		t.Fatalf("persistent lock file prevented reacquisition: %v", err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
}

func TestWorkspaceLockProcess(t *testing.T) {
	if root := os.Getenv("BEAR_TEST_LOCK_ROOT"); root != "" {
		_, err := acquireWorkspaceLock(root)
		if os.Getenv("BEAR_TEST_LOCK_HELD") == "1" {
			if err == nil {
				t.Fatal("child acquired parent's lock")
			}
		} else if err != nil {
			t.Fatal(err)
		}
		// Deliberately do not release: process exit must release the OS lock.
		return
	}
	root := t.TempDir()
	release, err := acquireWorkspaceLock(root)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	for _, held := range []string{"1", "0"} {
		if held == "0" {
			if err := release(); err != nil {
				t.Fatal(err)
			}
		}
		cmd := exec.Command(os.Args[0], "-test.run=^TestWorkspaceLockProcess$")
		cmd.Env = append(os.Environ(), "BEAR_TEST_LOCK_ROOT="+root, "BEAR_TEST_LOCK_HELD="+held)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("child: %v\n%s", err, out)
		}
	}
	release, err = acquireWorkspaceLock(root)
	if err != nil {
		t.Fatalf("process exit left stale lock: %v", err)
	}
	release()
}

func TestWorkspaceLockInvalidRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(root, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := acquireWorkspaceLock(root); err == nil || !strings.Contains(err.Error(), "workspace lock directory") {
		t.Fatalf("expected actionable root error, got %v", err)
	}
}

func TestRepositoryLockSharedScopes(t *testing.T) {
	root, independent := gitFixture(t)
	siblingA, siblingB := filepath.Join(root, "a"), filepath.Join(root, "b")
	for _, dir := range []string{siblingA, siblingB} {
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	linked := filepath.Join(t.TempDir(), "linked")
	gitFixtureCommand(t, root, "worktree", "add", "--detach", linked, "HEAD")
	for _, holder := range []string{siblingA, linked} {
		release, err := acquireRepositoryLock(context.Background(), holder)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { release() })
		for _, contender := range []string{root, siblingA, siblingB, linked} {
			if other, err := acquireRepositoryLock(context.Background(), contender); err == nil {
				other()
				t.Fatalf("%s acquired repository lock held by %s", contender, holder)
			} else if !strings.Contains(err.Error(), "another bear process") {
				t.Fatalf("unexpected lock error: %v", err)
			}
		}
		other, err := acquireRepositoryLock(context.Background(), independent)
		if err != nil {
			t.Fatalf("independent repository blocked: %v", err)
		}
		if err := other(); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 2; i++ {
			if err := release(); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".git", "bear.repository.lock")); err != nil {
		t.Fatalf("missing persistent common-directory lock: %v", err)
	}
}

func TestRepositoryLockSanitizesEnvironment(t *testing.T) {
	root, other := gitFixture(t)
	release, err := acquireRepositoryLock(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	for key, value := range map[string]string{"GIT_DIR": other, "GIT_COMMON_DIR": other, "GIT_WORK_TREE": other} {
		t.Setenv(key, value)
	}
	if release, err := acquireRepositoryLock(context.Background(), root); err == nil {
		release()
		t.Fatal("Git environment redirected repository lock")
	} else if !strings.Contains(err.Error(), "another bear process") {
		t.Fatalf("unexpected lock error: %v", err)
	}
}

func TestRepositoryLockPreflightAndCancellation(t *testing.T) {
	root, _ := gitFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if release, err := acquireRepositoryLock(ctx, root); err == nil {
		release()
		t.Fatal("canceled acquisition succeeded")
	} else if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	nonRepo := t.TempDir()
	if release, err := acquireRepositoryLock(context.Background(), nonRepo); err == nil {
		release()
		t.Fatal("non-repository silently fell back to a weaker lock")
	}
	// Workspace locking remains safe before preflight even with no Git executable.
	t.Setenv("PATH", t.TempDir())
	release, err := acquireWorkspaceLock(nonRepo)
	if err != nil {
		t.Fatal(err)
	}
	if err := release(); err != nil {
		t.Fatal(err)
	}
}
