package loader

import (
	"fmt"

	"github.com/IRevolve/Bear/internal/config"
	"github.com/IRevolve/Bear/internal/presets"
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

	return cfg, nil
}

// resolveLanguages fügt vordefinierte Languages hinzu
func resolveLanguages(cfg *config.Config) error {
	if len(cfg.Use.Languages) == 0 {
		return nil
	}

	manager := presets.NewManager()

	// Create map of already defined languages
	existing := make(map[string]bool)
	for _, lang := range cfg.Languages {
		existing[lang.Name] = true
	}

	// Add presets first (can be overridden)
	var presetLangs []config.Language
	for _, name := range cfg.Use.Languages {
		// Try to load from remote first
		preset, err := manager.GetLanguage(name)
		if err != nil {
			// Fallback to embedded presets
			var ok bool
			preset, ok = presets.GetLanguage(name)
			if !ok {
				return fmt.Errorf("unknown language preset: %s", name)
			}
		}
		// Only add if not already defined
		if !existing[name] {
			presetLangs = append(presetLangs, preset)
		}
	}

	// Presets first, then custom (custom overrides)
	cfg.Languages = append(presetLangs, cfg.Languages...)

	return nil
}

// resolveTargets fügt vordefinierte Targets hinzu
func resolveTargets(cfg *config.Config) error {
	if len(cfg.Use.Targets) == 0 {
		return nil
	}

	manager := presets.NewManager()

	// Create map of already defined targets
	existing := make(map[string]bool)
	for _, target := range cfg.Targets {
		existing[target.Name] = true
	}

	// Add presets first
	var presetTargets []config.TargetTemplate
	for _, name := range cfg.Use.Targets {
		// Try to load from remote first
		preset, err := manager.GetTarget(name)
		if err != nil {
			// Fallback to embedded presets
			var ok bool
			preset, ok = presets.GetTarget(name)
			if !ok {
				return fmt.Errorf("unknown target preset: %s", name)
			}
		}
		if !existing[name] {
			presetTargets = append(presetTargets, preset)
		}
	}

	cfg.Targets = append(presetTargets, cfg.Targets...)

	return nil
}
