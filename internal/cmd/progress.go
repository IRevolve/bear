package cmd

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/term"
)

// TaskStatus represents the state of a tracked task
type TaskStatus int

const (
	TaskPending TaskStatus = iota
	TaskRunning
	TaskDone
	TaskFailed
)

// TrackedTask represents a single task being tracked
type TrackedTask struct {
	Name           string
	Status         TaskStatus
	StartTime      time.Time
	Duration       time.Duration
	Error          error
	Output         string // captured output (shown only on error)
	Phase          string
	PhaseStartTime time.Time
	PhaseDuration  time.Duration
	streamed       bool
}

// progressVerbs names the lifecycle of one tracked operation.
type progressVerbs struct {
	active, still, done, failed string
}

var progressOperations = map[string]progressVerbs{
	"deploy":   {"Deploying", "Still deploying", "Deployment complete", "Deployment failed"},
	"validate": {"Validating", "Still validating", "Validation complete", "Validation failed"},
	"doctor":   {"Checking", "Still checking", "Check complete", "Check failed"},
}

var defaultVerbs = progressVerbs{"Running", "Still running", "Complete", "Failed"}

// ProgressTracker provides live terminal progress display with spinner,
// progress bar, timer, and real-time task completion updates. Without a
// terminal it reports one line per job on every status change plus a periodic
// update for each running job, which stays readable in CI logs.
type ProgressTracker struct {
	mu         sync.Mutex
	tasks      []*TrackedTask
	startTime  time.Time
	printer    *Printer
	isTTY      bool
	done       chan struct{}
	finished   chan struct{}
	stoppedAt  time.Time
	linesDrawn int
	spinnerIdx int
	verbs      progressVerbs
	nameWidth  int
	writers    map[*stepWriter]struct{}
}

var spinnerFrames = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

var ansiColorPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// NewProgressTracker creates a new progress tracker. Callers print their own
// phase header, so the tracker only reports the jobs themselves.
func NewProgressTracker(p *Printer, taskNames []string) *ProgressTracker {
	tasks := make([]*TrackedTask, len(taskNames))
	for i, name := range taskNames {
		tasks[i] = &TrackedTask{
			Name:   name,
			Status: TaskPending,
		}
	}

	isTTY := false
	if f, ok := p.out.(*os.File); ok {
		isTTY = term.IsTerminal(int(f.Fd()))
	}
	// Pad names so status lines form columns instead of ragged text.
	width := 0
	for _, name := range taskNames {
		width = max(width, utf8.RuneCountInString(name))
	}

	return &ProgressTracker{
		tasks:     tasks,
		startTime: time.Now(),
		printer:   p,
		isTTY:     isTTY,
		done:      make(chan struct{}),
		finished:  make(chan struct{}),
		verbs:     defaultVerbs,
		nameWidth: width,
	}
}

// SetOperation selects the verbs used for status lines ("deploy", "validate",
// or "doctor"). Call before Start.
func (pt *ProgressTracker) SetOperation(kind string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	if verbs, ok := progressOperations[kind]; ok {
		pt.verbs = verbs
	}
}

// UsePlainOutput disables cursor-based rendering. Call before Start when
// streaming verbose output or sharing the terminal with other writers.
func (pt *ProgressTracker) UsePlainOutput() {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.clearLines()
	pt.isTTY = false
}

const stepLineSize = 4096

// heartbeatInterval is how often each running job reports progress without a
// terminal, matching the cadence CI logs stay readable at.
const heartbeatInterval = 10 * time.Second

type stepWriter struct {
	tracker *ProgressTracker
	index   int
	step    string
	partial [stepLineSize]byte
	size    int
	chunked bool
}

// StepWriter streams prefixed lines under the progress display's lock. Long
// lines are split at 4 KiB; unfinished lines flush when the task changes phase
// or finishes. The returned writer is safe for concurrent stdout/stderr writes.
func (pt *ProgressTracker) StepWriter(index int, step string) io.Writer {
	return &stepWriter{tracker: pt, index: index, step: step}
}

func (w *stepWriter) Write(p []byte) (int, error) {
	pt := w.tracker
	pt.mu.Lock()
	defer pt.mu.Unlock()
	if w.index < 0 || w.index >= len(pt.tasks) {
		return 0, fmt.Errorf("invalid progress task index %d", w.index)
	}
	n := len(p)
	if n == 0 {
		return 0, nil
	}
	pt.tasks[w.index].streamed = true
	for i, b := range p {
		if b == '\n' {
			if w.size > 0 || !w.chunked {
				if err := w.flush(); err != nil {
					return i + 1, err
				}
			}
			w.chunked = false
			continue
		}
		w.partial[w.size] = b
		w.size++
		if w.size == len(w.partial) {
			if err := w.flush(); err != nil {
				return i + 1, err
			}
			w.chunked = true
		}
	}
	if w.size > 0 {
		if pt.writers == nil {
			pt.writers = make(map[*stepWriter]struct{})
		}
		pt.writers[w] = struct{}{}
	}
	return n, nil
}

// flush and flushWriters are called with the tracker lock held.
func (w *stepWriter) flush() error {
	pt := w.tracker
	pt.clearLines()
	_, err := fmt.Fprintf(pt.printer.out, "  %s | %s | %s\n", pt.tasks[w.index].Name, w.step, w.partial[:w.size])
	w.size = 0
	delete(pt.writers, w)
	return err
}

func (pt *ProgressTracker) flushWriters(index int) {
	for w := range pt.writers {
		if index < 0 || w.index == index {
			_ = w.flush()
		}
	}
}

// Start begins the live progress display (call in a goroutine or before work starts)
func (pt *ProgressTracker) Start() {
	pt.mu.Lock()
	interval := 80 * time.Millisecond
	if !pt.isTTY {
		interval = heartbeatInterval
	}
	pt.mu.Unlock()

	go func() {
		defer close(pt.finished)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-pt.done:
				return
			case <-ticker.C:
				pt.render()
			}
		}
	}()
}

// MarkRunning marks a task as running
func (pt *ProgressTracker) MarkRunning(index int) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	if index >= 0 && index < len(pt.tasks) {
		pt.tasks[index].Status = TaskRunning
		pt.tasks[index].StartTime = time.Now()
		if !pt.isTTY {
			pt.printer.Println(pt.statusLine(pt.tasks[index], pt.verbs.active, false))
		}
	}
}

// MarkStep starts a new phase timer immediately before executing a step.
func (pt *ProgressTracker) MarkStep(index int, step string, number, total int) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	if index < 0 || index >= len(pt.tasks) {
		return
	}
	task := pt.tasks[index]
	pt.flushWriters(index)
	task.streamed = false
	task.Phase = step
	if total > 1 {
		task.Phase = fmt.Sprintf("%d/%d %s", number, total, step)
	}
	task.PhaseStartTime = time.Now()
	task.PhaseDuration = 0
	if !pt.isTTY {
		pt.printer.Println(pt.statusLine(task, pt.verbs.active, false))
	}
}

// MarkDone marks a task as completed successfully
func (pt *ProgressTracker) MarkDone(index int) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.flushWriters(index)
	if index >= 0 && index < len(pt.tasks) {
		pt.tasks[index].Status = TaskDone
		pt.tasks[index].Duration = time.Since(pt.tasks[index].StartTime)
		if !pt.tasks[index].PhaseStartTime.IsZero() {
			pt.tasks[index].PhaseDuration = time.Since(pt.tasks[index].PhaseStartTime)
		}
		if !pt.isTTY {
			pt.printer.Println(pt.completionLine(pt.tasks[index]))
		}
	}
}

// MarkFailed marks a task as failed with error and captured output
func (pt *ProgressTracker) MarkFailed(index int, err error, output string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.flushWriters(index)
	if index >= 0 && index < len(pt.tasks) {
		if pt.tasks[index].Status == TaskFailed {
			return
		}
		pt.tasks[index].Status = TaskFailed
		if !pt.tasks[index].StartTime.IsZero() {
			pt.tasks[index].Duration = time.Since(pt.tasks[index].StartTime)
		}
		pt.tasks[index].Error = err
		if !pt.tasks[index].PhaseStartTime.IsZero() {
			pt.tasks[index].PhaseDuration = time.Since(pt.tasks[index].PhaseStartTime)
		}
		pt.clearLines()
		if pt.isTTY {
			pt.printer.Println(pt.buildTaskLine(pt.tasks[index]))
			if err != nil {
				pt.printer.Printf("    %v\n", err)
			}
		} else {
			pt.printer.Println(pt.completionLine(pt.tasks[index]))
		}
		if !pt.tasks[index].streamed {
			if len(output) > tailBufferSize {
				output = output[len(output)-tailBufferSize:]
			}
			pt.printer.ErrorBox(output)
		}
	}
}

// Stop ends the live progress display and prints the final state
func (pt *ProgressTracker) Stop() {
	close(pt.done)
	<-pt.finished
	pt.mu.Lock()
	defer pt.mu.Unlock()
	pt.stoppedAt = time.Now()
	pt.flushWriters(-1)

	if pt.isTTY {
		// Clear the live area and print final static output
		pt.clearLines()
		pt.printFinal()
	}
}

// TotalElapsed returns the total elapsed time
func (pt *ProgressTracker) TotalElapsed() time.Duration {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	if !pt.stoppedAt.IsZero() {
		return pt.stoppedAt.Sub(pt.startTime)
	}
	return time.Since(pt.startTime)
}

// HasFailures returns whether any tasks failed
func (pt *ProgressTracker) HasFailures() bool {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	for _, t := range pt.tasks {
		if t.Status == TaskFailed {
			return true
		}
	}
	return false
}

// render draws the current progress state to the terminal
func (pt *ProgressTracker) render() {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	if !pt.isTTY {
		var running []*TrackedTask
		queued := 0
		for _, task := range pt.tasks {
			switch task.Status {
			case TaskRunning:
				running = append(running, task)
			case TaskPending:
				queued++
			}
		}
		for i, task := range running {
			line := pt.statusLine(task, pt.verbs.still, true)
			// Report the backlog once per update, on the last running job.
			if queued > 0 && i == len(running)-1 {
				line += fmt.Sprintf(" (%s queued)", plural(queued, "job", "jobs"))
			}
			pt.printer.Println(line)
		}
		return
	}

	width, height := 80, 24
	if f, ok := pt.printer.out.(*os.File); ok {
		if w, h, err := term.GetSize(int(f.Fd())); err == nil {
			width, height = w, h
		}
	}
	// A terminal may shrink between frames. Never move above its visible area.
	pt.linesDrawn = min(pt.linesDrawn, max(0, height-1))
	pt.clearLines()
	lines := pt.liveLines(height)
	for i, line := range lines {
		lines[i] = fitProgressLine(line, max(0, width-1))
	}
	output := strings.Join(lines, "\n") + "\n"
	fmt.Fprint(pt.printer.out, output)
	pt.linesDrawn = len(lines)
	pt.spinnerIdx = (pt.spinnerIdx + 1) % len(spinnerFrames)
}

// Reserve the cursor's row and prioritize running work over idle/completed
// tasks, so redrawing never scrolls a display taller than the terminal.
func (pt *ProgressTracker) liveLines(height int) []string {
	rows := max(1, height-1)
	completed, _, total := pt.counts()
	lines := []string{pt.buildProgressBar(completed, total, time.Since(pt.startTime))}
	budget := rows - 1
	hidden := len(pt.tasks) > budget
	if hidden {
		budget = max(0, budget-1)
	}
	var pending, done, running int
	for _, status := range []TaskStatus{TaskRunning, TaskFailed, TaskPending, TaskDone} {
		for _, task := range pt.tasks {
			if task.Status != status {
				continue
			}
			if budget > 0 {
				lines = append(lines, pt.buildTaskLine(task))
				budget--
			} else {
				switch status {
				case TaskRunning:
					running++
				case TaskPending:
					pending++
				default:
					done++
				}
			}
		}
	}
	if hidden {
		summary := fmt.Sprintf("  ... %d running, %d pending, %d completed hidden", running, pending, done)
		if rows == 1 {
			lines[0] = summary
		} else {
			lines = append(lines, summary)
		}
	}
	return lines
}

// Keep live lines from wrapping, which would invalidate the cursor row count.
func fitProgressLine(line string, width int) string {
	plain := ansiColorPattern.ReplaceAllString(line, "")
	if len(plain) <= width {
		return line
	}
	if width <= 3 {
		return strings.Repeat(".", max(0, width))
	}
	// A byte budget is conservative for wide Unicode characters. Trim only at
	// rune boundaries and omit colors on truncated lines to preserve ANSI codes.
	plain = plain[:width-3]
	for !utf8.ValidString(plain) {
		plain = plain[:len(plain)-1]
	}
	return plain + "..."
}

func (pt *ProgressTracker) counts() (completed, running, total int) {
	total = len(pt.tasks)
	for _, t := range pt.tasks {
		switch t.Status {
		case TaskDone, TaskFailed:
			completed++
		case TaskRunning:
			running++
		}
	}
	return
}

// buildProgressBar and buildTaskLine render the animated terminal view only.
// Without a terminal, statusLine and completionLine report each job instead.
func (pt *ProgressTracker) buildProgressBar(completed, total int, elapsed time.Duration) string {
	barWidth := 20
	filled := 0
	if total > 0 {
		filled = (completed * barWidth) / total
	}
	if filled > barWidth {
		filled = barWidth
	}

	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	return fmt.Sprintf("  %s  %d/%d  ⏱ %s",
		pt.printer.cyan(bar),
		completed, total,
		pt.printer.dim(formatDuration(elapsed)))
}

func (pt *ProgressTracker) buildTaskLine(task *TrackedTask) string {
	duration := task.Duration
	phaseDuration := task.PhaseDuration
	if task.Status == TaskRunning {
		duration = time.Since(task.StartTime)
		phaseDuration = time.Since(task.PhaseStartTime)
	}
	detail := fmt.Sprintf("total %s", formatDuration(duration))
	if task.Phase != "" {
		detail = fmt.Sprintf("%s | phase %s | %s", task.Phase, formatDuration(phaseDuration), detail)
	}
	switch task.Status {
	case TaskDone:
		return fmt.Sprintf("  %s %s %s",
			pt.printer.green("✓"),
			task.Name,
			pt.printer.dim(detail))
	case TaskFailed:
		return fmt.Sprintf("  %s %s %s",
			pt.printer.red("✗"),
			task.Name,
			pt.printer.dim(detail))
	case TaskRunning:
		spinner := string(spinnerFrames[pt.spinnerIdx])
		return fmt.Sprintf("  %s %s | %s",
			pt.printer.cyan(spinner),
			task.Name, pt.printer.dim(detail))
	case TaskPending:
		return fmt.Sprintf("  %s %s",
			pt.printer.dim("◦"),
			pt.printer.dim(task.Name))
	}
	return ""
}

// jobLabel pads a job name so every status line starts in the same column.
func (pt *ProgressTracker) jobLabel(task *TrackedTask) string {
	padding := max(0, pt.nameWidth-utf8.RuneCountInString(task.Name))
	return fmt.Sprintf("  %s:%s", task.Name, strings.Repeat(" ", padding))
}

// statusLine reports one job's current state on a single line. It is called
// with mu held so parallel workers cannot interleave output.
func (pt *ProgressTracker) statusLine(task *TrackedTask, verb string, elapsed bool) string {
	var details []string
	if elapsed && !task.StartTime.IsZero() {
		details = append(details, formatElapsed(time.Since(task.StartTime))+" elapsed")
	}
	if task.Phase != "" {
		details = append(details, task.Phase)
	}
	line := fmt.Sprintf("%s %s...", pt.jobLabel(task), verb)
	if len(details) > 0 {
		line += fmt.Sprintf(" [%s]", strings.Join(details, ", "))
	}
	return line
}

// completionLine reports a finished job, including why it failed.
func (pt *ProgressTracker) completionLine(task *TrackedTask) string {
	verb := pt.verbs.done
	if task.Status == TaskFailed {
		verb = pt.verbs.failed
	}
	line := fmt.Sprintf("%s %s after %s", pt.jobLabel(task), verb, formatElapsed(task.Duration))
	if task.Status == TaskFailed && task.Error != nil {
		line += fmt.Sprintf(": %v", task.Error)
	}
	return line
}

// clearLines moves the cursor up and clears the previously drawn lines
func (pt *ProgressTracker) clearLines() {
	if pt.linesDrawn > 0 {
		// Move cursor up and clear each line
		for i := 0; i < pt.linesDrawn; i++ {
			fmt.Fprint(pt.printer.out, "\033[A\033[2K")
		}
		pt.linesDrawn = 0
	}
}

// printFinal prints the final static output (no ANSI cursor movement)
func (pt *ProgressTracker) printFinal() {
	for _, task := range pt.tasks {
		switch task.Status {
		case TaskDone:
			pt.printer.Println(pt.buildTaskLine(task))
		case TaskFailed:
			pt.printer.Println(pt.buildTaskLine(task))
		}
	}
}

// formatElapsed formats a duration in whole seconds for status lines.
func formatElapsed(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}

// formatDuration formats a duration as a human-readable string
func formatDuration(d time.Duration) string {
	if d < time.Second {
		ms := d.Milliseconds()
		if ms == 0 {
			return "0.0s"
		}
		return fmt.Sprintf("0.%ds", ms/100)
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	minutes := int(d.Minutes())
	seconds := d.Seconds() - float64(minutes*60)
	return fmt.Sprintf("%dm%.1fs", minutes, seconds)
}
