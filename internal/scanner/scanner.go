package scanner

import (
	"os"
	"path/filepath"

	"github.com/IRevolve/Bear/internal/config"
)

// DiscoveredArtifact enthält ein Artefakt mit seinem Pfad und erkannter Sprache
type DiscoveredArtifact struct {
	Path     string
	Artifact *config.Artifact
	Language string
}

// ScanArtifacts durchsucht ein Verzeichnis rekursiv nach bear.artifact.yml und bear.lib.yml Dateien
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

		if isArtifact || isLib {
			artifact, err := config.LoadArtifact(path)
			if err != nil {
				return err
			}

			artifact.IsLib = isLib
			dir := filepath.Dir(path)
			lang := detectLanguage(dir, cfg.Languages)

			artifacts = append(artifacts, DiscoveredArtifact{
				Path:     dir,
				Artifact: artifact,
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

// detectLanguage erkennt die Sprache eines Verzeichnisses anhand der Detection-Regeln
func detectLanguage(dir string, languages []config.Language) string {
	for _, lang := range languages {
		// Prüfe ob eine der Detection-Dateien existiert
		for _, file := range lang.Detection.Files {
			if _, err := os.Stat(filepath.Join(dir, file)); err == nil {
				return lang.Name
			}
		}

		// Prüfe Pattern
		if lang.Detection.Pattern != "" {
			matches, err := filepath.Glob(filepath.Join(dir, lang.Detection.Pattern))
			if err == nil && len(matches) > 0 {
				return lang.Name
			}
		}
	}

	return "unknown"
}
