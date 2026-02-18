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

// Language defines a language with detection and validation rules
type Language struct {
	Name      string            `yaml:"-"` // Populated from map key
	Detection Detection         `yaml:"detection"`
	Vars      map[string]string `yaml:"vars,omitempty"` // Default variables for this language
	Steps     []Step            `yaml:"steps"`          // Validation steps (e.g. lint, test, build)
}

// Target defines a reusable deployment template
type Target struct {
	Name  string            `yaml:"-"`              // Populated from map key
	Vars  map[string]string `yaml:"vars,omitempty"` // Default variables for this target
	Steps []Step            `yaml:"steps"`          // Deployment steps (with $VAR placeholders)
}

// UseConfig defines which presets to import
type UseConfig struct {
	Languages []string `yaml:"languages,omitempty"` // e.g. ["go", "node", "python"]
	Targets   []string `yaml:"targets,omitempty"`   // e.g. ["docker", "cloudrun", "lambda"]
}

// Config is the main configuration (bear.config.yml)
type Config struct {
	Name      string              `yaml:"name"`
	Use       UseConfig           `yaml:"use,omitempty"` // Import predefined presets
	Languages map[string]Language `yaml:"languages"`
	Targets   map[string]Target   `yaml:"targets,omitempty"`
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

	// Populate Name from map keys
	for name, lang := range cfg.Languages {
		lang.Name = name
		cfg.Languages[name] = lang
	}
	for name, target := range cfg.Targets {
		target.Name = name
		cfg.Targets[name] = target
	}

	return &cfg, nil
}
