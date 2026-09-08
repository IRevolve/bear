//go:build !windows

package cmd

import (
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"
)

func configureProcessTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = sync.OnceValue(func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	})
}

func cleanupProcessTree(cmd *exec.Cmd) {
	if cmd.Process != nil {
		// Signal the group immediately after Wait fails: the leader may already
		// be reaped, but descendants still hold its process group and pipes.
		// Share the one-shot cancellation signal; never retry a stale group ID.
		_ = cmd.Cancel()
	}
}
