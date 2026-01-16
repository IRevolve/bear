package internal

import (
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
			input: "A\tpath/to/file.go",
			expected: []ChangedFile{
				{Status: "A", Path: "path/to/file.go"},
			},
		},
		{
			name:  "single modified file",
			input: "M\tpath/to/file.go",
			expected: []ChangedFile{
				{Status: "M", Path: "path/to/file.go"},
			},
		},
		{
			name:  "single deleted file",
			input: "D\tpath/to/file.go",
			expected: []ChangedFile{
				{Status: "D", Path: "path/to/file.go"},
			},
		},
		{
			name: "multiple files",
			input: `A	new-file.go
M	modified-file.go
D	deleted-file.go`,
			expected: []ChangedFile{
				{Status: "A", Path: "new-file.go"},
				{Status: "M", Path: "modified-file.go"},
				{Status: "D", Path: "deleted-file.go"},
			},
		},
		{
			name:  "renamed file",
			input: "R100\told/path.go\tnew/path.go",
			expected: []ChangedFile{
				{Status: "R100", Path: "new/path.go"},
			},
		},
		{
			name: "with empty lines",
			input: `A	file1.go

M	file2.go
`,
			expected: []ChangedFile{
				{Status: "A", Path: "file1.go"},
				{Status: "M", Path: "file2.go"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseGitDiff(tt.input)

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
			expected: map[string]bool{},
		},
		{
			name: "single file in subdirectory",
			files: []ChangedFile{
				{Path: "src/file.go"},
			},
			expected: map[string]bool{
				"src": true,
			},
		},
		{
			name: "single file in nested directory",
			files: []ChangedFile{
				{Path: "src/pkg/file.go"},
			},
			expected: map[string]bool{
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
