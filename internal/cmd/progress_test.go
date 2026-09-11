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

// Status lines report whole seconds so repeated CI heartbeats stay diffable.
func TestFormatElapsed(t *testing.T) {
	for _, test := range []struct {
		in   time.Duration
		want string
	}{
		{0, "0s"},
		{499 * time.Millisecond, "0s"},
		{1500 * time.Millisecond, "2s"},
		{20 * time.Second, "20s"},
		{32 * time.Second, "32s"},
		{59500 * time.Millisecond, "1m00s"},
		{65 * time.Second, "1m05s"},
		{time.Hour + 90*time.Second, "61m30s"},
	} {
		if got := formatElapsed(test.in); got != test.want {
			t.Errorf("formatElapsed(%s) = %q, want %q", test.in, got, test.want)
		}
	}
}

func progressOutput(pt *ProgressTracker, out *bytes.Buffer) string {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	return out.String()
}

func TestProgressNonTTYReportsBeforeStop(t *testing.T) {
	var out bytes.Buffer
	pt := NewProgressTracker(NewPrinterWithWriter(&out), []string{"fast", "slow"})
	pt.SetOperation("deploy")
	pt.Start()
	pt.MarkRunning(0)
	pt.MarkStep(0, "build", 1, 2)
	pt.MarkRunning(1)
	pt.MarkStep(1, "deploy", 1, 1)
	pt.MarkFailed(0, errors.New("exit status 1"), "build error\n")
	output := progressOutput(pt, &out)
	// One line per job on every status change: running, current step, outcome.
	for _, want := range []string{
		"  fast: Deploying...\n",
		"  fast: Deploying... [1/2 build]\n",
		"  slow: Deploying...\n",
		"  slow: Deploying... [deploy]\n",
		"  fast: Deployment failed after ",
		": exit status 1\n",
		"build error",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("missing %q in live output:\n%s", want, output)
		}
	}
	// The ASCII bar and the "name | status | phase ... | total ..." table are
	// gone from plain output; every job reports on its own line instead.
	for _, unwanted := range []string{"| total ", "| phase ", "fast | ", "slow | ", "0/2", "2/2"} {
		if strings.Contains(output, unwanted) {
			t.Errorf("plain output still renders %q:\n%s", unwanted, output)
		}
	}
	pt.MarkDone(1)
	beforeStop := progressOutput(pt, &out)
	if !strings.Contains(beforeStop, "  slow: Deployment complete after ") {
		t.Fatalf("completion not reported live:\n%s", beforeStop)
	}
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
	if strings.Count(out.String(), "Deployment failed after ") != 1 || !pt.HasFailures() {
		t.Fatal("missing final outcome or failure state")
	}
}

// Each command names its own lifecycle, and an unknown kind keeps the neutral
// default rather than silently reporting the wrong operation.
func TestProgressOperationVerbs(t *testing.T) {
	for _, test := range []struct{ kind, active, still, done, failed string }{
		{"deploy", "Deploying", "Still deploying", "Deployment complete", "Deployment failed"},
		{"validate", "Validating", "Still validating", "Validation complete", "Validation failed"},
		{"check", "Checking", "Still checking", "Check complete", "Check failed"},
		{"unknown", "Running", "Still running", "Complete", "Failed"},
	} {
		t.Run(test.kind, func(t *testing.T) {
			var out bytes.Buffer
			pt := NewProgressTracker(NewPrinterWithWriter(&out), []string{"api", "web"})
			pt.SetOperation(test.kind)
			pt.MarkRunning(0)
			pt.MarkStep(0, "step", 1, 1)
			pt.render()
			pt.MarkDone(0)
			pt.MarkRunning(1)
			pt.MarkFailed(1, errors.New("boom"), "")
			for _, want := range []string{
				"  api: " + test.active + "...\n",
				"  api: " + test.active + "... [step]\n",
				// The backlog rides on the last running job's line instead of
				// taking a line of its own.
				"  api: " + test.still + "... [0s elapsed, step] (1 job queued)\n",
				"  api: " + test.done + " after 0s\n",
				"  web: " + test.failed + " after 0s: boom\n",
			} {
				if !strings.Contains(out.String(), want) {
					t.Errorf("missing %q:\n%s", want, out.String())
				}
			}
			// One heartbeat reports the backlog exactly once, never separately.
			if got := strings.Count(out.String(), "queued"); got != 1 {
				t.Errorf("queued backlog reported %d times, want 1:\n%s", got, out.String())
			}
		})
	}
}

// Job names are padded to a common width so plain status lines form columns,
// and a heartbeat appends the remaining backlog to the last running job rather
// than spending a line of its own on it.
func TestProgressColumnsAndQueuedBacklog(t *testing.T) {
	var out bytes.Buffer
	names := []string{"kira-mail-adapter", "kira-teams-adapter", "api", "web"}
	pt := NewProgressTracker(NewPrinterWithWriter(&out), names)
	pt.SetOperation("validate")
	pt.MarkRunning(0)
	pt.MarkStep(0, "Test", 1, 2)
	pt.MarkRunning(1)
	pt.render() // two running, two queued
	pt.MarkDone(1)
	pt.MarkRunning(2)
	pt.render() // two running, one queued
	pt.MarkRunning(3)
	pt.MarkFailed(3, errors.New("boom"), "")
	for _, want := range []string{
		// The widest name sets the column; shorter names are padded to it.
		"  kira-mail-adapter:  Validating... [1/2 Test]\n",
		"  kira-teams-adapter: Validating...\n",
		"  api:                Validating...\n",
		"  kira-teams-adapter: Validation complete after 0s\n",
		"  web:                Validation failed after 0s: boom\n",
		// Only the last running job carries the backlog, and it is pluralised.
		"  kira-mail-adapter:  Still validating... [0s elapsed, 1/2 Test]\n",
		"  kira-teams-adapter: Still validating... [0s elapsed] (2 jobs queued)\n",
		"  api:                Still validating... [0s elapsed] (1 job queued)\n",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q:\n%s", want, out.String())
		}
	}
	// Two heartbeats, so the backlog is reported twice and never on its own line.
	if got := strings.Count(out.String(), "queued"); got != 2 {
		t.Errorf("queued backlog reported %d times, want 2:\n%s", got, out.String())
	}
	const column = len("  kira-teams-adapter: ")
	for _, line := range strings.Split(strings.TrimRight(out.String(), "\n"), "\n") {
		if len(line) <= column || line[column] == ' ' || !strings.HasSuffix(strings.TrimRight(line[:column], " "), ":") {
			t.Errorf("line is not padded to the name column: %q", line)
		}
	}
}

func TestProgressPhaseTimerAndHeartbeat(t *testing.T) {
	var out bytes.Buffer
	pt := NewProgressTracker(NewPrinterWithWriter(&out), []string{"api", "web"})
	pt.SetOperation("validate")
	// Test-only cadence override: Start picks the 80ms terminal interval, then
	// the tracker switches to plain rendering, so the real renderer goroutine
	// exercises the heartbeat path without waiting heartbeatInterval. Elapsed
	// times are driven through the task fields, never by sleeping.
	pt.isTTY = true
	pt.Start()
	defer pt.Stop()
	pt.UsePlainOutput()
	pt.MarkRunning(0)
	pt.MarkStep(0, "build", 1, 2)
	pt.mu.Lock()
	pt.tasks[0].PhaseStartTime = time.Now().Add(-12 * time.Second)
	pt.tasks[0].StartTime = time.Now().Add(-20 * time.Second)
	out.Reset()
	pt.mu.Unlock()

	// A silent command must still produce periodic output without state changes.
	deadline := time.After(7 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for !strings.Contains(progressOutput(pt, &out), "  api: Still validating... [20s elapsed, 1/2 build] (1 job queued)\n") {
		select {
		case <-deadline:
			t.Fatalf("no periodic non-TTY progress while the step was running:\n%s", progressOutput(pt, &out))
		case <-ticker.C:
		}
	}
	// The backlog is appended to the last running job, never printed alone, so
	// every heartbeat line carries it while a job is still queued.
	snapshot := progressOutput(pt, &out)
	if beats := strings.Count(snapshot, "Still validating"); beats == 0 || beats != strings.Count(snapshot, " (1 job queued)\n") {
		t.Fatalf("queued backlog is not appended to every heartbeat line:\n%s", snapshot)
	}

	pt.MarkStep(0, "test", 2, 2)
	output := progressOutput(pt, &out)
	if !strings.Contains(output, "  api: Validating... [2/2 test]\n") {
		t.Fatalf("new step not reported:\n%s", output)
	}
	pt.mu.Lock()
	phase, duration, since := pt.tasks[0].Phase, pt.tasks[0].PhaseDuration, time.Since(pt.tasks[0].PhaseStartTime)
	pt.mu.Unlock()
	if phase != "2/2 test" || duration != 0 || since > time.Second {
		t.Fatalf("phase timer not reset: %q, %s, %s", phase, duration, since)
	}
	pt.mu.Lock()
	pt.tasks[0].PhaseStartTime = time.Now().Add(-3 * time.Second)
	pt.tasks[0].StartTime = time.Now().Add(-32 * time.Second)
	pt.mu.Unlock()
	pt.MarkDone(0)
	if !strings.Contains(progressOutput(pt, &out), "  api: Validation complete after 32s\n") {
		t.Fatalf("completion elapsed time missing:\n%s", progressOutput(pt, &out))
	}
	pt.mu.Lock()
	phaseDuration := pt.tasks[0].PhaseDuration.Round(time.Second)
	pt.mu.Unlock()
	if phaseDuration != 3*time.Second {
		t.Fatalf("completed phase duration = %s", phaseDuration)
	}
}

// Plain output must stay readable in CI logs: running jobs report on the
// heartbeat cadence only, never on the terminal frame rate.
func TestProgressPlainHeartbeatIsThrottled(t *testing.T) {
	var out bytes.Buffer
	pt := NewProgressTracker(NewPrinterWithWriter(&out), []string{"api"})
	pt.SetOperation("validate")
	pt.Start()
	defer pt.Stop()
	pt.MarkRunning(0)
	pt.mu.Lock()
	out.Reset()
	pt.mu.Unlock()
	time.Sleep(200 * time.Millisecond)
	if got := progressOutput(pt, &out); got != "" {
		t.Fatalf("plain heartbeat is faster than %s:\n%s", heartbeatInterval, got)
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
			pt := NewProgressTracker(NewPrinterWithWriter(&out), names)
			pt.SetOperation("deploy")
			pt.isTTY = tty
			pt.Start()
			var wg sync.WaitGroup
			for i := range names {
				wg.Go(func() {
					pt.MarkRunning(i)
					pt.MarkStep(i, "build", 1, 2)
					pt.render()
					pt.MarkStep(i, "deploy", 2, 2)
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
			want := []string{"2/2 deploy | phase "}
			if !tty {
				// Names are padded to the widest name, so "api-0" carries one
				// more space than "api-19" and both verbs share a column.
				want = []string{
					"  api-0:  Deploying... [2/2 deploy]\n",
					"  api-0:  Deployment complete after ",
					"  api-19: Deploying... [2/2 deploy]\n",
					"  api-19: Deployment complete after ",
				}
			}
			for _, text := range want {
				if !strings.Contains(out.String(), text) {
					t.Fatalf("missing %q in task display", text)
				}
			}
			if tty {
				return
			}
			// Plain mode writes one whole line per status change and per outcome:
			// 3 status lines (running, step 1, step 2) and 1 outcome per job.
			if got := strings.Count(out.String(), "Deploying...\n"); got != len(names) {
				t.Fatalf("running lines = %d, want %d", got, len(names))
			}
			if got := strings.Count(out.String(), "Deploying... ["); got != 2*len(names) {
				t.Fatalf("step lines = %d, want %d", got, 2*len(names))
			}
			if got := strings.Count(out.String(), "Deployment complete after "); got != len(names) {
				t.Fatalf("completion lines = %d, want %d", got, len(names))
			}
		})
	}
}

func TestStepWriterConcurrentLinesAndBoundedPartials(t *testing.T) {
	var out bytes.Buffer
	pt := NewProgressTracker(NewPrinterWithWriter(&out), []string{"api", "web"})
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
				pt := NewProgressTracker(NewPrinterWithWriter(&out), []string{"api"})
				pt.isTTY = tty
				pt.Start()
				pt.MarkRunning(0)
				pt.MarkStep(0, "compile", 1, 1)
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
	pt := NewProgressTracker(NewPrinterWithWriter(io.Discard), names)
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
	pt := NewProgressTracker(NewPrinterWithWriter(&out), []string{"api"})
	w := pt.StepWriter(0, "compile")
	_, _ = io.WriteString(w, strings.Repeat("x", stepLineSize))
	_, _ = io.WriteString(w, "\n\nlast")
	pt.MarkDone(0)
	if strings.Count(out.String(), "api | compile | ") != 3 {
		t.Fatalf("extra or missing lines at chunk boundary: %q", out.String())
	}
}
