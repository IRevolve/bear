package config

import (
	"fmt"
	"os"
	"strings"
)

// Library defines a shared library (bear.lib.yml)
type Library struct {
	Name     string   `yaml:"name"`
	Language string   `yaml:"language,omitempty"`
	Depends  []string `yaml:"depends,omitempty"` // Dependencies to other artifacts/libraries
}

// LoadLibrary loads a bear.lib.yml file
func LoadLibrary(path string) (*Library, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var lib Library
	if err := DecodeStrict(data, &lib); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if strings.TrimSpace(lib.Name) == "" {
		return nil, fmt.Errorf("%s: library name must not be blank", path)
	}
	for _, dep := range lib.Depends {
		if strings.TrimSpace(dep) == "" {
			return nil, fmt.Errorf("%s: dependency name must not be blank", path)
		}
	}

	return &lib, nil
}

// ToArtifact converts a Library to an Artifact for unified handling
func (l *Library) ToArtifact() *Artifact {
	return &Artifact{
		Name:     l.Name,
		Language: l.Language,
		Depends:  l.Depends,
		IsLib:    true,
	}
}
