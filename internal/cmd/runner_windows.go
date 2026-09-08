//go:build windows

package cmd

import (
	"context"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
)

func configureProcessTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
	cmd.Cancel = sync.OnceValue(func() error {
		ctx, cancel := context.WithTimeout(context.Background(), stepWaitDelay)
		defer cancel()
		kill := exec.CommandContext(ctx, "taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
		kill.WaitDelay = stepWaitDelay
		if err := kill.Run(); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	})
}

func cleanupProcessTree(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Cancel()
	}
}
