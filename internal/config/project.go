package config

import (
	"os"

	toml "github.com/pelletier/go-toml/v2"
)

// Detection defines how a language is detected in a directory
type Detection struct {
	Files   []string `toml:"files,omitempty" yaml:"files,omitempty"`   // e.g. ["go.mod", "go.sum"]
	Pattern string   `toml:"pattern,omitempty" yaml:"pattern,omitempty"` // Glob pattern, e.g. "*.go"
}

// Step defines a CI step
type Step struct {
	Name string `toml:"name" yaml:"name"`
	Run  string `toml:"run" yaml:"run"`
}

// Language defines a language with detection and validation rules
type Language struct {
	Name      string            `toml:"-" yaml:"name"`              // Map key (TOML) or field (YAML presets)
	Detection Detection         `toml:"detection" yaml:"detection"`
	Vars      map[string]string `toml:"vars,omitempty" yaml:"vars,omitempty"` // Default variables for this language
	Steps     []Step            `toml:"steps" yaml:"steps"`          // Validation steps (e.g. lint, test, build)
}

// Target defines a reusable deployment template
type Target struct {
	Name  string            `toml:"-" yaml:"name"`               // Map key (TOML) or field (YAML presets)
	Vars  map[string]string `toml:"vars,omitempty" yaml:"vars,omitempty"` // Default variables for this target
	Steps []Step            `toml:"steps" yaml:"steps"`          // Deployment steps (with $VAR placeholders)
}

// UseConfig defines which presets to import
type UseConfig struct {
	Languages []string `toml:"languages,omitempty"` // e.g. ["go", "node", "python"]
	Targets   []string `toml:"targets,omitempty"`   // e.g. ["docker", "cloudrun", "lambda"]
}

// Config is the main configuration (bear.config.toml)
type Config struct {
	Name      string                `toml:"name"`
	Use       UseConfig             `toml:"use,omitempty"` // Import predefined presets
	Languages map[string]Language   `toml:"languages"`
	Targets   map[string]Target     `toml:"targets,omitempty"`
}

// Load loads a bear.config.toml file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
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
