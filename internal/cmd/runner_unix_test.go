//go:build !windows

package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestExecuteStepCancelsGrandchildren(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var output TailBuffer
	done := make(chan error, 1)
	go func() {
		done <- ExecuteStep(ctx, `sh -c 'sleep 60 & child=$!; printf "%s\n" "$child"; wait' & wait`, "", nil, &output, &output)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(output.String(), "\n") {
		if time.Now().After(deadline) {
			t.Fatal("grandchild did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(output.String()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ExecuteStep hung after cancellation")
	}
	// Orphaned children can briefly remain as zombies until reaped by init.
	deadline = time.Now().Add(3 * time.Second)
	for {
		psCtx, stop := context.WithTimeout(context.Background(), time.Second)
		state, err := exec.CommandContext(psCtx, "ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
		stop()
		if err != nil || strings.HasPrefix(strings.TrimSpace(string(state)), "Z") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("grandchild %d still running: %s", pid, state)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestExecuteStepWaitDelay(t *testing.T) {
	for _, test := range []struct {
		name     string
		exitCode int
		redirect string
	}{
		{"wait-delay", 0, ""},
		{"exit-error-with-open-pipes", 7, ""},
		{"exit-error-with-closed-pipes", 7, ">/dev/null 2>&1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output TailBuffer
			start := time.Now()
			err := ExecuteStep(context.Background(), fmt.Sprintf(`sleep 60 %s & printf '%%s\n' "$!"; exit %d`, test.redirect, test.exitCode), "", nil, &output, &output)
			if time.Since(start) > 5*time.Second {
				t.Fatalf("WaitDelay result: %v after %s", err, time.Since(start))
			}
			if test.exitCode == 0 {
				if !errors.Is(err, exec.ErrWaitDelay) {
					t.Fatalf("WaitDelay result: %v", err)
				}
			} else {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) || exitErr.ExitCode() != test.exitCode {
					t.Fatalf("exit error not preserved: %v", err)
				}
			}
			pid, parseErr := strconv.Atoi(strings.TrimSpace(output.String()))
			if parseErr != nil || pid <= 0 {
				t.Fatalf("invalid child PID %q: %v", output.String(), parseErr)
			}
			// Do not kill the child in the test: failure cleanup must terminate it.
			deadline := time.Now().Add(3 * time.Second)
			for {
				psCtx, cancel := context.WithTimeout(context.Background(), time.Second)
				state, psErr := exec.CommandContext(psCtx, "ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
				cancel()
				if psErr != nil {
					var exitErr *exec.ExitError
					if errors.As(psErr, &exitErr) && exitErr.ExitCode() == 1 && len(strings.TrimSpace(string(state))) == 0 {
						break
					}
					t.Fatalf("check child status: %v", psErr)
				}
				// A zombie is dead but awaiting reaping by the system's init.
				if strings.HasPrefix(strings.TrimSpace(string(state)), "Z") {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("child %d survived failed step: %s", pid, state)
				}
				time.Sleep(10 * time.Millisecond)
			}
		})
	}
}

func TestExecuteStepPreservesSuccessfulBackgroundChild(t *testing.T) {
	var output TailBuffer
	err := ExecuteStep(context.Background(), `sleep 2 >/dev/null 2>&1 & printf '%s\n' "$!"`, "", nil, &output, &output)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(output.String()))
	if err != nil || pid <= 0 {
		t.Fatalf("invalid child PID %q: %v", output.String(), err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	state, err := exec.CommandContext(ctx, "ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	if err != nil || strings.TrimSpace(string(state)) == "" || strings.HasPrefix(strings.TrimSpace(string(state)), "Z") {
		t.Fatalf("successful background child was terminated: %s, %v", state, err)
	}
}

func TestExecuteStepStreamsBeforeExit(t *testing.T) {
	var output TailBuffer
	pt := NewProgressTracker(NewPrinterWithWriter(&output), "Build", []string{"api"})
	pt.UsePlainOutput()
	pt.Start()
	defer pt.Stop()
	pt.MarkRunning(0)
	pt.MarkStep(0, "compile")
	var capture TailBuffer
	writer := io.MultiWriter(&capture, pt.StepWriter(0, "compile"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- ExecuteStep(ctx, `printf 'live-line\n'; sleep 1; printf 'last-line'`, "", nil, writer, writer)
	}()
	deadline := time.Now().Add(3 * time.Second)
	for !strings.Contains(output.String(), "api | compile | live-line") {
		select {
		case err := <-done:
			t.Fatalf("command exited before output streamed: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("live output not streamed")
		}
		time.Sleep(time.Millisecond)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	pt.MarkDone(0)
	for _, line := range []string{"live-line", "last-line"} {
		if strings.Count(output.String(), fmt.Sprintf("api | compile | %s", line)) != 1 {
			t.Fatalf("missing or duplicate streamed output: %s", output.String())
		}
	}
}

func TestExecuteStepSecretContentsStayLiteral(t *testing.T) {
	t.Setenv("BEAR_TEST_SECRET", "secret$DOLLAR/${OTHER}/$$")
	var output TailBuffer
	err := ExecuteStep(context.Background(), `printf '%s' "$SECRET"`, "", map[string]string{
		"SECRET": "${BEAR_TEST_SECRET}",
		"OTHER":  "not-the-secret",
	}, &output, &output)
	if err != nil || output.String() != "secret$DOLLAR/${OTHER}/$$" {
		t.Fatalf("secret output = %q, error = %v", output.String(), err)
	}
}
