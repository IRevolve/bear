package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Library defines a shared library (bear.lib.yml)
type Library struct {
	Name      string   `yaml:"name"`
	DependsOn []string `yaml:"depends_on,omitempty"` // Dependencies to other artifacts/libraries
}

// LoadLibrary loads a bear.lib.yml file
func LoadLibrary(path string) (*Library, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var lib Library
	if err := yaml.Unmarshal(data, &lib); err != nil {
		return nil, err
	}

	return &lib, nil
}

// ToArtifact converts a Library to an Artifact for unified handling
func (l *Library) ToArtifact() *Artifact {
	return &Artifact{
		Name:      l.Name,
		DependsOn: l.DependsOn,
		IsLib:     true,
	}
}
