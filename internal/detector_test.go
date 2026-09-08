package internal

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseGitDiff(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []ChangedFile
	}{
		{
			name:     "empty input",
			input:    "",
			expected: nil,
		},
		{
			name:  "single added file",
			input: "A\x00path/to/file.go\x00",
			expected: []ChangedFile{
				{Status: "A", Path: "path/to/file.go"},
			},
		},
		{
			name:  "single modified file",
			input: "M\x00path/to/file.go\x00",
			expected: []ChangedFile{
				{Status: "M", Path: "path/to/file.go"},
			},
		},
		{
			name:  "single deleted file",
			input: "D\x00path/to/file.go\x00",
			expected: []ChangedFile{
				{Status: "D", Path: "path/to/file.go"},
			},
		},
		{
			name:  "multiple files",
			input: "A\x00new-file.go\x00M\x00modified-file.go\x00D\x00deleted-file.go\x00",
			expected: []ChangedFile{
				{Status: "A", Path: "new-file.go"},
				{Status: "M", Path: "modified-file.go"},
				{Status: "D", Path: "deleted-file.go"},
			},
		},
		{
			name:  "renamed file",
			input: "R100\x00old/path.go\x00new/path.go\x00",
			expected: []ChangedFile{
				{Status: "R100", Path: "old/path.go"},
				{Status: "R100", Path: "new/path.go"},
			},
		},
		{
			name:  "unusual filenames",
			input: "A\x00space tab\tunicode-\u00e9\n.go\x00M\x00file2.go\x00",
			expected: []ChangedFile{
				{Status: "A", Path: "space tab\tunicode-\u00e9\n.go"},
				{Status: "M", Path: "file2.go"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseGitDiff(tt.input)
			if err != nil {
				t.Fatal(err)
			}

			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d files, got %d", len(tt.expected), len(result))
			}

			for i, f := range result {
				if f.Status != tt.expected[i].Status {
					t.Errorf("file %d: expected status '%s', got '%s'", i, tt.expected[i].Status, f.Status)
				}
				if f.Path != tt.expected[i].Path {
					t.Errorf("file %d: expected path '%s', got '%s'", i, tt.expected[i].Path, f.Path)
				}
			}
		})
	}
}

func TestGetAffectedDirs(t *testing.T) {
	tests := []struct {
		name     string
		files    []ChangedFile
		expected map[string]bool
	}{
		{
			name:     "empty files",
			files:    []ChangedFile{},
			expected: map[string]bool{},
		},
		{
			name: "single file in root",
			files: []ChangedFile{
				{Path: "file.go"},
			},
			expected: map[string]bool{".": true},
		},
		{
			name: "single file in subdirectory",
			files: []ChangedFile{
				{Path: "src/file.go"},
			},
			expected: map[string]bool{
				".":   true,
				"src": true,
			},
		},
		{
			name: "single file in nested directory",
			files: []ChangedFile{
				{Path: "src/pkg/file.go"},
			},
			expected: map[string]bool{
				".":       true,
				"src/pkg": true,
				"src":     true,
			},
		},
		{
			name: "multiple files in different directories",
			files: []ChangedFile{
				{Path: "services/api/main.go"},
				{Path: "libs/common/utils.go"},
			},
			expected: map[string]bool{
				".":            true,
				"services/api": true,
				"services":     true,
				"libs/common":  true,
				"libs":         true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetAffectedDirs(tt.files)

			if len(result) != len(tt.expected) {
				t.Fatalf("expected %d dirs, got %d: %v", len(tt.expected), len(result), result)
			}

			for dir := range tt.expected {
				if !result[dir] {
					t.Errorf("expected dir '%s' to be present", dir)
				}
			}
		})
	}
}

func detectorGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Bear Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Bear Test", "GIT_COMMITTER_EMAIL=test@example.com", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func detectorRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	detectorGit(t, root, "init")
	detectorWrite(t, root, ".gitignore", "bear.lock.yml\n")
	detectorCommit(t, root)
	return root
}

func detectorWrite(t *testing.T, root, path, content string) {
	t.Helper()
	full := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func detectorCommit(t *testing.T, root string) string {
	t.Helper()
	detectorGit(t, root, "add", ".")
	detectorGit(t, root, "commit", "-m", "test")
	return detectorGit(t, root, "rev-parse", "HEAD")
}

func TestDetectorGitPaths(t *testing.T) {
	for _, nested := range []bool{false, true} {
		t.Run(map[bool]string{false: "root", true: "nested"}[nested], func(t *testing.T) {
			root := detectorRepo(t)
			workspace := root
			prefix := ""
			if nested {
				prefix = "nested workspace/"
				workspace = filepath.Join(root, "nested workspace")
			}
			old := "source/space tab\tunicode-\u00e9.go"
			newPath := "destination/new name\t\u00e9.go"
			detectorWrite(t, root, prefix+old, "original content\n")
			detectorWrite(t, root, prefix+"unstaged.go", "original\n")
			baseline := detectorCommit(t, root)
			if err := os.MkdirAll(filepath.Join(workspace, "destination"), 0755); err != nil {
				t.Fatal(err)
			}
			detectorGit(t, root, "mv", prefix+old, prefix+newPath)
			files, err := GetUncommittedChanges(workspace)
			want := []ChangedFile{{Path: old, Status: "R100"}, {Path: newPath, Status: "R100"}}
			if err != nil || !reflect.DeepEqual(files, want) {
				t.Fatalf("staged rename: %v, %v; want %v", files, err, want)
			}
			current := detectorCommit(t, root)
			files, err = GetChangedFilesBetweenCommits(workspace, baseline, current)
			if err != nil || !reflect.DeepEqual(files, want) {
				t.Fatalf("committed rename: %v, %v; want %v", files, err, want)
			}
			detectorWrite(t, workspace, "unstaged.go", "changed\n")
			detectorWrite(t, workspace, "untracked \t\u00e9\n.go", "new\n")
			files, err = GetUncommittedChanges(workspace)
			want = []ChangedFile{{Path: "unstaged.go", Status: "M"}, {Path: "untracked \t\u00e9\n.go", Status: "A"}}
			if err != nil || !reflect.DeepEqual(files, want) {
				t.Fatalf("working changes: %v, %v; want %v", files, err, want)
			}
			if nested {
				detectorGit(t, root, "mv", prefix+newPath, "outside.go")
				files, err = GetUncommittedChanges(workspace)
				if err != nil || len(files) != 3 || files[0].Path != newPath {
					t.Fatalf("rename out of workspace lost source: %v, %v", files, err)
				}
				detectorCommit(t, root)
				files, err = GetChangedFilesBetweenCommits(workspace, current, "HEAD")
				if err != nil || len(files) != 3 {
					t.Fatalf("committed rename out: %v, %v", files, err)
				}
			}
		})
	}
}

func TestDetectorErrors(t *testing.T) {
	root := t.TempDir()
	if _, err := ResolveCommit(root, "HEAD"); err == nil {
		t.Fatal("missing repo should fail")
	}
	if _, err := GetUncommittedChanges(root); err == nil {
		t.Fatal("missing repo changes should fail")
	}
	root = detectorRepo(t)
	if _, err := GetChangedFilesBetweenCommits(root, "missing-commit", "HEAD"); err == nil {
		t.Fatal("invalid baseline should fail")
	}
	for _, input := range []string{"M", "M\x00", "R100\x00old\x00", "M\x00\x00"} {
		if _, err := parseGitDiff(input); err == nil {
			t.Errorf("malformed input %q should fail", input)
		}
	}
}

func TestDetectorIgnoresBearState(t *testing.T) {
	for _, nested := range []bool{false, true} {
		t.Run(map[bool]string{false: "root", true: "nested"}[nested], func(t *testing.T) {
			root := detectorRepo(t)
			// State must be excluded by detection, not merely by .gitignore.
			detectorWrite(t, root, ".gitignore", "")
			workspace := root
			if nested {
				workspace = filepath.Join(root, "workspace")
			}
			statePaths := []string{"bear.lock.yml", "nested/bear.lock.yml", ".bear/marker", "nested/.bear/marker"}
			sourcePaths := []string{".bear-source/code.go", "nested/bear.lock.yml.example", "nested/not.bear/code.go"}
			for _, path := range append(append([]string(nil), statePaths...), sourcePaths...) {
				detectorWrite(t, workspace, path, "original\n")
			}
			baseline := detectorCommit(t, root)
			var want []ChangedFile
			for _, path := range sourcePaths {
				want = append(want, ChangedFile{Path: path, Status: "M"})
			}
			for _, path := range append(append([]string(nil), statePaths...), sourcePaths...) {
				detectorWrite(t, workspace, path, "changed\n")
			}
			for _, staged := range []bool{false, true} {
				if staged {
					detectorGit(t, root, "add", ".")
				}
				files, err := GetUncommittedChanges(workspace)
				if err != nil || !reflect.DeepEqual(files, want) {
					t.Fatalf("working changes (staged=%v): %v, %v; want %v", staged, files, err, want)
				}
			}
			detectorCommit(t, root)
			files, err := GetChangedFilesBetweenCommits(workspace, baseline, "HEAD")
			if err != nil || !reflect.DeepEqual(files, want) {
				t.Fatalf("historical changes: %v, %v; want %v", files, err, want)
			}
			for _, path := range []string{"untracked/bear.lock.yml", ".bear/new-marker", "untracked/.bear/marker"} {
				detectorWrite(t, workspace, path, "untracked\n")
			}
			files, err = GetUncommittedChanges(workspace)
			if err != nil || len(files) != 0 {
				t.Fatalf("untracked state: %v, %v", files, err)
			}
		})
	}
}

func TestDetectorIgnoresInheritedRepositoryOverrides(t *testing.T) {
	root := detectorRepo(t)
	foreign := detectorRepo(t)
	detectorWrite(t, foreign, "foreign.go", "foreign repository\n")
	detectorCommit(t, foreign)
	detectorWrite(t, foreign, "foreign.go", "foreign working change\n")
	detectorGit(t, foreign, "add", ".")
	workspace := filepath.Join(root, "workspace")
	for _, path := range []string{"committed.go", "staged.go", "unstaged.go"} {
		detectorWrite(t, workspace, path, "original\n")
	}
	baseline := detectorCommit(t, root)
	detectorWrite(t, workspace, "committed.go", "committed change\n")
	current := detectorCommit(t, root)
	detectorWrite(t, workspace, "staged.go", "staged change\n")
	detectorGit(t, root, "add", ".")
	detectorWrite(t, workspace, "unstaged.go", "unstaged change\n")
	detectorWrite(t, workspace, "untracked.go", "untracked change\n")
	overrides := map[string]string{
		"GIT_DIR":                          filepath.Join(foreign, ".git"),
		"GIT_WORK_TREE":                    foreign,
		"GIT_COMMON_DIR":                   filepath.Join(foreign, ".git"),
		"GIT_INDEX_FILE":                   filepath.Join(foreign, ".git", "index"),
		"GIT_OBJECT_DIRECTORY":             filepath.Join(foreign, ".git", "objects"),
		"GIT_ALTERNATE_OBJECT_DIRECTORIES": filepath.Join(foreign, ".git", "objects"),
		"GIT_PREFIX":                       "foreign/",
		"GIT_NAMESPACE":                    "foreign",
	}
	// Check each override independently as well as the complete foreign environment.
	for _, key := range []string{"all", "GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_PREFIX", "GIT_NAMESPACE"} {
		t.Run(key, func(t *testing.T) {
			for name, value := range overrides {
				if key == "all" || key == name {
					t.Setenv(name, value)
				}
			}
			commit, err := ResolveCommit(workspace, "HEAD")
			if err != nil || commit != current {
				t.Fatalf("resolved foreign HEAD: %q, %v; want %q", commit, err, current)
			}
			files, err := GetChangedFilesBetweenCommits(workspace, baseline, "HEAD")
			want := []ChangedFile{{Path: "committed.go", Status: "M"}}
			if err != nil || !reflect.DeepEqual(files, want) {
				t.Fatalf("historical changes: %v, %v; want %v", files, err, want)
			}
			files, err = GetUncommittedChanges(workspace)
			want = []ChangedFile{{Path: "staged.go", Status: "M"}, {Path: "unstaged.go", Status: "M"}, {Path: "untracked.go", Status: "A"}}
			if err != nil || !reflect.DeepEqual(files, want) {
				t.Fatalf("working changes: %v, %v; want %v", files, err, want)
			}
		})
	}
}
