package cmd

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ProgressTracker provides CI-friendly streaming progress output.
// Results are printed immediately as tasks complete, with a progress bar.
type ProgressTracker struct {
	p       *Printer
	total   int
	done    atomic.Int32
	mu      sync.Mutex
	label   string
	started time.Time
}

// NewProgressTracker creates a new progress tracker.
func NewProgressTracker(p *Printer, total int, label string) *ProgressTracker {
	return &ProgressTracker{
		p:       p,
		total:   total,
		label:   label,
		started: time.Now(),
	}
}

// Complete marks one task as done and prints its result immediately.
// The printFn is called under a mutex to prevent interleaved output.
func (pt *ProgressTracker) Complete(printFn func(p *Printer)) {
	pt.mu.Lock()
	defer pt.mu.Unlock()

	pt.done.Add(1)
	printFn(pt.p)
	pt.printProgress()
}

// printProgress prints a CI-friendly progress bar line.
func (pt *ProgressTracker) printProgress() {
	done := int(pt.done.Load())
	remaining := pt.total - done

	if remaining == 0 {
		return
	}

	barWidth := 20
	filled := (done * barWidth) / pt.total
	empty := barWidth - filled

	bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)
	pct := (done * 100) / pt.total
	elapsed := time.Since(pt.started).Round(time.Second)

	pt.p.Printf("  %s %s %s %s\n",
		pt.p.dim(fmt.Sprintf("[%d/%d]", done, pt.total)),
		pt.p.dim(bar),
		pt.p.dim(fmt.Sprintf("%d%%", pct)),
		pt.p.dim(fmt.Sprintf("(%s elapsed, %d remaining)", elapsed, remaining)),
	)
}
