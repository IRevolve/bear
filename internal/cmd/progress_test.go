package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

func TestFitProgressLine(t *testing.T) {
	line := "\033[36m" + strings.Repeat("build ", 20) + "\033[0m"
	for _, width := range []int{1, 3, 10, 79} {
		got := fitProgressLine(line, width)
		if len(got) > width || strings.Contains(got, "\033") || !utf8.ValidString(got) {
			t.Fatalf("invalid truncated line for width %d: %q", width, got)
		}
	}
	short := "\033[36mbuild\033[0m"
	if fitProgressLine(short, 10) != short {
		t.Fatal("short colored line was modified")
	}
}

func progressOutput(pt *ProgressTracker, out *bytes.Buffer) string {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	return out.String()
}

func TestProgressNonTTYReportsBeforeStop(t *testing.T) {
	var out bytes.Buffer
	pt := NewProgressTracker(NewPrinterWithWriter(&out), "Deploying", []string{"fast", "slow"})
	pt.Start()
	pt.MarkRunning(0)
	pt.MarkStep(0, "deploy / build (1/2)")
	pt.MarkRunning(1)
	pt.MarkStep(1, "deploy / deploy (1/1)")
	pt.MarkFailed(0, errors.New("exit status 1"), "build error\n")
	output := progressOutput(pt, &out)
	for _, want := range []string{"0/2", "1/2", "fast | running", "deploy / build (1/2) | phase", "fast | failed", "exit status 1", "build error", "slow | running"} {
		if !strings.Contains(output, want) {
			t.Errorf("missing %q in live output:\n%s", want, output)
		}
	}
	pt.MarkDone(1)
	beforeStop := progressOutput(pt, &out)
	pt.Stop()
	if out.String() != beforeStop {
		t.Fatal("Stop replayed non-TTY completion output")
	}
	if strings.Contains(out.String(), "\033[") || strings.Contains(out.String(), "\r") {
		t.Fatal("non-TTY progress contains terminal control sequences")
	}
	if strings.Count(out.String(), "build error") != 1 {
		t.Fatal("failure output should appear exactly once")
	}
	if !strings.Contains(out.String(), "2/2") || !pt.HasFailures() {
		t.Fatal("missing final progress or failure state")
	}
}

func TestProgressPhaseTimerAndHeartbeat(t *testing.T) {
	var out bytes.Buffer
	pt := NewProgressTracker(NewPrinterWithWriter(&out), "Validating", []string{"api"})
	pt.Start()
	defer pt.Stop()
	pt.MarkRunning(0)
	pt.MarkStep(0, "validate / build (1/2)")
	pt.mu.Lock()
	pt.tasks[0].PhaseStartTime = time.Now().Add(-12 * time.Second)
	pt.tasks[0].StartTime = time.Now().Add(-20 * time.Second)
	out.Reset()
	pt.mu.Unlock()

	// A silent command must still produce periodic output without state changes.
	deadline := time.After(7 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for !strings.Contains(progressOutput(pt, &out), "validate / build (1/2) | phase") {
		select {
		case <-deadline:
			t.Fatal("no periodic non-TTY progress while the step was running")
		case <-ticker.C:
		}
	}
	pt.MarkStep(0, "validate / test (2/2)")
	output := progressOutput(pt, &out)
	if !strings.Contains(output, "validate / build (1/2) completed in") || !strings.Contains(output, "validate / test (2/2) | phase 0.0s") {
		t.Fatalf("phase completion or reset missing:\n%s", output)
	}
	pt.mu.Lock()
	pt.tasks[0].PhaseStartTime = time.Now().Add(-3 * time.Second)
	pt.mu.Unlock()
	pt.MarkDone(0)
	if !strings.Contains(progressOutput(pt, &out), "api | done | validate / test (2/2) | phase 3.0s") {
		t.Fatal("completed phase duration missing")
	}
}

func TestProgressConcurrentUpdatesAndShutdown(t *testing.T) {
	for _, tty := range []bool{false, true} {
		t.Run(fmt.Sprintf("tty=%t", tty), func(t *testing.T) {
			var out bytes.Buffer
			names := make([]string, 20)
			for i := range names {
				names[i] = fmt.Sprintf("api-%d", i)
			}
			pt := NewProgressTracker(NewPrinterWithWriter(&out), "Deploying", names)
			pt.isTTY = tty
			pt.Start()
			var wg sync.WaitGroup
			for i := range names {
				wg.Go(func() {
					pt.MarkRunning(i)
					pt.MarkStep(i, "build")
					pt.render()
					pt.MarkStep(i, "deploy")
					pt.MarkDone(i)
				})
			}
			wg.Wait()
			pt.Stop()
			select {
			case <-pt.finished:
			default:
				t.Fatal("renderer was not joined")
			}
			elapsed := pt.TotalElapsed()
			time.Sleep(time.Millisecond)
			if pt.TotalElapsed() != elapsed {
				t.Fatal("elapsed time kept increasing after Stop")
			}
			if !strings.Contains(out.String(), "deploy | phase") {
				t.Fatal("phase and timer missing from task display")
			}
		})
	}
}

func TestStepWriterConcurrentLinesAndBoundedPartials(t *testing.T) {
	var out bytes.Buffer
	pt := NewProgressTracker(NewPrinterWithWriter(&out), "Build", []string{"api", "web"})
	pt.isTTY = true
	pt.UsePlainOutput()
	pt.Start()
	var wg sync.WaitGroup
	for index := range 2 {
		writer := pt.StepWriter(index, "compile")
		for worker := range 8 {
			wg.Go(func() {
				for line := range 20 {
					_, _ = fmt.Fprintf(writer, "worker-%d-line-%d\n", worker, line)
				}
			})
		}
	}
	wg.Wait()
	for _, name := range []string{"api", "web"} {
		for worker := range 8 {
			for line := range 20 {
				want := fmt.Sprintf("  %s | compile | worker-%d-line-%d\n", name, worker, line)
				if strings.Count(progressOutput(pt, &out), want) != 1 {
					t.Fatalf("interleaved or missing line %q", want)
				}
			}
		}
	}
	writer := pt.StepWriter(0, "compile").(*stepWriter)
	_, _ = io.WriteString(writer, strings.Repeat("x", 100*stepLineSize+3))
	pt.mu.Lock()
	if writer.size != 3 || len(pt.writers) != 1 || len(writer.partial) != stepLineSize {
		t.Error("partial line storage is not bounded")
	}
	pt.mu.Unlock()
	pt.MarkDone(0)
	pt.MarkDone(1)
	pt.Stop()
	if strings.Contains(out.String(), "\033[") || !strings.Contains(out.String(), "api | compile | xxx\n") {
		t.Fatal("plain output has cursor controls or lost partial line")
	}
	if len(pt.writers) != 0 {
		t.Fatal("finished writers retained")
	}
}

func TestProgressFailuresLiveWithoutReplay(t *testing.T) {
	for _, tty := range []bool{false, true} {
		for _, streamed := range []bool{false, true} {
			t.Run(fmt.Sprintf("tty=%t/stream=%t", tty, streamed), func(t *testing.T) {
				var out bytes.Buffer
				pt := NewProgressTracker(NewPrinterWithWriter(&out), "Build", []string{"api"})
				pt.isTTY = tty
				pt.Start()
				pt.MarkRunning(0)
				pt.MarkStep(0, "compile")
				pt.render()
				if streamed {
					_, _ = io.WriteString(pt.StepWriter(0, "compile"), "unique-output")
				}
				pt.MarkFailed(0, errors.New("unique-error"), "unique-output")
				live := progressOutput(pt, &out)
				if strings.Count(live, "unique-output") != 1 || strings.Count(live, "unique-error") != 1 {
					t.Fatalf("error details missing or duplicated live: %s", live)
				}
				pt.Stop()
				if strings.Count(out.String(), "unique-output") != 1 || strings.Count(out.String(), "unique-error") != 1 {
					t.Fatalf("error details replayed at stop: %s", out.String())
				}
			})
		}
	}
}

func TestProgressLiveLinesFitHeight(t *testing.T) {
	names := make([]string, 100)
	for i := range names {
		names[i] = fmt.Sprintf("task-%d", i)
	}
	pt := NewProgressTracker(NewPrinterWithWriter(io.Discard), "Build", names)
	pt.isTTY = true
	pt.tasks[90].Status = TaskRunning
	pt.tasks[91].Status = TaskDone
	for _, height := range []int{1, 2, 3, 4, 10, 24, 120} {
		lines := pt.liveLines(height)
		if len(lines) > max(1, height-1) {
			t.Fatalf("height %d: rendered %d rows", height, len(lines))
		}
		output := strings.Join(lines, "\n")
		if height >= 4 && !strings.Contains(output, "task-90") {
			t.Fatalf("height %d: running task not prioritized", height)
		}
		if height < 100 && (!strings.Contains(output, "pending") || !strings.Contains(output, "completed hidden")) {
			t.Fatalf("height %d: hidden counts missing: %s", height, output)
		}
	}
}

func TestStepWriterChunkBoundary(t *testing.T) {
	var out bytes.Buffer
	pt := NewProgressTracker(NewPrinterWithWriter(&out), "Build", []string{"api"})
	w := pt.StepWriter(0, "compile")
	_, _ = io.WriteString(w, strings.Repeat("x", stepLineSize))
	_, _ = io.WriteString(w, "\n\nlast")
	pt.MarkDone(0)
	if strings.Count(out.String(), "api | compile | ") != 3 {
		t.Fatalf("extra or missing lines at chunk boundary: %q", out.String())
	}
}
