package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadLock_NewFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "bear-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	lockPath := filepath.Join(tmpDir, "bear.lock.yml")

	// Load non-existent file should create empty lock
	lock, err := LoadLock(lockPath)
	if err != nil {
		t.Fatalf("LoadLock failed: %v", err)
	}

	if lock.Artifacts == nil {
		t.Error("expected Artifacts map to be initialized")
	}
	if len(lock.Artifacts) != 0 {
		t.Errorf("expected 0 artifacts, got %d", len(lock.Artifacts))
	}
}

func TestLoadLock_ExistingFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "bear-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	lockContent := `artifacts:
  user-api:
    commit: abc1234
    timestamp: "2026-01-01T10:00:00Z"
    version: v1.0.0
    target: cloudrun
    pinned: true
`
	lockPath := filepath.Join(tmpDir, "bear.lock.yml")
	if err := os.WriteFile(lockPath, []byte(lockContent), 0644); err != nil {
		t.Fatalf("failed to write lock: %v", err)
	}

	lock, err := LoadLock(lockPath)
	if err != nil {
		t.Fatalf("LoadLock failed: %v", err)
	}

	if len(lock.Artifacts) != 1 {
		t.Errorf("expected 1 artifact, got %d", len(lock.Artifacts))
	}

	entry, ok := lock.Artifacts["user-api"]
	if !ok {
		t.Fatal("expected 'user-api' artifact")
	}
	if entry.Commit != "abc1234" {
		t.Errorf("expected commit 'abc1234', got '%s'", entry.Commit)
	}
	if !entry.Pinned {
		t.Error("expected artifact to be pinned")
	}
}

func TestLockFile_GetLastDeployedCommit(t *testing.T) {
	lock := &LockFile{
		Artifacts: map[string]LockEntry{
			"api": {Commit: "abc123"},
		},
	}

	// Existing artifact
	commit := lock.GetLastDeployedCommit("api")
	if commit != "abc123" {
		t.Errorf("expected 'abc123', got '%s'", commit)
	}

	// Non-existent artifact
	commit = lock.GetLastDeployedCommit("nonexistent")
	if commit != "" {
		t.Errorf("expected empty string, got '%s'", commit)
	}
}

func TestLockFile_IsPinned(t *testing.T) {
	lock := &LockFile{
		Artifacts: map[string]LockEntry{
			"pinned-api":   {Pinned: true},
			"unpinned-api": {Pinned: false},
		},
	}

	if !lock.IsPinned("pinned-api") {
		t.Error("expected pinned-api to be pinned")
	}
	if lock.IsPinned("unpinned-api") {
		t.Error("expected unpinned-api to not be pinned")
	}
	if lock.IsPinned("nonexistent") {
		t.Error("expected nonexistent to not be pinned")
	}
}

func TestLockFile_UpdateArtifact(t *testing.T) {
	lock := &LockFile{Artifacts: make(map[string]LockEntry)}

	lock.UpdateArtifact("my-service", "abc123", "cloudrun", "v1.0")

	entry, ok := lock.Artifacts["my-service"]
	if !ok {
		t.Fatal("expected artifact to exist")
	}
	if entry.Commit != "abc123" {
		t.Errorf("expected commit 'abc123', got '%s'", entry.Commit)
	}
	if entry.Target != "cloudrun" {
		t.Errorf("expected target 'cloudrun', got '%s'", entry.Target)
	}
	if entry.Pinned {
		t.Error("expected artifact to not be pinned")
	}

	// Check timestamp is recent
	ts, err := time.Parse(time.RFC3339, entry.Timestamp)
	if err != nil {
		t.Fatalf("invalid timestamp format: %v", err)
	}
	if time.Since(ts) > time.Minute {
		t.Error("timestamp should be recent")
	}
}

func TestLockFile_UpdateArtifactPinned(t *testing.T) {
	lock := &LockFile{Artifacts: make(map[string]LockEntry)}

	lock.UpdateArtifactPinned("my-service", "abc123", "cloudrun", "v1.0")

	entry, ok := lock.Artifacts["my-service"]
	if !ok {
		t.Fatal("expected artifact to exist")
	}
	if !entry.Pinned {
		t.Error("expected artifact to be pinned")
	}
}

func TestLockFile_Save(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "bear-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	lock := &LockFile{
		Artifacts: map[string]LockEntry{
			"test": {Commit: "xyz789", Target: "docker"},
		},
	}

	lockPath := filepath.Join(tmpDir, "bear.lock.yml")
	if err := lock.Save(lockPath); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load it back
	loaded, err := LoadLock(lockPath)
	if err != nil {
		t.Fatalf("LoadLock failed: %v", err)
	}

	if loaded.Artifacts["test"].Commit != "xyz789" {
		t.Errorf("expected commit 'xyz789', got '%s'", loaded.Artifacts["test"].Commit)
	}
}
