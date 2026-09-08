package internal

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/irevolve/bear/internal/config"
	"gopkg.in/yaml.v3"
)

const (
	DefaultPresetsRevision = "dd1dc2dc7854e95be9cb8bbbfbf888225ac2088a"
	DefaultPresetsRepo     = "https://raw.githubusercontent.com/irevolve/bear-presets/" + DefaultPresetsRevision
	CacheDir               = ".bear/presets"
)

// Maintained corrections for bear-presets/languages/{python,java}.yml at this SHA.
// Upstream hides validation failures with successful shell fallbacks. Keep
// this revision independent of DefaultPresetsRevision so future imports are not
// silently rewritten. Local language definitions bypass GetLanguage entirely.
const correctedPresetsRevision = "dd1dc2dc7854e95be9cb8bbbfbf888225ac2088a"

// This basic preset checks syntax, not ruff/pylint style rules. Projects needing
// those tools should define local steps. Selection depends on files/module
// availability, never on whether installation, tests, or builds succeeded.
const correctedPythonPreset = `name: python
detection:
  files: [requirements.txt, pyproject.toml, setup.py]
validation:
  setup:
    - name: Install
      run: |
        if [ -f requirements.txt ]; then
          python -m pip install -r requirements.txt
        else
          python -m pip install -e .
        fi
  lint:
    - name: Check Python syntax
      run: python -m compileall .
  test:
    - name: Test
      run: |
        python -c 'import importlib.util, os, sys; args = ["pytest"] if importlib.util.find_spec("pytest") is not None else ["unittest", "discover"]; os.execv(sys.executable, [sys.executable, "-m", *args])'
  build:
    - name: Build package
      run: |
        if [ -f pyproject.toml ] || [ -f setup.py ]; then
          python -m build
        else
          printf '%s\n' 'No Python package metadata; skipping package build'
        fi
`

const correctedJavaPreset = `name: java
detection:
  files: [pom.xml, build.gradle, build.gradle.kts]
validation:
  setup:
    - name: Download
      run: |
        if [ -f pom.xml ]; then
          mvn dependency:go-offline
        else
          gradle dependencies
        fi
  lint:
    - name: Check
      run: |
        if [ -f pom.xml ]; then
          mvn checkstyle:check
        else
          gradle check
        fi
  test:
    - name: Test
      run: |
        if [ -f pom.xml ]; then
          mvn test
        else
          gradle test
        fi
  build:
    - name: Build
      run: |
        if [ -f pom.xml ]; then
          mvn package -DskipTests
        else
          gradle build -x test
        fi
`

type PresetIndex struct {
	Version   int      `yaml:"version"`
	Languages []string `yaml:"languages"`
	Targets   []string `yaml:"targets"`
}

type Manager struct {
	repoURL  string
	cacheDir string
	revision string
}

// NewManager defaults to a pinned upstream commit. Each revision has its own cache.
func NewManager(revisions ...string) *Manager {
	revision := DefaultPresetsRevision
	if len(revisions) > 0 && revisions[0] != "" {
		revision = revisions[0]
	}
	homeDir, _ := os.UserHomeDir()
	return &Manager{
		repoURL:  "https://raw.githubusercontent.com/irevolve/bear-presets/" + revision,
		cacheDir: filepath.Join(homeDir, CacheDir),
		revision: revision,
	}
}

// Remote DTOs deliberately differ from local config: upstream publishes named,
// phased languages and defaults/deploy targets, while local config uses steps/vars.
type remoteLanguage struct {
	Name       string            `yaml:"name"`
	Detection  config.Detection  `yaml:"detection"`
	Vars       map[string]string `yaml:"vars"`
	Steps      []config.Step     `yaml:"steps"`
	Validation struct {
		Setup []config.Step `yaml:"setup"`
		Lint  []config.Step `yaml:"lint"`
		Test  []config.Step `yaml:"test"`
		Build []config.Step `yaml:"build"`
	} `yaml:"validation"`
}

type remoteTarget struct {
	Name     string            `yaml:"name"`
	Vars     map[string]string `yaml:"vars"`
	Steps    []config.Step     `yaml:"steps"`
	Defaults map[string]string `yaml:"defaults"`
	Deploy   []config.Step     `yaml:"deploy"`
}

func presetFields(data []byte, name string) (map[string]bool, error) {
	var node yaml.Node
	if err := config.DecodeStrict(data, &node); err != nil {
		return nil, err
	}
	if len(node.Content) != 1 || node.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("preset must be a YAML mapping")
	}
	fields := make(map[string]bool)
	content := node.Content[0].Content
	for i := 0; i < len(content); i += 2 {
		key, value := content[i], content[i+1]
		if key.Value == "<<" {
			return nil, fmt.Errorf("preset schema must be explicit; YAML merge keys are not supported")
		}
		fields[key.Value] = true
		if key.Value == "name" && (value.Tag != "!!str" || value.Value != name) {
			return nil, fmt.Errorf("preset name %q does not match requested name %q", value.Value, name)
		}
		if value.Tag == "!!null" {
			return nil, fmt.Errorf("preset field %q must not be null", key.Value)
		}
	}
	return fields, nil
}

func parseLanguage(data []byte, name string) (config.Language, error) {
	var dto remoteLanguage
	if err := config.DecodeStrict(data, &dto); err != nil {
		return config.Language{}, err
	}
	fields, err := presetFields(data, name)
	if err != nil {
		return config.Language{}, err
	}
	if fields["validation"] && (fields["steps"] || fields["vars"]) {
		return config.Language{}, fmt.Errorf("ambiguous language schema: validation cannot be combined with steps or vars")
	}
	if !fields["validation"] && !fields["steps"] {
		return config.Language{}, fmt.Errorf("language requires steps or validation")
	}
	steps := dto.Steps
	if fields["validation"] {
		steps = append(steps, dto.Validation.Setup...)
		steps = append(steps, dto.Validation.Lint...)
		steps = append(steps, dto.Validation.Test...)
		steps = append(steps, dto.Validation.Build...)
		if len(steps) == 0 {
			return config.Language{}, fmt.Errorf("upstream validation must contain steps")
		}
	}
	language := config.Language{Name: name, Detection: dto.Detection, Vars: dto.Vars, Steps: steps}
	if err := config.ValidateLanguage(language); err != nil {
		return config.Language{}, err
	}
	return language, nil
}

func parseTarget(data []byte, name string) (config.Target, error) {
	var dto remoteTarget
	if err := config.DecodeStrict(data, &dto); err != nil {
		return config.Target{}, err
	}
	fields, err := presetFields(data, name)
	if err != nil {
		return config.Target{}, err
	}
	if (fields["defaults"] || fields["deploy"]) && (fields["vars"] || fields["steps"]) {
		return config.Target{}, fmt.Errorf("ambiguous target schema: defaults/deploy cannot be combined with vars/steps")
	}
	target := config.Target{Name: name, Vars: dto.Vars, Steps: dto.Steps}
	if fields["deploy"] {
		target.Vars, target.Steps = dto.Defaults, dto.Deploy
	}
	if len(target.Steps) == 0 {
		return config.Target{}, fmt.Errorf("target requires nonempty deployment steps")
	}
	if err := config.ValidateSteps(target.Steps); err != nil {
		return config.Target{}, err
	}
	return target, nil
}

func parseIndex(data []byte) (*PresetIndex, error) {
	var index PresetIndex
	if err := config.DecodeStrict(data, &index); err != nil {
		return nil, err
	}
	if index.Version != 1 {
		return nil, fmt.Errorf("unsupported preset index version %d", index.Version)
	}
	for _, names := range [][]string{index.Languages, index.Targets} {
		seen := make(map[string]bool)
		for _, name := range names {
			if err := config.ValidatePresetName(name); err != nil {
				return nil, err
			}
			if seen[name] {
				return nil, fmt.Errorf("duplicate preset %q", name)
			}
			seen[name] = true
		}
	}
	return &index, nil
}

func (m *Manager) GetLanguage(name string) (config.Language, error) {
	if err := config.ValidatePresetName(name); err != nil {
		return config.Language{}, err
	}
	if m.revision == correctedPresetsRevision {
		switch name {
		case "python":
			return parseLanguage([]byte(correctedPythonPreset), name)
		case "java":
			return parseLanguage([]byte(correctedJavaPreset), name)
		}
	}
	data, err := m.fetchFile("languages/" + name + ".yml")
	if err != nil {
		return config.Language{}, err
	}
	return parseLanguage(data, name)
}

func (m *Manager) GetTarget(name string) (config.Target, error) {
	if err := config.ValidatePresetName(name); err != nil {
		return config.Target{}, err
	}
	data, err := m.fetchFile("targets/" + name + ".yml")
	if err != nil {
		return config.Target{}, err
	}
	return parseTarget(data, name)
}

func (m *Manager) GetIndex() (*PresetIndex, error) {
	data, err := m.fetchFile("index.yml")
	if err != nil {
		return nil, err
	}
	return parseIndex(data)
}

func validatePresetFile(filename string, data []byte) error {
	if err := validatePresetPath(filename); err != nil {
		return err
	}
	if filename == "index.yml" {
		_, err := parseIndex(data)
		return err
	}
	parts := strings.Split(filename, "/")
	name := strings.TrimSuffix(parts[1], ".yml")
	switch parts[0] {
	case "languages":
		_, err := parseLanguage(data, name)
		return err
	case "targets":
		_, err := parseTarget(data, name)
		return err
	default:
		return fmt.Errorf("invalid preset category %q", parts[0])
	}
}

func validatePresetPath(filename string) error {
	if filename == "index.yml" {
		return nil
	}
	parts := strings.Split(filename, "/")
	if len(parts) != 2 || (parts[0] != "languages" && parts[0] != "targets") || !strings.HasSuffix(parts[1], ".yml") {
		return fmt.Errorf("invalid preset path %q", filename)
	}
	return config.ValidatePresetName(strings.TrimSuffix(parts[1], ".yml"))
}

// Update fetches and validates the complete revision before publishing any files.
// A failed download or decode never removes a previously usable offline cache.
func (m *Manager) Update() error {
	if err := config.ValidateRevision(m.revision); err != nil {
		return err
	}
	data, err := m.download(m.repoURL + "/index.yml")
	if err != nil {
		return err
	}
	index, err := parseIndex(data)
	if err != nil {
		return fmt.Errorf("index.yml: %w", err)
	}
	files := map[string][]byte{"index.yml": data}
	for category, names := range map[string][]string{"languages": index.Languages, "targets": index.Targets} {
		for _, name := range names {
			filename := category + "/" + name + ".yml"
			data, err := m.download(m.repoURL + "/" + filename)
			if err != nil {
				return err
			}
			if err := validatePresetFile(filename, data); err != nil {
				return fmt.Errorf("%s: %w", filename, err)
			}
			files[filename] = data
		}
	}
	for filename, data := range files {
		path := filepath.Join(m.cacheDir, m.revision, filename)
		// Valid files at an immutable revision never need replacement. Keep them
		// intact even if publishing a missing or corrupt file subsequently fails.
		if cached, err := os.ReadFile(path); err == nil && validatePresetFile(filename, cached) == nil {
			continue
		}
		if err := m.writeCache(path, data); err != nil {
			return fmt.Errorf("cache %s: %w", filename, err)
		}
	}
	return nil
}

func (m *Manager) fetchFile(filename string) ([]byte, error) {
	if err := validatePresetPath(filename); err != nil {
		return nil, err
	}
	if err := config.ValidateRevision(m.revision); err != nil {
		return nil, err
	}
	cachePath := filepath.Join(m.cacheDir, m.revision, filename)
	// Immutable revisions do not expire, but cached bytes are always validated.
	if data, err := os.ReadFile(cachePath); err == nil && validatePresetFile(filename, data) == nil {
		return data, nil
	}
	data, err := m.download(m.repoURL + "/" + filename)
	if err != nil {
		return nil, err
	}
	if err := validatePresetFile(filename, data); err != nil {
		return nil, fmt.Errorf("%s at revision %s: %w", filename, m.revision, err)
	}
	if err := m.writeCache(cachePath, data); err != nil {
		return nil, fmt.Errorf("cache %s: %w", filename, err)
	}
	return data, nil
}

func (m *Manager) writeCache(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".preset-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

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
