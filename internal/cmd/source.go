package cmd

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

func sourceGit(ctx context.Context, root string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"--no-optional-locks"}, args...)...)
	cmd.Dir = root
	// Git-invoking tools/hooks can export these; they must not redirect a
	// source check or private checkout to another repository or index.
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch key {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_PREFIX", "GIT_NAMESPACE":
			continue
		}
		cmd.Env = append(cmd.Env, entry)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func sourceRepository(ctx context.Context, root string) (string, string, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return "", "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", "", err
	}
	out, err := sourceGit(ctx, root, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", "", err
	}
	repo, err := filepath.EvalSymlinks(strings.TrimSuffix(string(out), "\n"))
	if err != nil {
		return "", "", err
	}
	rel, err := filepath.Rel(repo, root)
	if err != nil || !filepath.IsLocal(rel) {
		return "", "", fmt.Errorf("project root %q is outside repository %q", root, repo)
	}
	return repo, rel, nil
}

// prepareSource materializes a revision in a private detached worktree, preserving
// the project's repository-relative location. It never copies dirty or ignored
// files (including dependencies/build outputs) from the caller's workspace.
// On success cleanup is non-nil and must be called, even after cancellation.
func prepareSource(ctx context.Context, projectRoot, revision string) (sourceRoot, resolvedCommit string, cleanup func() error, err error) {
	repo, rel, err := sourceRepository(ctx, projectRoot)
	if err != nil {
		return "", "", nil, err
	}
	out, err := sourceGit(ctx, repo, "rev-parse", "--verify", "--end-of-options", revision+"^{commit}")
	if err != nil {
		return "", "", nil, err
	}
	commit := strings.TrimSpace(string(out))
	temp, err := os.MkdirTemp("", "bear-source-")
	if err != nil {
		return "", "", nil, err
	}
	worktree := filepath.Join(temp, "worktree")
	registered, removed := false, false
	cleanup = func() error {
		if removed {
			return nil
		}
		// Cleanup must not inherit the canceled validation/deployment context.
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if registered {
			if _, err := sourceGit(cleanupCtx, repo, "worktree", "remove", "--force", "--force", worktree); err != nil {
				return err // Retain the directory so cleanup can be retried.
			}
			registered = false
		}
		if err := os.RemoveAll(temp); err != nil {
			return err
		}
		removed = true
		return nil
	}
	// No checkout hook is invoked: populate the private index/tree directly.
	if _, err = sourceGit(ctx, repo, "worktree", "add", "--detach", "--no-checkout", worktree, commit); err != nil {
		// A canceled add can leave a registered worktree behind.
		_, statErr := os.Stat(filepath.Join(worktree, ".git"))
		registered = statErr == nil
		return "", "", nil, errors.Join(err, cleanup())
	}
	registered = true
	if _, err = sourceGit(ctx, worktree, "read-tree", "--no-sparse-checkout", "--reset", "-u", commit); err == nil {
		sourceRoot, err = safeArtifactPath(worktree, filepath.ToSlash(rel))
		if err == nil {
			var info os.FileInfo
			info, err = os.Stat(sourceRoot)
			if err == nil && !info.IsDir() {
				err = fmt.Errorf("project root %q is not a directory at %s", rel, commit)
			}
		}
	}
	if err != nil {
		return "", "", nil, errors.Join(err, cleanup())
	}
	return sourceRoot, commit, cleanup, nil
}

// Bear state is excluded at any repository depth, including nested workspaces.
func sourceExcluded(path string) bool {
	return filepath.Base(path) == "bear.lock.yml" || slices.Contains(strings.Split(filepath.ToSlash(path), "/"), ".bear")
}

// sourceFingerprint hashes the entire repository's tracked and nonignored
// untracked working files, not index contents. Names, Git-style modes, symlink
// targets and contents are included; absolute paths and timestamps are not.
// Deleted tracked paths are represented explicitly. Bear state is excluded.
// Ignored untracked dependencies are deliberately outside this contract.
// Submodules/nested repositories, special files, and index flags that hide
// changes fail closed. This is not an atomic filesystem snapshot: callers must
// keep source quiescent while checking.
func sourceFingerprint(ctx context.Context, root string) (string, error) {
	repo, _, err := sourceRepository(ctx, root)
	if err != nil {
		return "", err
	}
	tracked, err := sourceGit(ctx, repo, "ls-files", "--stage", "-v", "-z")
	if err != nil {
		return "", err
	}
	paths := map[string]bool{}
	for _, entry := range bytes.Split(tracked, []byte{0}) {
		if len(entry) == 0 {
			continue
		}
		metadata, path, ok := strings.Cut(string(entry), "\t")
		if !ok {
			return "", fmt.Errorf("invalid Git index entry")
		}
		if sourceExcluded(path) {
			continue
		}
		if len(metadata) < 2 || metadata[0] == 'S' || metadata[0] >= 'a' && metadata[0] <= 'z' {
			return "", fmt.Errorf("cannot verify source %q with skip-worktree or assume-unchanged index flags", path)
		}
		if strings.HasPrefix(metadata[2:], "160000 ") {
			return "", fmt.Errorf("cannot fingerprint submodule %q", path)
		}
		paths[path] = true
	}
	untracked, err := sourceGit(ctx, repo, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return "", err
	}
	for _, path := range bytes.Split(untracked, []byte{0}) {
		if len(path) > 0 && !sourceExcluded(string(path)) {
			paths[string(path)] = true
		}
	}
	names := make([]string, 0, len(paths))
	for path := range paths {
		names = append(names, path)
	}
	slices.Sort(names)
	hash := sha256.New()
	io.WriteString(hash, "bear-source-v1\x00")
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		// Resolve parents separately: hash a leaf symlink's target, never its
		// referent, and never follow a replaced parent out of the repository.
		parent, err := safeArtifactPath(repo, filepath.ToSlash(filepath.Dir(name)))
		if err != nil {
			return "", err
		}
		path := filepath.Join(parent, filepath.Base(name))
		info, err := os.Lstat(path)
		mode, content := "missing", ""
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if err == nil {
			switch {
			case info.Mode()&os.ModeSymlink != 0:
				mode = "120000"
				content, err = os.Readlink(path)
			case info.Mode().IsRegular():
				mode = "100644"
				if info.Mode().Perm()&0111 != 0 {
					mode = "100755"
				}
				var file *os.File
				file, err = os.Open(path)
				if err == nil {
					fileHash := sha256.New()
					_, copyErr := io.Copy(fileHash, file)
					err = errors.Join(copyErr, file.Close())
					content = fmt.Sprintf("%x", fileHash.Sum(nil))
				}
			default:
				return "", fmt.Errorf("cannot fingerprint non-file source %q", name)
			}
			if err != nil {
				return "", err
			}
		}
		fmt.Fprintf(hash, "%d:%s%d:%s%d:%s", len(name), name, len(mode), mode, len(content), content)
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// sourceState returns full HEAD, the working-source fingerprint, and whether
// tracked/index or nonignored untracked source differs from HEAD. Dirty source
// is reported, not rejected; only bear.lock.yml and .bear state are exempt.
// Callers should reject dirty initial production plans and compare BOTH commit
// and the post-validation fingerprint before any deployment. Generated outputs
// may legitimately make the post-validation state dirty.
func sourceState(ctx context.Context, root string) (commit, fingerprint string, dirty bool, err error) {
	repo, _, err := sourceRepository(ctx, root)
	if err != nil {
		return "", "", false, err
	}
	out, err := sourceGit(ctx, repo, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return "", "", false, err
	}
	commit = strings.TrimSpace(string(out))
	fingerprint, err = sourceFingerprint(ctx, repo)
	if err != nil {
		return "", "", false, err
	}
	out, err = sourceGit(ctx, repo, "-c", "status.renames=false", "-c", "core.fileMode=true", "status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return "", "", false, err
	}
	for _, entry := range bytes.Split(out, []byte{0}) {
		if len(entry) > 3 && !sourceExcluded(string(entry[3:])) {
			dirty = true
		}
	}
	out, err = sourceGit(ctx, repo, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return "", "", false, err
	}
	if strings.TrimSpace(string(out)) != commit {
		return "", "", false, fmt.Errorf("HEAD changed while checking source")
	}
	return commit, fingerprint, dirty, nil
}

// safeArtifactPath resolves a portable relative path beneath root. Existing
// symlinks must stay inside root; missing suffixes are allowed for build outputs.
// This is a preflight check, not protection against concurrent symlink swaps.
func safeArtifactPath(root, rel string) (string, error) {
	if !filepath.IsLocal(rel) || strings.ContainsAny(rel, "\\:\x00") {
		return "", fmt.Errorf("unsafe artifact path %q", rel)
	}
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if part == ".." {
			return "", fmt.Errorf("unsafe artifact path %q", rel)
		}
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	path := root
	for _, part := range strings.Split(filepath.ToSlash(filepath.Clean(rel)), "/") {
		path = filepath.Join(path, part)
		_, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		path, err = filepath.EvalSymlinks(path)
		if err != nil {
			return "", err
		}
		inside, err := filepath.Rel(root, path)
		if err != nil || !filepath.IsLocal(inside) {
			return "", fmt.Errorf("artifact path %q escapes source root", rel)
		}
	}
	return path, nil
}
