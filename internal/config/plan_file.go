package config

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// PlanArtifact represents a single artifact in the plan file
type PlanArtifact struct {
	Name         string            `yaml:"name"`
	Path         string            `yaml:"path"`
	Language     string            `yaml:"language"`
	Target       string            `yaml:"target,omitempty"`
	Action       string            `yaml:"action"` // "deploy" or "skip"
	Reason       string            `yaml:"reason"`
	ChangedFiles []string          `yaml:"changed_files,omitempty"`
	Vars         map[string]string `yaml:"vars,omitempty"`
	Steps        []Step            `yaml:"steps,omitempty"` // Deploy steps only (validation already ran)
	Pinned       bool              `yaml:"pinned,omitempty"`
	PinCommit    string            `yaml:"pin_commit,omitempty"`
	IsLib        bool              `yaml:"is_lib,omitempty"`
}

// PlanSkipped represents a skipped artifact
type PlanSkipped struct {
	Name   string `yaml:"name"`
	Reason string `yaml:"reason"`
}

// PlanFile is the serializable plan written to .bear/plan.yml
type PlanFile struct {
	CreatedAt  string         `yaml:"created_at"`
	Commit     string         `yaml:"commit"`
	Artifacts  []PlanArtifact `yaml:"artifacts"`
	Skipped    []PlanSkipped  `yaml:"skipped,omitempty"`
	Validated  int            `yaml:"validated"`
	ToDeploy   int            `yaml:"to_deploy"`
	TotalSkips int            `yaml:"total_skipped"`
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

// PlanFilePath returns the path to .bear/plan.yml
func PlanFilePath(rootPath string) string {
	return filepath.Join(BearDir(rootPath), "plan.yml")
}

// WritePlan writes the plan file to .bear/plan.yml
func WritePlan(rootPath string, plan *PlanFile) error {
	dir := BearDir(rootPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(plan)
	if err != nil {
		return err
	}

	return os.WriteFile(PlanFilePath(rootPath), data, 0644)
}

// ReadPlan reads the plan file from .bear/plan.yml
func ReadPlan(rootPath string) (*PlanFile, error) {
	data, err := os.ReadFile(PlanFilePath(rootPath))
	if err != nil {
		return nil, err
	}

	var plan PlanFile
	if err := yaml.Unmarshal(data, &plan); err != nil {
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
