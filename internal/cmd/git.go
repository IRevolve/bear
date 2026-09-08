package cmd

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/irevolve/bear/internal/config"
)

// commitLockFile commits the working-tree lock using a private index, runs the
// normal Git hooks, and pushes only the verified commit SHA to an existing remote
// branch, never a mutable HEAD ref. The real
// index is never written, including its lock entry: staged lock content survives
// exactly, but may appear as a reverse change relative to the new HEAD. Callers
// must hold acquireRepositoryLock to serialize cooperating repository writers.
// External Git writers do not honor that lock; parent/tree validation and the
// immutable push source prevent publishing their unverified commits, but cannot
// prevent local HEAD changes. A failed push leaves the local commit for
// manual recovery; we never reset, force push, change identity, or disable hooks.
func commitLockFile(ctx context.Context, rootPath, lockPath string, deployed []config.PlanArtifact, remote, branch string) error {
	var index, pushURL string
	run := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "git", args...)
		cmd.Dir = rootPath
		// Repository/index overrides from a surrounding Git process must not
		// redirect a workspace commit. Keep authentication and identity settings.
		for _, value := range os.Environ() {
			key, _, _ := strings.Cut(value, "=")
			switch key {
			case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_NAMESPACE":
				continue
			}
			cmd.Env = append(cmd.Env, value)
		}
		cmd.Env = append(cmd.Env, "GIT_TERMINAL_PROMPT=0", "GIT_LITERAL_PATHSPECS=1")
		if index != "" {
			cmd.Env = append(cmd.Env, "GIT_INDEX_FILE="+index)
		}
		cmd.WaitDelay = time.Second
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			if ctx.Err() != nil {
				err = ctx.Err()
			}
			return "", fmt.Errorf("git %s failed: %w: %s", args[0], err, sanitizeGitError(strings.TrimSpace(stderr.String()), pushURL))
		}
		return strings.TrimSpace(stdout.String()), nil
	}
	if remote == "" {
		remote = "origin"
	}
	if strings.HasPrefix(remote, "-") || strings.ContainsAny(remote, "\r\n") {
		return fmt.Errorf("invalid Git remote; use a configured remote name with --git-remote")
	}
	var err error
	pushURL, err = run("remote", "get-url", "--push", "--all", remote)
	if err != nil {
		return err
	}
	if pushURL == "" || strings.ContainsAny(pushURL, "\r\n") || strings.HasPrefix(pushURL, "-") {
		return fmt.Errorf("Git remote must have exactly one push URL")
	}
	if branch == "" {
		branch, err = run("symbolic-ref", "--quiet", "--short", "HEAD")
		if err != nil {
			branch, err = jenkinsGitBranch(remote)
			if err != nil {
				return err
			}
		}
	}
	if branch == "" || strings.HasPrefix(branch, "-") || strings.HasPrefix(branch, "refs/") || branch == "HEAD" {
		return fmt.Errorf("invalid Git branch; specify --git-branch with a branch name")
	}
	if _, err := run("check-ref-format", "refs/heads/"+branch); err != nil {
		return fmt.Errorf("invalid Git branch; specify --git-branch: %w", err)
	}
	head, err := run("rev-parse", "--verify", "HEAD")
	if err != nil {
		return err
	}
	tip, err := run("ls-remote", "--refs", "--", pushURL, "refs/heads/"+branch)
	if err != nil {
		return err
	}
	if tip != head+"\trefs/heads/"+branch {
		return fmt.Errorf("unsafe Git push: target branch is missing or its tip differs from HEAD; synchronize the checkout first (refusing to publish unrelated commits)")
	}
	root, err := filepath.Abs(rootPath)
	if err != nil {
		return err
	}
	lock, err := filepath.Abs(lockPath)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, lock)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("lock file must be inside the workspace root")
	}
	info, err := os.Lstat(lock)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("lock file must be a regular file")
	}
	tmp, err := os.MkdirTemp("", "bear-git-index-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	index = filepath.Join(tmp, "index")
	if _, err := run("read-tree", head); err != nil {
		return err
	}
	if _, err := run("add", "--", filepath.ToSlash(rel)); err != nil {
		return err
	}
	tree, err := run("write-tree")
	if err != nil {
		return err
	}
	oldTree, err := run("rev-parse", head+"^{tree}")
	if err != nil {
		return err
	}
	if tree == oldTree {
		return nil
	}
	var names []string
	for _, artifact := range deployed {
		names = append(names, artifact.Name)
	}
	message := "chore(bear): update lock file [skip ci]\n\nDeployed: " + strings.Join(names, ", ")
	currentHead, err := run("rev-parse", "--verify", "HEAD")
	if err != nil {
		return err
	}
	if currentHead != head {
		return fmt.Errorf("HEAD changed before lock commit; refusing to commit, inspect HEAD manually")
	}
	if _, err := run("commit", "-m", message); err != nil {
		return err
	}
	commit, err := run("rev-parse", "--verify", "HEAD")
	if err != nil {
		return err
	}
	committed, err := run("show", "--no-patch", "--format=%T %P", commit)
	if err != nil {
		return err
	}
	if committed != tree+" "+head {
		return fmt.Errorf("Git hooks or a concurrent writer changed the commit; refusing to push, inspect HEAD manually")
	}
	// Use the single resolved push URL to avoid remote mirror/multiple-push-URL
	// configuration, and explicitly disable automatic tag/submodule publication.
	if _, err := run("push", "--no-follow-tags", "--recurse-submodules=no", "--", pushURL, commit+":refs/heads/"+branch); err != nil {
		return fmt.Errorf("lock commit %s created locally but not pushed; inspect it and retry the push manually: %w", commit, err)
	}
	return nil
}

// Jenkins hints are used only for detached HEAD. Never guess a remote prefix,
// reconcile conflicting hints, or treat a PR build's synthetic ref as a branch.
func jenkinsGitBranch(remote string) (string, error) {
	if os.Getenv("CHANGE_ID") != "" || os.Getenv("CHANGE_BRANCH") != "" || os.Getenv("CHANGE_TARGET") != "" {
		return "", fmt.Errorf("detached PR checkout: specify --git-branch explicitly")
	}
	branch := ""
	for _, key := range []string{"BRANCH_NAME", "GIT_LOCAL_BRANCH", "GIT_BRANCH"} {
		value := os.Getenv(key)
		if value == "" {
			continue
		}
		if key == "GIT_BRANCH" {
			if !strings.HasPrefix(value, remote+"/") {
				return "", fmt.Errorf("ambiguous Jenkins GIT_BRANCH; specify --git-branch explicitly")
			}
			value = strings.TrimPrefix(value, remote+"/")
		}
		lower := strings.ToLower(value)
		if strings.HasPrefix(lower, "pr-") || strings.HasPrefix(lower, "pull/") || strings.HasPrefix(lower, "merge-requests/") || strings.HasPrefix(lower, "refs/") || (branch != "" && branch != value) {
			return "", fmt.Errorf("PR or ambiguous Jenkins branch hints; specify --git-branch explicitly")
		}
		branch = value
	}
	if branch == "" {
		return "", fmt.Errorf("detached HEAD has no safe branch hint; specify --git-branch explicitly")
	}
	return branch, nil
}

var gitURLPattern = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^\s'"<>]+`)

func sanitizeGitError(text, remoteURL string) string {
	text = gitURLPattern.ReplaceAllString(text, "[redacted-url]")
	if remoteURL != "" {
		text = strings.ReplaceAll(text, remoteURL, "[redacted-remote]")
		if parsed, err := url.Parse(remoteURL); err == nil {
			var secrets []string
			if parsed.User != nil {
				password, _ := parsed.User.Password()
				secrets = append(secrets, parsed.User.Username(), password)
			}
			for _, values := range parsed.Query() {
				secrets = append(secrets, values...)
			}
			for _, secret := range secrets {
				if secret != "" {
					text = strings.ReplaceAll(text, url.QueryEscape(secret), "[redacted]")
					text = strings.ReplaceAll(text, url.PathEscape(secret), "[redacted]")
					text = strings.ReplaceAll(text, secret, "[redacted]")
				}
			}
		}
	}
	return text
}
