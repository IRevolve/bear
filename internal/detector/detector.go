package detector

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// ChangedFile repräsentiert eine geänderte Datei
type ChangedFile struct {
	Path   string
	Status string // A=Added, M=Modified, D=Deleted, R=Renamed
}

// GetChangedFiles gibt alle geänderten Dateien zurück (git diff)
func GetChangedFiles(rootPath string, baseBranch string) ([]ChangedFile, error) {
	if baseBranch == "" {
		baseBranch = "main"
	}

	// Hole Git-Root-Verzeichnis
	gitRoot := getGitRoot(rootPath)

	var allFiles []ChangedFile

	// 1. Hole Änderungen zwischen base branch und HEAD (ignoriere Whitespace)
	cmd := exec.Command("git", "diff", "--name-status", "--ignore-space-change", "--ignore-blank-lines", baseBranch+"...HEAD")
	cmd.Dir = rootPath
	output, _ := cmd.Output()
	allFiles = append(allFiles, parseGitDiff(string(output))...)

	// 2. Hole uncommitted staged changes (ignoriere Whitespace)
	cmd = exec.Command("git", "diff", "--name-status", "--cached", "--ignore-space-change", "--ignore-blank-lines")
	cmd.Dir = rootPath
	output, _ = cmd.Output()
	allFiles = append(allFiles, parseGitDiff(string(output))...)

	// 3. Hole uncommitted unstaged changes (ignoriere Whitespace)
	cmd = exec.Command("git", "diff", "--name-status", "--ignore-space-change", "--ignore-blank-lines")
	cmd.Dir = rootPath
	output, _ = cmd.Output()
	allFiles = append(allFiles, parseGitDiff(string(output))...)

	// 4. Hole untracked files
	cmd = exec.Command("git", "ls-files", "--others", "--exclude-standard")
	cmd.Dir = rootPath
	output, _ = cmd.Output()
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line != "" {
			allFiles = append(allFiles, ChangedFile{Status: "A", Path: line})
		}
	}

	// Konvertiere Pfade von git-root-relativ zu workspace-relativ
	workspacePrefix := ""
	if gitRoot != "" && rootPath != gitRoot {
		// Berechne den relativen Pfad vom Git-Root zum Workspace
		relPath, err := filepath.Rel(gitRoot, rootPath)
		if err == nil && relPath != "." {
			workspacePrefix = relPath + "/"
		}
	}

	// Filtere Dateien die im Workspace sind und konvertiere Pfade
	var filteredFiles []ChangedFile
	for _, f := range allFiles {
		if workspacePrefix == "" {
			filteredFiles = append(filteredFiles, f)
		} else if strings.HasPrefix(f.Path, workspacePrefix) {
			// Entferne Workspace-Prefix für lokale Pfade
			filteredFiles = append(filteredFiles, ChangedFile{
				Status: f.Status,
				Path:   strings.TrimPrefix(f.Path, workspacePrefix),
			})
		}
	}

	// Dedupliziere
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

// getGitRoot gibt das Wurzelverzeichnis des Git-Repositories zurück
func getGitRoot(path string) string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = path
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// GetChangedFilesFromCommit gibt geänderte Dateien seit einem Commit zurück
func GetChangedFilesFromCommit(rootPath string, commitHash string) ([]ChangedFile, error) {
	cmd := exec.Command("git", "diff", "--name-status", commitHash+"...HEAD")
	cmd.Dir = rootPath

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	return parseGitDiff(string(output)), nil
}

// GetChangedFilesBetweenCommits gibt geänderte Dateien zwischen zwei Commits zurück
func GetChangedFilesBetweenCommits(rootPath string, fromCommit, toCommit string) ([]ChangedFile, error) {
	cmd := exec.Command("git", "diff", "--name-status", "--ignore-space-change", "--ignore-blank-lines", fromCommit+".."+toCommit)
	cmd.Dir = rootPath

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	return parseGitDiff(string(output)), nil
}

// GetCurrentCommit gibt den aktuellen HEAD Commit zurück
func GetCurrentCommit(rootPath string) string {
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = rootPath

	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(output))
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

// GetAffectedDirs gibt alle Verzeichnisse zurück, die von Änderungen betroffen sind
func GetAffectedDirs(files []ChangedFile) map[string]bool {
	dirs := make(map[string]bool)

	for _, f := range files {
		dir := filepath.Dir(f.Path)
		// Füge alle Parent-Verzeichnisse hinzu
		for dir != "." && dir != "/" {
			dirs[dir] = true
			dir = filepath.Dir(dir)
		}
	}

	return dirs
}
