package internal

import (
	"fmt"

	"github.com/irevolve/bear/internal/config"
)

// Load loads a config and resolves all presets
func Load(path string) (*config.Config, error) {
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}

	// Resolve language presets
	if err := resolveLanguages(cfg); err != nil {
		return nil, err
	}

	// Resolve target presets
	if err := resolveTargets(cfg); err != nil {
		return nil, err
	}
	for name, target := range cfg.Targets {
		if len(target.Steps) == 0 {
			return nil, fmt.Errorf("target %q has no deployment steps", name)
		}
	}

	return cfg, nil
}

// resolveLanguages adds language presets from remote
func resolveLanguages(cfg *config.Config) error {
	if len(cfg.Use.Languages) == 0 {
		return nil
	}

	manager := NewManager(cfg.Use.Revision)

	// Initialize map if nil
	if cfg.Languages == nil {
		cfg.Languages = make(map[string]config.Language)
	}

	for _, name := range cfg.Use.Languages {
		// Only add if not already defined (local overrides preset)
		if _, exists := cfg.Languages[name]; !exists {
			preset, err := manager.GetLanguage(name)
			if err != nil {
				return fmt.Errorf("language preset %q: %w", name, err)
			}
			preset.Name = name
			cfg.Languages[name] = preset
		}
	}

	return nil
}

// resolveTargets adds target presets from remote
func resolveTargets(cfg *config.Config) error {
	if len(cfg.Use.Targets) == 0 {
		return nil
	}

	manager := NewManager(cfg.Use.Revision)

	// Initialize map if nil
	if cfg.Targets == nil {
		cfg.Targets = make(map[string]config.Target)
	}

	for _, name := range cfg.Use.Targets {
		// Only add if not already defined (local overrides preset)
		if _, exists := cfg.Targets[name]; !exists {
			preset, err := manager.GetTarget(name)
			if err != nil {
				return fmt.Errorf("target preset %q: %w", name, err)
			}
			preset.Name = name
			cfg.Targets[name] = preset
		}
	}

	return nil
}
