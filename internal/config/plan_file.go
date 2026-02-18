package config

import (
	"os"
	"path/filepath"
	"time"

	toml "github.com/pelletier/go-toml/v2"
)

// PlanArtifact represents a single artifact in the plan file
type PlanArtifact struct {
	Name         string            `toml:"name"`
	Path         string            `toml:"path"`
	Language     string            `toml:"language"`
	Target       string            `toml:"target,omitempty"`
	Action       string            `toml:"action"` // "deploy" or "skip"
	Reason       string            `toml:"reason"`
	ChangedFiles []string          `toml:"changed_files,omitempty"`
	Vars         map[string]string `toml:"vars,omitempty"`
	Steps        []Step            `toml:"steps,omitempty"` // Deploy steps only (validation already ran)
	Pinned       bool              `toml:"pinned,omitempty"`
	PinCommit    string            `toml:"pin_commit,omitempty"`
	IsLib        bool              `toml:"is_lib,omitempty"`
}

// PlanSkipped represents a skipped artifact
type PlanSkipped struct {
	Name   string `toml:"name"`
	Reason string `toml:"reason"`
}

// PlanFile is the serializable plan written to .bear/plan.toml
type PlanFile struct {
	CreatedAt  string         `toml:"created_at"`
	Commit     string         `toml:"commit"`
	Artifacts  []PlanArtifact `toml:"artifacts"`
	Skipped    []PlanSkipped  `toml:"skipped,omitempty"`
	Validated  int            `toml:"validated"`
	ToDeploy   int            `toml:"to_deploy"`
	TotalSkips int            `toml:"total_skipped"`
}

// NewPlanFile creates a new PlanFile with current timestamp
func NewPlanFile(commit string) *PlanFile {
	return &PlanFile{
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Commit:    commit,
	}
}

// BearDir returns the path to the .bear directory
func BearDir(rootPath string) string {
	return filepath.Join(rootPath, ".bear")
}

// PlanFilePath returns the path to .bear/plan.toml
func PlanFilePath(rootPath string) string {
	return filepath.Join(BearDir(rootPath), "plan.toml")
}

// WritePlan writes the plan file to .bear/plan.toml
func WritePlan(rootPath string, plan *PlanFile) error {
	dir := BearDir(rootPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := toml.Marshal(plan)
	if err != nil {
		return err
	}

	return os.WriteFile(PlanFilePath(rootPath), data, 0644)
}

// ReadPlan reads the plan file from .bear/plan.toml
func ReadPlan(rootPath string) (*PlanFile, error) {
	data, err := os.ReadFile(PlanFilePath(rootPath))
	if err != nil {
		return nil, err
	}

	var plan PlanFile
	if err := toml.Unmarshal(data, &plan); err != nil {
		return nil, err
	}

	return &plan, nil
}

// RemovePlan deletes the plan file
func RemovePlan(rootPath string) error {
	return os.Remove(PlanFilePath(rootPath))
}

// PlanExists checks if a plan file exists
func PlanExists(rootPath string) bool {
	_, err := os.Stat(PlanFilePath(rootPath))
	return err == nil
}
