package config

import (
	"os"

	toml "github.com/pelletier/go-toml/v2"
)

// Library defines a shared library (bear.lib.toml)
type Library struct {
	Name    string   `toml:"name"`
	Depends []string `toml:"depends,omitempty"` // Dependencies to other artifacts/libraries
}

// LoadLibrary loads a bear.lib.toml file
func LoadLibrary(path string) (*Library, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var lib Library
	if err := toml.Unmarshal(data, &lib); err != nil {
		return nil, err
	}

	return &lib, nil
}

// ToArtifact converts a Library to an Artifact for unified handling
func (l *Library) ToArtifact() *Artifact {
	return &Artifact{
		Name:    l.Name,
		Depends: l.Depends,
		IsLib:   true,
	}
}
