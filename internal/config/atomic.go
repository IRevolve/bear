package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// WriteFileAtomic replaces path with data and the supplied permissions. The
// parent directory must exist. Failures before rename leave the old file intact;
// a directory-sync failure is returned even though the replacement is visible.
// On Windows directory sync is unavailable and rename has OS-specific guarantees.
func WriteFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".bear-write-*")
	if err != nil {
		return fmt.Errorf("create temporary file for %s: %w", path, err)
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err := f.Chmod(mode); err != nil {
		return fmt.Errorf("set permissions for %s: %w", path, err)
	}
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open directory after replacing %s: %w", path, err)
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return fmt.Errorf("sync directory after replacing %s: %w", path, err)
	}
	return nil
}
