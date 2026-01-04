package loader

import (
	"fmt"

	"github.com/IRevolve/Bear/internal/config"
	"github.com/IRevolve/Bear/internal/presets"
)

// Load lädt eine Config und löst alle Presets auf
func Load(path string) (*config.Config, error) {
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}

	// Löse Language-Presets auf
	if err := resolveLanguages(cfg); err != nil {
		return nil, err
	}

	// Löse Target-Presets auf
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

	// Erstelle Map der bereits definierten Languages
	existing := make(map[string]bool)
	for _, lang := range cfg.Languages {
		existing[lang.Name] = true
	}

	// Füge Presets am Anfang hinzu (können überschrieben werden)
	var presetLangs []config.Language
	for _, name := range cfg.Use.Languages {
		preset, ok := presets.GetLanguage(name)
		if !ok {
			return fmt.Errorf("unknown language preset: %s (available: %v)", name, presets.ListLanguages())
		}
		// Nur hinzufügen wenn nicht bereits definiert
		if !existing[name] {
			presetLangs = append(presetLangs, preset)
		}
	}

	// Presets zuerst, dann custom (custom überschreibt)
	cfg.Languages = append(presetLangs, cfg.Languages...)

	return nil
}

// resolveTargets fügt vordefinierte Targets hinzu
func resolveTargets(cfg *config.Config) error {
	if len(cfg.Use.Targets) == 0 {
		return nil
	}

	// Erstelle Map der bereits definierten Targets
	existing := make(map[string]bool)
	for _, target := range cfg.Targets {
		existing[target.Name] = true
	}

	// Füge Presets am Anfang hinzu
	var presetTargets []config.TargetTemplate
	for _, name := range cfg.Use.Targets {
		preset, ok := presets.GetTarget(name)
		if !ok {
			return fmt.Errorf("unknown target preset: %s (available: %v)", name, presets.ListTargets())
		}
		if !existing[name] {
			presetTargets = append(presetTargets, preset)
		}
	}

	cfg.Targets = append(presetTargets, cfg.Targets...)

	return nil
}
