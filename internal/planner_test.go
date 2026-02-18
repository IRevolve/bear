package internal

import (
	"testing"

	"github.com/irevolve/bear/internal/config"
)

func TestGetValidationSteps(t *testing.T) {
	cfg := &config.Config{
		Languages: map[string]config.Language{
			"go": {
				Name: "go",
				Steps: []config.Step{
					{Name: "Download", Run: "go mod download"},
					{Name: "Vet", Run: "go vet ./..."},
					{Name: "Test", Run: "go test ./..."},
					{Name: "Build", Run: "go build ."},
				},
			},
			"node": {
				Name: "node",
				Steps: []config.Step{
					{Name: "Install", Run: "npm install"},
					{Name: "Lint", Run: "npm run lint"},
					{Name: "Test", Run: "npm test"},
					{Name: "Build", Run: "npm run build"},
				},
			},
		},
	}

	tests := []struct {
		name          string
		language      string
		expectedCount int
		firstStepName string
	}{
		{
			name:          "go language",
			language:      "go",
			expectedCount: 4,
			firstStepName: "Download",
		},
		{
			name:          "node language",
			language:      "node",
			expectedCount: 4,
			firstStepName: "Install",
		},
		{
			name:          "unknown language",
			language:      "unknown",
			expectedCount: 0,
			firstStepName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			steps := getValidationSteps(cfg, tt.language)

			if len(steps) != tt.expectedCount {
				t.Errorf("expected %d steps, got %d", tt.expectedCount, len(steps))
			}

			if tt.expectedCount > 0 && steps[0].Name != tt.firstStepName {
				t.Errorf("expected first step '%s', got '%s'", tt.firstStepName, steps[0].Name)
			}
		})
	}
}

func TestFilterArtifacts(t *testing.T) {
	artifacts := []DiscoveredArtifact{
		{Path: "/path/to/api", Artifact: &config.Artifact{Name: "user-api"}},
		{Path: "/path/to/web", Artifact: &config.Artifact{Name: "dashboard"}},
		{Path: "/path/to/lib", Artifact: &config.Artifact{Name: "shared-lib"}},
	}

	tests := []struct {
		name          string
		names         []string
		expectedCount int
		expectedNames []string
	}{
		{
			name:          "empty names returns all",
			names:         []string{},
			expectedCount: 3,
			expectedNames: []string{"user-api", "dashboard", "shared-lib"},
		},
		{
			name:          "single name",
			names:         []string{"user-api"},
			expectedCount: 1,
			expectedNames: []string{"user-api"},
		},
		{
			name:          "multiple names",
			names:         []string{"user-api", "dashboard"},
			expectedCount: 2,
			expectedNames: []string{"user-api", "dashboard"},
		},
		{
			name:          "non-existent name",
			names:         []string{"nonexistent"},
			expectedCount: 0,
			expectedNames: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterArtifacts(artifacts, tt.names)

			if len(result) != tt.expectedCount {
				t.Errorf("expected %d artifacts, got %d", tt.expectedCount, len(result))
			}

			for i, name := range tt.expectedNames {
				if i < len(result) && result[i].Artifact.Name != name {
					t.Errorf("expected artifact '%s', got '%s'", name, result[i].Artifact.Name)
				}
			}
		})
	}
}

func TestIsArtifactAffected(t *testing.T) {
	tests := []struct {
		name         string
		artifactPath string
		changedFiles []ChangedFile
		expectHit    bool
		expectCount  int
	}{
		{
			name:         "no changes",
			artifactPath: "services/api",
			changedFiles: []ChangedFile{},
			expectHit:    false,
			expectCount:  0,
		},
		{
			name:         "direct file change",
			artifactPath: "services/api",
			changedFiles: []ChangedFile{
				{Path: "services/api/main.go"},
			},
			expectHit:    true,
			expectCount:  1,
		},
		{
			name:         "nested file change",
			artifactPath: "services/api",
			changedFiles: []ChangedFile{
				{Path: "services/api/handlers/user.go"},
			},
			expectHit:    true,
			expectCount:  1,
		},
		{
			name:         "unrelated change",
			artifactPath: "services/api",
			changedFiles: []ChangedFile{
				{Path: "services/web/main.go"},
			},
			expectHit:    false,
			expectCount:  0,
		},
		{
			name:         "multiple changes mixed",
			artifactPath: "services/api",
			changedFiles: []ChangedFile{
				{Path: "services/api/main.go"},
				{Path: "services/api/handlers/user.go"},
				{Path: "services/web/main.go"},
			},
			expectHit:    true,
			expectCount:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			affected, files := isArtifactAffected(tt.artifactPath, tt.changedFiles)

			if affected != tt.expectHit {
				t.Errorf("expected affected=%v, got %v", tt.expectHit, affected)
			}

			if len(files) != tt.expectCount {
				t.Errorf("expected %d files, got %d", tt.expectCount, len(files))
			}
		})
	}
}
