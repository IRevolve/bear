package config

import (
	"os"
	"time"

	toml "github.com/pelletier/go-toml/v2"
)

// LockEntry contains the deployment status of an artifact
type LockEntry struct {
	Commit    string `toml:"commit"`            // Last successfully deployed commit
	Timestamp string `toml:"timestamp"`         // Time of deployment
	Version   string `toml:"version,omitempty"` // Optional version
	Target    string `toml:"target"`            // Used target template
	Pinned    bool   `toml:"pinned,omitempty"`  // If true, this artifact is not automatically updated
}

// LockFile contains the deployment status of all artifacts
type LockFile struct {
	Artifacts map[string]LockEntry `toml:"artifacts"`
}

// LoadLock loads the bear.lock.toml file
func LoadLock(path string) (*LockFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Create new lock file
			return &LockFile{Artifacts: make(map[string]LockEntry)}, nil
		}
		return nil, err
	}

	var lock LockFile
	if err := toml.Unmarshal(data, &lock); err != nil {
		return nil, err
	}

	if lock.Artifacts == nil {
		lock.Artifacts = make(map[string]LockEntry)
	}

	return &lock, nil
}

// Save saves the lock file
func (l *LockFile) Save(path string) error {
	data, err := toml.Marshal(l)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// GetLastDeployedCommit returns the last deployed commit for an artifact
func (l *LockFile) GetLastDeployedCommit(artifactName string) string {
	if entry, ok := l.Artifacts[artifactName]; ok {
		return entry.Commit
	}
	return ""
}

// IsPinned checks if an artifact is pinned
func (l *LockFile) IsPinned(artifactName string) bool {
	if entry, ok := l.Artifacts[artifactName]; ok {
		return entry.Pinned
	}
	return false
}

// UpdateArtifact updates the deployment status of an artifact
func (l *LockFile) UpdateArtifact(artifactName, commit, target, version string) {
	l.Artifacts[artifactName] = LockEntry{
		Commit:    commit,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version:   version,
		Target:    target,
		Pinned:    false,
	}
}

// UpdateArtifactPinned updates the deployment status and pins the artifact
func (l *LockFile) UpdateArtifactPinned(artifactName, commit, target, version string) {
	l.Artifacts[artifactName] = LockEntry{
		Commit:    commit,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version:   version,
		Target:    target,
		Pinned:    true,
	}
}
