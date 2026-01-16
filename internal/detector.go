package internal

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// ChangedFile represents a changed file
type ChangedFile struct {
	Path   string
	Status string // A=Added, M=Modified, D=Deleted, R=Renamed
}

// getGitRoot returns the root directory of the Git repository
func getGitRoot(path string) string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// GetChangedFilesBetweenCommits returns changed files between two commits
func GetChangedFilesBetweenCommits(rootPath string, fromCommit, toCommit string) ([]ChangedFile, error) {
	cmd := exec.Command("git", "diff", "--name-status", "--ignore-space-change", "--ignore-blank-lines", fromCommit+".."+toCommit)
	cmd.Dir = rootPath

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	return parseGitDiff(string(output)), nil
}

// GetCurrentCommit returns the current HEAD commit
func GetCurrentCommit(rootPath string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = rootPath

	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(output))
}

// GetUncommittedChanges returns all uncommitted and untracked files
func GetUncommittedChanges(rootPath string) ([]ChangedFile, error) {
	gitRoot := getGitRoot(rootPath)
	var allFiles []ChangedFile

	// 1. Staged changes
	cmd := exec.Command("git", "diff", "--name-status", "--cached", "--ignore-space-change", "--ignore-blank-lines")
	cmd.Dir = rootPath
	output, err := cmd.Output()
	if err != nil {
		// Log but continue - partial results are still useful
		Warn("failed to get staged changes", "error", err)
	} else {
		allFiles = append(allFiles, parseGitDiff(string(output))...)
	}

	// 2. Unstaged changes
	cmd = exec.Command("git", "diff", "--name-status", "--ignore-space-change", "--ignore-blank-lines")
	cmd.Dir = rootPath
	output, err = cmd.Output()
	if err != nil {
		Warn("failed to get unstaged changes", "error", err)
	} else {
		allFiles = append(allFiles, parseGitDiff(string(output))...)
	}

	// 3. Untracked files
	cmd = exec.Command("git", "ls-files", "--others", "--exclude-standard")
	cmd.Dir = rootPath
	output, err = cmd.Output()
	if err != nil {
		Warn("failed to get untracked files", "error", err)
	} else {
		for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
			if line != "" {
				allFiles = append(allFiles, ChangedFile{Status: "A", Path: line})
			}
		}
	}

	// Convert paths from git-root-relative to workspace-relative
	workspacePrefix := ""
	if gitRoot != "" && rootPath != gitRoot {
		relPath, err := filepath.Rel(gitRoot, rootPath)
		if err == nil && relPath != "." {
			workspacePrefix = relPath + "/"
		}
	}

	var filteredFiles []ChangedFile
	for _, f := range allFiles {
		if workspacePrefix == "" {
			filteredFiles = append(filteredFiles, f)
		} else if strings.HasPrefix(f.Path, workspacePrefix) {
			filteredFiles = append(filteredFiles, ChangedFile{
				Status: f.Status,
				Path:   strings.TrimPrefix(f.Path, workspacePrefix),
			})
		}
	}

	// Deduplicate
	seen := make(map[string]bool)
	var uniqueFiles []ChangedFile
	for _, f := range filteredFiles {
		if !seen[f.Path] {
			seen[f.Path] = true
			uniqueFiles = append(uniqueFiles, f)
		}
	}

	return uniqueFiles, nil
}

func parseGitDiff(output string) []ChangedFile {
	var files []ChangedFile
	lines := strings.Split(strings.TrimSpace(output), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 2 {
			files = append(files, ChangedFile{
				Status: parts[0],
				Path:   parts[len(parts)-1],
			})
		}
	}

	return files
}

// GetAffectedDirs returns all directories affected by changes
func GetAffectedDirs(files []ChangedFile) map[string]bool {
	dirs := make(map[string]bool)

	for _, f := range files {
		dir := filepath.Dir(f.Path)
		// Add all parent directories
		for dir != "." && dir != "/" {
			dirs[dir] = true
			dir = filepath.Dir(dir)
		}
	}

	return dirs
}
