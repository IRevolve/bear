package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// acquireWorkspaceLock takes a nonblocking advisory lock for this workspace
// root. All writers must cooperate. Different roots (including nested projects
// in one repository) are separate scopes; callers sharing state must use the
// same root. This helper runs no subprocesses and can precede config/preflight
// validation. Repository writers must ALSO acquire acquireRepositoryLock after
// preflight. The persistent .bear/workspace.lock must never be removed: the OS
// releases the lock on close or process exit, so its existence is not staleness.
// The returned release function is safe to call more than once.
func acquireWorkspaceLock(root string) (func() error, error) {
	dir := filepath.Join(root, ".bear")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create workspace lock directory: %w", err)
	}
	release, err := acquireLockFile(filepath.Join(dir, "workspace.lock"))
	if err != nil {
		return nil, fmt.Errorf("cannot lock workspace %s: %w", root, err)
	}
	return release, nil
}

// acquireRepositoryLock serializes cooperating repository writers, including
// sibling/nested projects and linked worktrees, using the common Git directory.
// Call after preflight (this runs Git), after acquiring the workspace lock, and
// before preparing source. Hold through Git publication and source cleanup; release
// in reverse acquisition order. Non-repositories return an error, not a weaker
// project-local fallback. Git itself does not honor this advisory lock.
func acquireRepositoryLock(ctx context.Context, root string) (func() error, error) {
	out, err := sourceGit(ctx, root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("resolve repository lock directory: %w", ctx.Err())
		}
		return nil, fmt.Errorf("resolve repository lock directory: %s", sanitizeGitError(err.Error(), ""))
	}
	dir := strings.TrimSuffix(string(out), "\n")
	if !filepath.IsAbs(dir) {
		return nil, fmt.Errorf("Git returned a non-absolute repository lock directory")
	}
	return acquireLockFile(filepath.Join(dir, "bear.repository.lock"))
}

// Lock files persist so all contenders use the same inode. Close/process exit
// releases the OS lock; release is idempotent for cleanup on every error path.
func acquireLockFile(path string) (func() error, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open advisory lock: %w", err)
	}
	if err := lockWorkspaceFile(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("cannot acquire lock %s (another bear process may be running): %w", path, err)
	}
	var once sync.Once
	var releaseErr error
	return func() error {
		once.Do(func() { releaseErr = f.Close() })
		return releaseErr
	}, nil
}
