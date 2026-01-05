package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Detection defines how a language is detected in a directory
type Detection struct {
	Files   []string `yaml:"files,omitempty"`   // e.g. ["go.mod", "go.sum"]
	Pattern string   `yaml:"pattern,omitempty"` // Glob pattern, e.g. "*.go"
}

// Step defines a CI step
type Step struct {
	Name string `yaml:"name"`
	Run  string `yaml:"run"`
}

// Validation defines how a language is validated
type Validation struct {
	Setup []Step `yaml:"setup,omitempty"` // e.g. install dependencies
	Lint  []Step `yaml:"lint,omitempty"`  // Linting steps
	Test  []Step `yaml:"test,omitempty"`  // Test steps
	Build []Step `yaml:"build,omitempty"` // Build steps
}

// Language defines a language with detection and validation rules
type Language struct {
	Name       string     `yaml:"name"`
	Detection  Detection  `yaml:"detection"`
	Validation Validation `yaml:"validation"`
}

// TargetTemplate defines a reusable deployment template
type TargetTemplate struct {
	Name     string            `yaml:"name"`
	Defaults map[string]string `yaml:"defaults,omitempty"` // Default parameter values
	Deploy   []Step            `yaml:"deploy,omitempty"`   // Deployment steps (with $PARAM placeholders)
}

// UseConfig defines which presets to import
type UseConfig struct {
	Languages []string `yaml:"languages,omitempty"` // e.g. ["go", "node", "python"]
	Targets   []string `yaml:"targets,omitempty"`   // e.g. ["docker", "cloudrun", "lambda"]
}

// Config is the main configuration (bear.config.yml)
type Config struct {
	Name      string           `yaml:"name"`
	Use       UseConfig        `yaml:"use,omitempty"` // Import predefined presets
	Languages []Language       `yaml:"languages"`
	Targets   []TargetTemplate `yaml:"targets,omitempty"`
}

// Load loads a bear.config.yml file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
