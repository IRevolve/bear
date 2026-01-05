package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Artifact defines a single deployable artifact (bear.artifact.yml)
type Artifact struct {
	Name      string            `yaml:"name"`
	Target    string            `yaml:"target"`               // Reference to TargetTemplate
	Params    map[string]string `yaml:"params,omitempty"`     // Parameters for the target
	DependsOn []string          `yaml:"depends_on,omitempty"` // Dependencies to other artifacts
	IsLib     bool              `yaml:"-"`                    // Set by scanner for libraries
}

// LoadArtifact loads a bear.artifact.yml file
func LoadArtifact(path string) (*Artifact, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var artifact Artifact
	if err := yaml.Unmarshal(data, &artifact); err != nil {
		return nil, err
	}

	return &artifact, nil
}
