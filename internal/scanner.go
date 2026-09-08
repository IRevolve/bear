package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/irevolve/bear/internal/config"
)

// DiscoveredArtifact contains an artifact with its path and detected language
type DiscoveredArtifact struct {
	Path     string
	Artifact *config.Artifact
	Language string
}

// ScanArtifacts recursively scans a directory for bear.artifact.yml and bear.lib.yml files
func ScanArtifacts(rootPath string, cfg *config.Config) ([]DiscoveredArtifact, error) {
	var artifacts []DiscoveredArtifact

	err := filepath.WalkDir(rootPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if path != rootPath {
				rel, err := filepath.Rel(rootPath, path)
				if err != nil {
					return err
				}
				for _, ignored := range append([]string{".git", ".bear", "node_modules", "vendor", "generated", "dist", "build"}, cfg.IgnoreDirs...) {
					if d.Name() == ignored || filepath.Clean(rel) == filepath.Clean(ignored) {
						return filepath.SkipDir
					}
				}
			}
			return nil
		}

		isLib := d.Name() == "bear.lib.yml"
		isArtifact := d.Name() == "bear.artifact.yml"

		if isArtifact {
			artifact, err := config.LoadArtifact(path)
			if err != nil {
				return err
			}

			dir := filepath.Dir(path)
			lang, err := detectLanguage(dir, cfg.Languages, artifact.Language)
			if err != nil {
				return err
			}

			artifacts = append(artifacts, DiscoveredArtifact{
				Path:     dir,
				Artifact: artifact,
				Language: lang,
			})
		} else if isLib {
			lib, err := config.LoadLibrary(path)
			if err != nil {
				return err
			}

			dir := filepath.Dir(path)
			lang, err := detectLanguage(dir, cfg.Languages, lib.Language)
			if err != nil {
				return err
			}

			artifacts = append(artifacts, DiscoveredArtifact{
				Path:     dir,
				Artifact: lib.ToArtifact(),
				Language: lang,
			})
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return artifacts, nil
}

// detectLanguage detects the language of a directory based on detection rules
func detectLanguage(dir string, languages map[string]config.Language, override string) (string, error) {
	if override != "" {
		if _, ok := languages[override]; !ok {
			return "", fmt.Errorf("%s: explicit language %q is not configured", dir, override)
		}
		return override, nil
	}
	var candidates []string
	for name, lang := range languages {
		matched := false
		// Check if one of the detection files exists
		for _, file := range lang.Detection.Files {
			if info, err := os.Stat(filepath.Join(dir, file)); err == nil && !info.IsDir() {
				matched = true
				break
			} else if err != nil && !os.IsNotExist(err) {
				return "", fmt.Errorf("%s: detecting language %q: %w", dir, name, err)
			}
		}

		// Check pattern
		if lang.Detection.Pattern != "" {
			matches, err := filepath.Glob(filepath.Join(dir, lang.Detection.Pattern))
			if err != nil {
				return "", fmt.Errorf("%s: detecting language %q: %w", dir, name, err)
			}
			if len(matches) > 0 {
				matched = true
			}
		}
		if matched {
			candidates = append(candidates, name)
		}
	}
	sort.Strings(candidates)
	if len(candidates) > 1 {
		return "", fmt.Errorf("%s: ambiguous language detection (%s); set language explicitly in bear.artifact.yml or bear.lib.yml", dir, strings.Join(candidates, ", "))
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	return "unknown", nil
}
