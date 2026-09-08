package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

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
	Revision  string   `yaml:"revision,omitempty"`
	Languages []string `yaml:"languages,omitempty"` // e.g. ["go", "node", "python"]
	Targets   []string `yaml:"targets,omitempty"`   // e.g. ["docker", "cloudrun", "lambda"]
}

// Config is the main configuration (bear.config.yml)
type Config struct {
	Name       string              `yaml:"name"`
	IgnoreDirs []string            `yaml:"ignore_dirs,omitempty"`
	Use        UseConfig           `yaml:"use,omitempty"` // Import predefined presets
	Languages  map[string]Language `yaml:"languages"`
	Targets    map[string]Target   `yaml:"targets,omitempty"`
}

// Load loads a bear.config.yml file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := DecodeStrict(data, &cfg); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if strings.TrimSpace(cfg.Name) == "" {
		return nil, fmt.Errorf("%s: project name must not be blank", path)
	}
	if err := ValidateRevision(cfg.Use.Revision); err != nil {
		return nil, fmt.Errorf("%s: use.revision: %w", path, err)
	}
	for _, name := range append(append([]string{}, cfg.Use.Languages...), cfg.Use.Targets...) {
		if err := ValidatePresetName(name); err != nil {
			return nil, fmt.Errorf("%s: use: %w", path, err)
		}
	}
	for _, dir := range cfg.IgnoreDirs {
		if err := validateRelativePath(dir); err != nil {
			return nil, fmt.Errorf("%s: ignore_dirs: %w", path, err)
		}
	}

	// Populate Name from map keys
	for name, lang := range cfg.Languages {
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("%s: language name must not be blank", path)
		}
		if lang.Steps == nil {
			return nil, fmt.Errorf("%s: language %q requires steps (use steps: [] for no validation)", path, name)
		}
		if err := ValidateLanguage(lang); err != nil {
			return nil, fmt.Errorf("%s: language %q: %w", path, name, err)
		}
		lang.Name = name
		cfg.Languages[name] = lang
	}
	for name, target := range cfg.Targets {
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("%s: target name must not be blank", path)
		}
		if err := ValidateSteps(target.Steps); err != nil {
			return nil, fmt.Errorf("%s: target %q: %w", path, name, err)
		}
		target.Name = name
		cfg.Targets[name] = target
	}

	return &cfg, nil
}

// DecodeStrict accepts exactly one YAML document and rejects unknown fields.
func DecodeStrict(data []byte, value interface{}) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return err
		}
		return fmt.Errorf("expected exactly one YAML document")
	}
	return nil
}

func ValidatePresetName(name string) error {
	if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`).MatchString(name) {
		return fmt.Errorf("invalid preset name %q: use letters, digits, hyphens, or underscores", name)
	}
	return nil
}

func ValidateRevision(revision string) error {
	if revision != "" && !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(revision) {
		return fmt.Errorf("expected an immutable 40-character lowercase commit SHA, got %q", revision)
	}
	return nil
}

func ValidateSteps(steps []Step) error {
	for i, step := range steps {
		if strings.TrimSpace(step.Name) == "" || strings.TrimSpace(step.Run) == "" {
			return fmt.Errorf("step %d requires nonblank name and run", i+1)
		}
	}
	return nil
}

func ValidateLanguage(language Language) error {
	for _, file := range language.Detection.Files {
		if err := validateRelativePath(file); err != nil {
			return fmt.Errorf("detection.files: %w", err)
		}
	}
	if pattern := language.Detection.Pattern; pattern != "" {
		if err := validateRelativePath(pattern); err != nil {
			return fmt.Errorf("detection.pattern: %w", err)
		}
		if _, err := filepath.Match(pattern, ""); err != nil {
			return fmt.Errorf("detection.pattern: %w", err)
		}
	}
	return ValidateSteps(language.Steps)
}

func validateRelativePath(path string) error {
	if strings.TrimSpace(path) == "" || filepath.IsAbs(path) || strings.Contains(path, `\`) {
		return fmt.Errorf("expected a relative path, got %q", path)
	}
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == ".." {
			return fmt.Errorf("path must not contain '..': %q", path)
		}
	}
	return nil
}
