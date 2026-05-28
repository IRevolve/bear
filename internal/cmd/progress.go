package cmd

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

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
	Name      string
	Status    TaskStatus
	StartTime time.Time
	Duration  time.Duration
	Error     error
	Output    string // captured output (shown only on error)
}

// ProgressTracker provides live terminal progress display with spinner,
// progress bar, timer, and real-time task completion updates.
type ProgressTracker struct {
	mu          sync.Mutex
	tasks       []*TrackedTask
	startTime   time.Time
	printer     *Printer
	isTTY       bool
	done        chan struct{}
	linesDrawn  int
	spinnerIdx  int
	title       string
}

var spinnerFrames = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

// NewProgressTracker creates a new progress tracker
func NewProgressTracker(p *Printer, title string, taskNames []string) *ProgressTracker {
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

	return &ProgressTracker{
		tasks:     tasks,
		startTime: time.Now(),
		printer:   p,
		isTTY:     isTTY,
		done:      make(chan struct{}),
		title:     title,
	}
}

// Start begins the live progress display (call in a goroutine or before work starts)
func (pt *ProgressTracker) Start() {
	if !pt.isTTY {
		// Non-TTY: just print header
		pt.printer.Printf("  %s\n", pt.title)
		return
	}

	go func() {
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-pt.done:
				pt.render()
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
	if index < len(pt.tasks) {
		pt.tasks[index].Status = TaskRunning
		pt.tasks[index].StartTime = time.Now()
	}
}

// MarkDone marks a task as completed successfully
func (pt *ProgressTracker) MarkDone(index int) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	if index < len(pt.tasks) {
		pt.tasks[index].Status = TaskDone
		pt.tasks[index].Duration = time.Since(pt.tasks[index].StartTime)
	}
}

// MarkFailed marks a task as failed with error and captured output
func (pt *ProgressTracker) MarkFailed(index int, err error, output string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	if index < len(pt.tasks) {
		pt.tasks[index].Status = TaskFailed
		pt.tasks[index].Duration = time.Since(pt.tasks[index].StartTime)
		pt.tasks[index].Error = err
		pt.tasks[index].Output = output
	}
}

// Stop ends the live progress display and prints the final state
func (pt *ProgressTracker) Stop() {
	close(pt.done)
	// Small delay to let the final render happen
	time.Sleep(100 * time.Millisecond)

	if pt.isTTY {
		// Clear the live area and print final static output
		pt.clearLines()
	}
	pt.printFinal()
}

// TotalElapsed returns the total elapsed time
func (pt *ProgressTracker) TotalElapsed() time.Duration {
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
		return
	}

	// Clear previous render
	pt.clearLines()

	var lines []string

	// Progress bar line
	completed, running, total := pt.counts()
	elapsed := time.Since(pt.startTime)
	barLine := pt.buildProgressBar(completed, total, elapsed)
	lines = append(lines, barLine)
	lines = append(lines, "") // blank line

	// Task lines
	for _, task := range pt.tasks {
		line := pt.buildTaskLine(task)
		if line != "" {
			lines = append(lines, line)
		}
	}

	// Show count of pending if there are many
	_, pending := pt.pendingCount()
	if pending > 3 && running == 0 {
		lines = append(lines, fmt.Sprintf("  %s", pt.printer.dim(fmt.Sprintf("... and %d more pending", pending))))
	}

	output := strings.Join(lines, "\n") + "\n"
	fmt.Fprint(pt.printer.out, output)
	pt.linesDrawn = len(lines)
	pt.spinnerIdx = (pt.spinnerIdx + 1) % len(spinnerFrames)
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

func (pt *ProgressTracker) pendingCount() (running, pending int) {
	for _, t := range pt.tasks {
		switch t.Status {
		case TaskPending:
			pending++
		case TaskRunning:
			running++
		}
	}
	return
}

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
	timeStr := formatDuration(elapsed)

	return fmt.Sprintf("  %s  %d/%d  ⏱ %s",
		pt.printer.cyan(bar),
		completed, total,
		pt.printer.dim(timeStr))
}

func (pt *ProgressTracker) buildTaskLine(task *TrackedTask) string {
	switch task.Status {
	case TaskDone:
		return fmt.Sprintf("  %s %s %s",
			pt.printer.green("✓"),
			task.Name,
			pt.printer.dim(formatDuration(task.Duration)))
	case TaskFailed:
		return fmt.Sprintf("  %s %s %s",
			pt.printer.red("✗"),
			task.Name,
			pt.printer.dim(formatDuration(task.Duration)))
	case TaskRunning:
		spinner := string(spinnerFrames[pt.spinnerIdx])
		return fmt.Sprintf("  %s %s...",
			pt.printer.cyan(spinner),
			task.Name)
	case TaskPending:
		return fmt.Sprintf("  %s %s",
			pt.printer.dim("◦"),
			pt.printer.dim(task.Name))
	}
	return ""
}

// clearLines moves the cursor up and clears the previously drawn lines
func (pt *ProgressTracker) clearLines() {
	if pt.linesDrawn > 0 {
		// Move cursor up and clear each line
		for i := 0; i < pt.linesDrawn; i++ {
			fmt.Fprint(pt.printer.out, "\033[A\033[2K")
		}
	}
}

// printFinal prints the final static output (no ANSI cursor movement)
func (pt *ProgressTracker) printFinal() {
	for _, task := range pt.tasks {
		switch task.Status {
		case TaskDone:
			pt.printer.Printf("  %s %s %s\n",
				pt.printer.green("✓"),
				task.Name,
				pt.printer.dim(formatDuration(task.Duration)))
		case TaskFailed:
			pt.printer.Printf("  %s %s %s\n",
				pt.printer.red("✗"),
				task.Name,
				pt.printer.dim(formatDuration(task.Duration)))
			if task.Output != "" {
				for _, line := range strings.Split(strings.TrimRight(task.Output, "\n"), "\n") {
					pt.printer.Printf("    %s %s\n", pt.printer.red("│"), pt.printer.dim(line))
				}
			}
		}
	}
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
