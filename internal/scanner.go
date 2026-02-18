package internal

import (
	"os"
	"path/filepath"

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
			lang := detectLanguage(dir, cfg.Languages)

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
			lang := detectLanguage(dir, cfg.Languages)

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
func detectLanguage(dir string, languages map[string]config.Language) string {
	for name, lang := range languages {
		// Check if one of the detection files exists
		for _, file := range lang.Detection.Files {
			if _, err := os.Stat(filepath.Join(dir, file)); err == nil {
				return name
			}
		}

		// Check pattern
		if lang.Detection.Pattern != "" {
			matches, err := filepath.Glob(filepath.Join(dir, lang.Detection.Pattern))
			if err == nil && len(matches) > 0 {
				return name
			}
		}
	}

	return "unknown"
}
