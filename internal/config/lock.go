package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// LockEntry contains the deployment status of an artifact
type LockEntry struct {
	Commit    string `yaml:"commit"`            // Last successfully deployed commit
	Timestamp string `yaml:"timestamp"`         // Time of deployment
	Version   string `yaml:"version,omitempty"` // Optional version
	Target    string `yaml:"target"`            // Used target template
	Pinned    bool   `yaml:"pinned,omitempty"`  // If true, this artifact is not automatically updated
}

// LockFile contains the deployment status of all artifacts
type LockFile struct {
	Environments map[string]map[string]LockEntry `yaml:"environments"`
	// Preserve legacy history for explicit migration, never as environment history.
	Artifacts map[string]LockEntry `yaml:"artifacts,omitempty"`
}

// LoadLock loads the bear.lock.yml file
func LoadLock(path string) (*LockFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Create new lock file
			return &LockFile{Environments: make(map[string]map[string]LockEntry)}, nil
		}
		return nil, err
	}

	var lock LockFile
	if err := yaml.Unmarshal(data, &lock); err != nil {
		return nil, err
	}

	if lock.Environments == nil {
		lock.Environments = make(map[string]map[string]LockEntry)
	}

	return &lock, nil
}

// Save saves the lock file
func (l *LockFile) Save(path string) error {
	data, err := yaml.Marshal(l)
	if err != nil {
		return err
	}

	return WriteFileAtomic(path, data, 0644)
}

// GetArtifact returns only the history recorded in the requested environment.
func (l *LockFile) GetArtifact(environment, artifactName string) (LockEntry, bool) {
	entry, ok := l.Environments[environment][artifactName]
	return entry, ok
}

// GetLastDeployedCommit returns the last deployed commit for an artifact.
func (l *LockFile) GetLastDeployedCommit(environment, artifactName string) string {
	entry, _ := l.GetArtifact(environment, artifactName)
	return entry.Commit
}

// IsPinned checks if an artifact is pinned
func (l *LockFile) IsPinned(environment, artifactName string) bool {
	entry, _ := l.GetArtifact(environment, artifactName)
	return entry.Pinned
}

// UpdateArtifact updates the deployment status of an artifact
func (l *LockFile) UpdateArtifact(environment, artifactName, commit, target, version string) {
	if l.Environments == nil {
		l.Environments = make(map[string]map[string]LockEntry)
	}
	if l.Environments[environment] == nil {
		l.Environments[environment] = make(map[string]LockEntry)
	}
	l.Environments[environment][artifactName] = LockEntry{
		Commit:    commit,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version:   version,
		Target:    target,
		Pinned:    false,
	}
}

// UpdateArtifactPinned updates the deployment status and pins the artifact
func (l *LockFile) UpdateArtifactPinned(environment, artifactName, commit, target, version string) {
	l.UpdateArtifact(environment, artifactName, commit, target, version)
	entry := l.Environments[environment][artifactName]
	entry.Pinned = true
	l.Environments[environment][artifactName] = entry
}
