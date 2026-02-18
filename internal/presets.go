package internal

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/irevolve/bear/internal/config"
	"gopkg.in/yaml.v3"
)

const (
	// DefaultPresetsRepo is the default repository for presets
	DefaultPresetsRepo = "https://raw.githubusercontent.com/irevolve/bear-presets/main"

	// CacheDir is the local cache directory
	CacheDir = ".bear/presets"

	// CacheTTL is the cache validity duration
	CacheTTL = 24 * time.Hour
)

// PresetIndex contains the list of all available presets
type PresetIndex struct {
	Version   int      `yaml:"version"`
	Languages []string `yaml:"languages"`
	Targets   []string `yaml:"targets"`
}

// Manager manages loading and caching of presets
type Manager struct {
	repoURL  string
	cacheDir string
}

// NewManager creates a new preset manager
func NewManager() *Manager {
	homeDir, _ := os.UserHomeDir()
	return &Manager{
		repoURL:  DefaultPresetsRepo,
		cacheDir: filepath.Join(homeDir, CacheDir),
	}
}

// GetLanguage loads a language preset
func (m *Manager) GetLanguage(name string) (config.Language, error) {
	data, err := m.fetchPreset("languages", name)
	if err != nil {
		return config.Language{}, err
	}

	var lang config.Language
	if err := yaml.Unmarshal(data, &lang); err != nil {
		return config.Language{}, fmt.Errorf("failed to parse language preset %s: %w", name, err)
	}
	lang.Name = name

	return lang, nil
}

// GetTarget loads a target preset
func (m *Manager) GetTarget(name string) (config.Target, error) {
	data, err := m.fetchPreset("targets", name)
	if err != nil {
		return config.Target{}, err
	}

	var target config.Target
	if err := yaml.Unmarshal(data, &target); err != nil {
		return config.Target{}, fmt.Errorf("failed to parse target preset %s: %w", name, err)
	}
	target.Name = name

	return target, nil
}

// GetIndex loads the preset index
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

// Update updates the local cache
func (m *Manager) Update() error {
	// Clear cache
	if err := os.RemoveAll(m.cacheDir); err != nil {
		return fmt.Errorf("failed to clear cache: %w", err)
	}

	// Load index to fill cache
	index, err := m.GetIndex()
	if err != nil {
		return err
	}

	// Load all languages
	for _, lang := range index.Languages {
		if _, err := m.GetLanguage(lang); err != nil {
			return fmt.Errorf("failed to fetch language %s: %w", lang, err)
		}
	}

	// Load all targets
	for _, target := range index.Targets {
		if _, err := m.GetTarget(target); err != nil {
			return fmt.Errorf("failed to fetch target %s: %w", target, err)
		}
	}

	return nil
}

// fetchPreset loads a preset (with caching)
func (m *Manager) fetchPreset(category, name string) ([]byte, error) {
	filename := fmt.Sprintf("%s/%s.yml", category, name)
	return m.fetchFile(filename)
}

// fetchFile loads a file (with caching)
func (m *Manager) fetchFile(filename string) ([]byte, error) {
	cachePath := filepath.Join(m.cacheDir, filename)

	// Check cache
	if data, err := m.readCache(cachePath); err == nil {
		return data, nil
	}

	// Load from GitHub
	url := fmt.Sprintf("%s/%s", m.repoURL, filename)
	data, err := m.download(url)
	if err != nil {
		return nil, err
	}

	// Save to cache
	if err := m.writeCache(cachePath, data); err != nil {
		// Cache errors are not critical
		fmt.Fprintf(os.Stderr, "Warning: failed to cache %s: %v\n", filename, err)
	}

	return data, nil
}

// readCache reads from cache (if valid)
func (m *Manager) readCache(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	// Check if cache is still valid
	if time.Since(info.ModTime()) > CacheTTL {
		return nil, fmt.Errorf("cache expired")
	}

	return os.ReadFile(path)
}

// writeCache writes to the cache
func (m *Manager) writeCache(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// download downloads a URL
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
