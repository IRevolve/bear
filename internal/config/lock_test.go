package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLockEnvironmentHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bear.lock.yml")
	lock, err := LoadLock(path)
	if err != nil || lock.Environments == nil || len(lock.Environments) != 0 {
		t.Fatalf("new lock: %+v, %v", lock, err)
	}
	lock.UpdateArtifactPinned("dev", "api", "dev-commit", "local", "v1")
	if lock.GetLastDeployedCommit("prd", "api") != "" || lock.IsPinned("prd", "api") {
		t.Fatal("dev history leaked into prd")
	}
	lock.UpdateArtifact("prd", "api", "prd-commit", "remote", "v2")
	if !lock.IsPinned("dev", "api") || lock.IsPinned("prd", "api") {
		t.Fatal("pins must be scoped to their environment")
	}
	entry, ok := lock.GetArtifact("dev", "api")
	if !ok || entry.Commit != "dev-commit" || entry.Target != "local" || entry.Version != "v1" {
		t.Fatalf("unexpected history: %+v", entry)
	}
	stamp, err := time.Parse(time.RFC3339, entry.Timestamp)
	if err != nil || time.Since(stamp) > time.Minute {
		t.Fatalf("invalid timestamp %q: %v", entry.Timestamp, err)
	}
	if err := lock.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadLock(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.GetLastDeployedCommit("prd", "api") != "prd-commit" || !loaded.IsPinned("dev", "api") {
		t.Fatalf("round trip lost history: %+v", loaded)
	}
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "environments:") || strings.Contains(string(data), "artifacts:") {
		t.Fatalf("unexpected schema: %s (%v)", data, err)
	}
	loaded.UpdateArtifact("dev", "api", "new", "local", "v3")
	if loaded.IsPinned("dev", "api") || loaded.GetLastDeployedCommit("prd", "api") != "prd-commit" {
		t.Fatal("update must unpin only the updated environment")
	}
}

func TestLockLegacyHistoryIsNotEnvironmentHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bear.lock.yml")
	if err := os.WriteFile(path, []byte("artifacts:\n  api:\n    commit: old\n    pinned: true\n"), 0644); err != nil {
		t.Fatal(err)
	}
	lock, err := LoadLock(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, environment := range []string{"dev", "int", "prd"} {
		if _, ok := lock.GetArtifact(environment, "api"); ok || lock.IsPinned(environment, "api") || lock.GetLastDeployedCommit(environment, "api") != "" {
			t.Fatalf("legacy history inferred for %s", environment)
		}
	}
	lock.UpdateArtifact("dev", "api", "new", "local", "")
	if err := lock.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadLock(path)
	if err != nil || loaded.Artifacts["api"].Commit != "old" || loaded.GetLastDeployedCommit("dev", "api") != "new" {
		t.Fatalf("migration data not preserved: %+v, %v", loaded, err)
	}
}

func TestLockZeroValueAndErrors(t *testing.T) {
	lock := &LockFile{}
	lock.UpdateArtifactPinned("dev", "api", "commit", "local", "")
	if !lock.IsPinned("dev", "api") {
		t.Fatal("zero value update failed")
	}
	root := t.TempDir()
	if _, err := LoadLock(root); err == nil {
		t.Fatal("expected read error")
	}
	if err := lock.Save(root); err == nil {
		t.Fatal("expected write error")
	}
	path := filepath.Join(root, "invalid.yml")
	if err := os.WriteFile(path, []byte("environments: ["), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadLock(path); err == nil {
		t.Fatal("expected YAML error")
	}
}
