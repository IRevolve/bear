//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !windows

package cmd

import (
	"fmt"
	"os"
)

func lockWorkspaceFile(f *os.File) error {
	return fmt.Errorf("advisory workspace locking is unsupported on this platform")
}
