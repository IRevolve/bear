package presets

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/IRevolve/Bear/internal/config"
	"gopkg.in/yaml.v3"
)

const (
	// DefaultPresetsRepo ist das Standard-Repository für Presets
	DefaultPresetsRepo = "https://raw.githubusercontent.com/IRevolve/bear-presets/main"

	// CacheDir ist der lokale Cache-Ordner
	CacheDir = ".bear/presets"

	// CacheTTL ist die Gültigkeitsdauer des Caches
	CacheTTL = 24 * time.Hour
)

// PresetIndex enthält die Liste aller verfügbaren Presets
type PresetIndex struct {
	Version   int      `yaml:"version"`
	Languages []string `yaml:"languages"`
	Targets   []string `yaml:"targets"`
}

// Manager verwaltet das Laden und Cachen von Presets
type Manager struct {
	repoURL  string
	cacheDir string
}

// NewManager erstellt einen neuen Preset-Manager
func NewManager() *Manager {
	homeDir, _ := os.UserHomeDir()
	return &Manager{
		repoURL:  DefaultPresetsRepo,
		cacheDir: filepath.Join(homeDir, CacheDir),
	}
}

// GetLanguage lädt ein Language-Preset
func (m *Manager) GetLanguage(name string) (config.Language, error) {
	data, err := m.fetchPreset("languages", name)
	if err != nil {
		return config.Language{}, err
	}

	var lang config.Language
	if err := yaml.Unmarshal(data, &lang); err != nil {
		return config.Language{}, fmt.Errorf("failed to parse language preset %s: %w", name, err)
	}

	return lang, nil
}

// GetTarget lädt ein Target-Preset
func (m *Manager) GetTarget(name string) (config.TargetTemplate, error) {
	data, err := m.fetchPreset("targets", name)
	if err != nil {
		return config.TargetTemplate{}, err
	}

	var target config.TargetTemplate
	if err := yaml.Unmarshal(data, &target); err != nil {
		return config.TargetTemplate{}, fmt.Errorf("failed to parse target preset %s: %w", name, err)
	}

	return target, nil
}

// GetIndex lädt den Preset-Index
func (m *Manager) GetIndex() (*PresetIndex, error) {
	data, err := m.fetchFile("index.yml")
	if err != nil {
		return nil, err
	}

	var index PresetIndex
	if err := yaml.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("failed to parse preset index: %w", err)
	}

	return &index, nil
}

// ListLanguages gibt alle verfügbaren Sprachen zurück
func (m *Manager) ListLanguages() ([]string, error) {
	index, err := m.GetIndex()
	if err != nil {
		return nil, err
	}
	return index.Languages, nil
}

// ListTargets gibt alle verfügbaren Targets zurück
func (m *Manager) ListTargets() ([]string, error) {
	index, err := m.GetIndex()
	if err != nil {
		return nil, err
	}
	return index.Targets, nil
}

// Update aktualisiert den lokalen Cache
func (m *Manager) Update() error {
	// Lösche Cache
	if err := os.RemoveAll(m.cacheDir); err != nil {
		return fmt.Errorf("failed to clear cache: %w", err)
	}

	// Lade Index um Cache zu füllen
	index, err := m.GetIndex()
	if err != nil {
		return err
	}

	// Lade alle Languages
	for _, lang := range index.Languages {
		if _, err := m.GetLanguage(lang); err != nil {
			return fmt.Errorf("failed to fetch language %s: %w", lang, err)
		}
	}

	// Lade alle Targets
	for _, target := range index.Targets {
		if _, err := m.GetTarget(target); err != nil {
			return fmt.Errorf("failed to fetch target %s: %w", target, err)
		}
	}

	return nil
}

// fetchPreset lädt ein Preset (mit Cache)
func (m *Manager) fetchPreset(category, name string) ([]byte, error) {
	filename := fmt.Sprintf("%s/%s.yml", category, name)
	return m.fetchFile(filename)
}

// fetchFile lädt eine Datei (mit Cache)
func (m *Manager) fetchFile(filename string) ([]byte, error) {
	cachePath := filepath.Join(m.cacheDir, filename)

	// Prüfe Cache
	if data, err := m.readCache(cachePath); err == nil {
		return data, nil
	}

	// Lade von GitHub
	url := fmt.Sprintf("%s/%s", m.repoURL, filename)
	data, err := m.download(url)
	if err != nil {
		return nil, err
	}

	// Speichere im Cache
	if err := m.writeCache(cachePath, data); err != nil {
		// Cache-Fehler sind nicht kritisch
		fmt.Fprintf(os.Stderr, "Warning: failed to cache %s: %v\n", filename, err)
	}

	return data, nil
}

// readCache liest aus dem Cache (falls gültig)
func (m *Manager) readCache(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	// Prüfe ob Cache noch gültig
	if time.Since(info.ModTime()) > CacheTTL {
		return nil, fmt.Errorf("cache expired")
	}

	return os.ReadFile(path)
}

// writeCache schreibt in den Cache
func (m *Manager) writeCache(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// download lädt eine URL herunter
func (m *Manager) download(url string) ([]byte, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request for %s: %w", url, err)
	}
	req.Header.Set("User-Agent", "Bear-CI/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch %s: HTTP %d", url, resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}
