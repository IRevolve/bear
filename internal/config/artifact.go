package config

import (
	"os"

	toml "github.com/pelletier/go-toml/v2"
)

// Artifact defines a single deployable artifact (bear.artifact.toml)
type Artifact struct {
	Name    string            `toml:"name"`
	Target  string            `toml:"target"`            // Reference to Target
	Vars    map[string]string `toml:"vars,omitempty"`    // Variables for the target
	Depends []string          `toml:"depends,omitempty"` // Dependencies to other artifacts
	IsLib   bool              `toml:"-"`                 // Set by scanner for libraries
}

// LoadArtifact loads a bear.artifact.toml file
func LoadArtifact(path string) (*Artifact, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var artifact Artifact
	if err := toml.Unmarshal(data, &artifact); err != nil {
		return nil, err
	}

	return &artifact, nil
}
