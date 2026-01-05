package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Artifact defines a single artifact (bear.artifact.yml or bear.lib.yml)
type Artifact struct {
	Name      string            `yaml:"name"`
	Target    string            `yaml:"target,omitempty"`     // Reference to TargetTemplate (empty for libraries)
	Params    map[string]string `yaml:"params,omitempty"`     // Parameters for the target
	DependsOn []string          `yaml:"depends_on,omitempty"` // Dependencies to other artifacts
	IsLib     bool              `yaml:"-"`                    // Set by scanner, not from YAML
}

// LoadArtifact loads a bear.artifact.yml or bear.lib.yml file
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
