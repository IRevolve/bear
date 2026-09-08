package config

import (
	"fmt"
	"os"
	"strings"
)

// Artifact defines a single deployable artifact (bear.artifact.yml)
type Artifact struct {
	Name         string            `yaml:"name"`
	Language     string            `yaml:"language,omitempty"`
	Target       string            `yaml:"target"`            // Reference to Target
	Vars         map[string]string `yaml:"vars,omitempty"`    // Variables for the target
	Depends      []string          `yaml:"depends,omitempty"` // Dependencies to other artifacts
	IsLib        bool              `yaml:"-"`                 // Set by scanner for libraries
	Environments []string          `yaml:"environments,omitempty"`
}

// LoadArtifact loads a bear.artifact.yml file
func LoadArtifact(path string) (*Artifact, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var artifact Artifact
	if err := DecodeStrict(data, &artifact); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if strings.TrimSpace(artifact.Name) == "" || strings.TrimSpace(artifact.Target) == "" {
		return nil, fmt.Errorf("%s: artifact requires nonblank name and target", path)
	}
	for _, dep := range artifact.Depends {
		if strings.TrimSpace(dep) == "" {
			return nil, fmt.Errorf("%s: dependency name must not be blank", path)
		}
	}
	for _, environment := range artifact.Environments {
		if err := ValidateEnvironment(environment); err != nil {
			return nil, fmt.Errorf("%s: environments: %w", path, err)
		}
	}

	return &artifact, nil
}

// ValidateEnvironment checks an explicit deployment environment.
func ValidateEnvironment(environment string) error {
	switch environment {
	case "dev", "int", "prd":
		return nil
	default:
		return fmt.Errorf("invalid environment %q: expected dev, int, or prd", environment)
	}
}
