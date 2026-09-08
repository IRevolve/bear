package internal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// ChangedFile represents a changed file. Renames include both endpoints.
type ChangedFile struct {
	Path   string
	Status string // A=Added, M=Modified, D=Deleted, R=Renamed
}

func gitOutput(rootPath string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = rootPath
	// Repository discovery must use rootPath, not a caller's Git process state.
	cmd.Env = []string{}
	for _, variable := range os.Environ() {
		key, _, _ := strings.Cut(variable, "=")
		switch key {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_PREFIX", "GIT_NAMESPACE":
			continue
		}
		cmd.Env = append(cmd.Env, variable)
	}
	output, err := cmd.Output()
	if err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(exit.Stderr)))
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(output), nil
}

// ResolveCommit resolves a revision to a commit, failing for missing repositories
// and invalid revisions rather than manufacturing deployment history.
func ResolveCommit(rootPath, revision string) (string, error) {
	output, err := gitOutput(rootPath, "rev-parse", "--verify", "--end-of-options", revision+"^{commit}")
	return strings.TrimSpace(output), err
}

// GetCurrentCommit returns HEAD, or an empty string if it cannot be resolved.
// Callers that need the error should use ResolveCommit.
func GetCurrentCommit(rootPath string) string {
	commit, _ := ResolveCommit(rootPath, "HEAD")
	return commit
}

// workspaceFiles converts Git-root-relative paths to workspace-relative paths.
func workspaceFiles(rootPath string, files []ChangedFile) ([]ChangedFile, error) {
	output, err := gitOutput(rootPath, "rev-parse", "--show-prefix")
	if err != nil {
		return nil, err
	}
	prefix := strings.TrimSuffix(output, "\n")
	seen := make(map[string]bool)
	var result []ChangedFile
	for _, file := range files {
		if !strings.HasPrefix(file.Path, prefix) {
			continue
		}
		file.Path = strings.TrimPrefix(file.Path, prefix)
		// Bear's deployment bookkeeping is not artifact source, even when tracked.
		if filepath.Base(file.Path) == "bear.lock.yml" || slices.Contains(strings.Split(file.Path, "/"), ".bear") {
			continue
		}
		if !seen[file.Path] {
			seen[file.Path] = true
			result = append(result, file)
		}
	}
	return result, nil
}

// GetChangedFilesBetweenCommits returns workspace-relative changes between commits.
func GetChangedFilesBetweenCommits(rootPath, fromCommit, toCommit string) ([]ChangedFile, error) {
	from, err := ResolveCommit(rootPath, fromCommit)
	if err != nil {
		return nil, err
	}
	to, err := ResolveCommit(rootPath, toCommit)
	if err != nil {
		return nil, err
	}
	output, err := gitOutput(rootPath, "diff", "--name-status", "-z", "--no-relative", "--find-renames", from, to, "--")
	if err != nil {
		return nil, err
	}
	files, err := parseGitDiff(output)
	if err != nil {
		return nil, err
	}
	return workspaceFiles(rootPath, files)
}

// GetUncommittedChanges returns all staged, unstaged and untracked files.
func GetUncommittedChanges(rootPath string) ([]ChangedFile, error) {
	var allFiles []ChangedFile
	for _, args := range [][]string{
		{"diff", "--name-status", "-z", "--no-relative", "--find-renames", "--cached", "--"},
		{"diff", "--name-status", "-z", "--no-relative", "--find-renames", "--"},
		{"ls-files", "--others", "--exclude-standard", "--full-name", "-z"},
	} {
		output, err := gitOutput(rootPath, args...)
		if err != nil {
			return nil, err
		}
		if args[0] == "ls-files" {
			for _, path := range strings.Split(output, "\x00") {
				if path != "" {
					allFiles = append(allFiles, ChangedFile{Status: "A", Path: path})
				}
			}
		} else {
			files, err := parseGitDiff(output)
			if err != nil {
				return nil, err
			}
			allFiles = append(allFiles, files...)
		}
	}
	return workspaceFiles(rootPath, allFiles)
}

func parseGitDiff(output string) ([]ChangedFile, error) {
	var files []ChangedFile
	if output == "" {
		return files, nil
	}
	if !strings.HasSuffix(output, "\x00") {
		return nil, fmt.Errorf("unterminated git diff record")
	}
	parts := strings.Split(strings.TrimSuffix(output, "\x00"), "\x00")
	for i := 0; i < len(parts); {
		status := parts[i]
		i++
		count := 1
		if strings.HasPrefix(status, "R") || strings.HasPrefix(status, "C") {
			count = 2
		}
		if status == "" || i+count > len(parts) {
			return nil, fmt.Errorf("malformed git diff record for status %q", status)
		}
		for j := 0; j < count; j++ {
			if parts[i] == "" {
				return nil, fmt.Errorf("empty path in git diff record")
			}
			// A copy does not modify its source; a rename does.
			if !strings.HasPrefix(status, "C") || j == 1 {
				files = append(files, ChangedFile{Status: status, Path: parts[i]})
			}
			i++
		}
	}
	return files, nil
}

// GetAffectedDirs returns all directories affected by changes, including root.
func GetAffectedDirs(files []ChangedFile) map[string]bool {
	dirs := make(map[string]bool)
	for _, f := range files {
		for dir := filepath.Dir(f.Path); ; dir = filepath.Dir(dir) {
			dirs[dir] = true
			if dir == "." || dir == filepath.Dir(dir) {
				break
			}
		}
	}
	return dirs
}
