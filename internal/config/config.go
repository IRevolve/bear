package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Detection definiert, wie eine Sprache in einem Ordner erkannt wird
type Detection struct {
	Files   []string `yaml:"files,omitempty"`   // z.B. ["go.mod", "go.sum"]
	Pattern string   `yaml:"pattern,omitempty"` // Glob-Pattern, z.B. "*.go"
}

// Step definiert einen CI-Schritt
type Step struct {
	Name string `yaml:"name"`
	Run  string `yaml:"run"`
}

// Validation definiert, wie eine Sprache validiert wird
type Validation struct {
	Setup []Step `yaml:"setup,omitempty"` // z.B. Dependencies installieren
	Lint  []Step `yaml:"lint,omitempty"`  // Linting-Schritte
	Test  []Step `yaml:"test,omitempty"`  // Test-Schritte
	Build []Step `yaml:"build,omitempty"` // Build-Schritte
}

// Language definiert eine Sprache mit Detection und Validation
type Language struct {
	Name       string     `yaml:"name"`
	Version    string     `yaml:"version,omitempty"`
	Detection  Detection  `yaml:"detection"`
	Validation Validation `yaml:"validation"`
}

// TargetType definiert den Typ eines Targets
type TargetType string

const (
	TargetTypeJob       TargetType = "job"       // Einmaliger CI-Job
	TargetTypeService   TargetType = "service"   // Langlebiger Service (Cloud Run, K8s, etc.)
	TargetTypeFunction  TargetType = "function"  // Serverless Function
	TargetTypeStatic    TargetType = "static"    // Statische Dateien (S3, CDN, etc.)
	TargetTypeContainer TargetType = "container" // Container Image
)

// TargetTemplate definiert ein wiederverwendbares Deployment-Template
type TargetTemplate struct {
	Name     string            `yaml:"name"`
	Defaults map[string]string `yaml:"defaults,omitempty"` // Default-Werte für Parameter
	Deploy   []Step            `yaml:"deploy,omitempty"`   // Deployment-Schritte (mit $PARAM Platzhaltern)
}

// Config ist die Hauptkonfiguration (build.bear)
type Config struct {
	Name      string           `yaml:"name"`
	Languages []Language       `yaml:"languages"`
	Targets   []TargetTemplate `yaml:"targets,omitempty"`
}

// Artifact definiert ein einzelnes Artefakt (bear.artifact.yml oder bear.lib.yml)
type Artifact struct {
	Name      string            `yaml:"name"`
	Target    string            `yaml:"target,omitempty"`     // Referenz auf TargetTemplate (leer bei Libraries)
	Params    map[string]string `yaml:"params,omitempty"`     // Parameter für das Target
	DependsOn []string          `yaml:"depends_on,omitempty"` // Abhängigkeiten zu anderen Artefakten
	IsLib     bool              `yaml:"-"`                    // Wird vom Scanner gesetzt, nicht aus YAML
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// LoadArtifact lädt eine bear.artifact Datei
func LoadArtifact(path string) (*Artifact, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var artifact Artifact
	if err := yaml.Unmarshal(data, &artifact); err != nil {
		return nil, err
	}

	return &artifact, nil
}

// LockEntry enthält den Deployment-Status eines Artefakts
type LockEntry struct {
	Commit    string `yaml:"commit"`            // Letzter erfolgreich deployeter Commit
	Timestamp string `yaml:"timestamp"`         // Zeitpunkt des Deployments
	Version   string `yaml:"version,omitempty"` // Optionale Version
	Target    string `yaml:"target"`            // Verwendetes Target-Template
}

// LockFile enthält den Deployment-Status aller Artefakte
type LockFile struct {
	Artifacts map[string]LockEntry `yaml:"artifacts"`
}

// LoadLock lädt die bear.lock.yml Datei
func LoadLock(path string) (*LockFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Neue Lock-Datei erstellen
			return &LockFile{Artifacts: make(map[string]LockEntry)}, nil
		}
		return nil, err
	}

	var lock LockFile
	if err := yaml.Unmarshal(data, &lock); err != nil {
		return nil, err
	}

	if lock.Artifacts == nil {
		lock.Artifacts = make(map[string]LockEntry)
	}

	return &lock, nil
}

// Save speichert die Lock-Datei
func (l *LockFile) Save(path string) error {
	data, err := yaml.Marshal(l)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// GetLastDeployedCommit gibt den letzten deployeten Commit für ein Artefakt zurück
func (l *LockFile) GetLastDeployedCommit(artifactName string) string {
	if entry, ok := l.Artifacts[artifactName]; ok {
		return entry.Commit
	}
	return ""
}

// UpdateArtifact aktualisiert den Deployment-Status eines Artefakts
func (l *LockFile) UpdateArtifact(artifactName, commit, target, version string) {
	l.Artifacts[artifactName] = LockEntry{
		Commit:    commit,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version:   version,
		Target:    target,
	}
}
