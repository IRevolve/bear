package cmd

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/irevolve/bear/internal/config"
)

// summaryRule separates the summary from the preceding progress output.
var summaryRule = strings.Repeat("─", 40)

// summaryEntry describes one artifact in a final plan/apply summary.
type summaryEntry struct {
	Name   string
	Path   string
	Reason string
}

// summarySection groups entries under one label, such as "deploy" or "skip".
type summarySection struct {
	Label   string
	Color   func(string) string
	Entries []summaryEntry
}

// shortCommit trims a commit for display without inventing a value.
func shortCommit(commit string) string {
	if commit == "" {
		return ""
	}
	return commit[:min(7, len(commit))]
}

// deployEntry describes a planned or completed deployment. The commit and
// target are reported once in the header, not repeated per artifact.
func deployEntry(artifact config.PlanArtifact) summaryEntry {
	return summaryEntry{
		Name:   artifact.Name,
		Path:   artifact.Path,
		Reason: artifact.Reason,
	}
}

// skipEntry describes an artifact that will not be deployed.
func skipEntry(skipped config.PlanSkipped) summaryEntry {
	return summaryEntry{
		Name:   skipped.Name,
		Path:   skipped.Path,
		Reason: skipped.Reason,
	}
}

// planSkipEntries converts every saved skip into a summary entry.
func planSkipEntries(skipped []config.PlanSkipped) []summaryEntry {
	entries := make([]summaryEntry, 0, len(skipped))
	for _, s := range skipped {
		entries = append(entries, skipEntry(s))
	}
	return entries
}

// summaryHeader labels a summary with the environment and any run-specific
// facts, such as an artifact filter or a pinned commit.
type summaryHeader struct {
	Environment string
	Facts       [][2]string
}

// printEnvironmentSummary prints the header followed by one labelled list per
// section. Empty sections are omitted so the summary stays scannable.
func printEnvironmentSummary(p *Printer, header summaryHeader, sections ...summarySection) {
	entries := 0
	for _, section := range sections {
		entries += len(section.Entries)
	}
	if entries == 0 && header.Environment == "" {
		return
	}
	facts := header.Facts
	if header.Environment != "" {
		facts = append([][2]string{{"Environment", p.bold(header.Environment)}}, facts...)
	}
	width := 0
	for _, fact := range facts {
		width = max(width, len(fact[0]))
	}
	p.Println(p.dim(summaryRule))
	for _, fact := range facts {
		p.Printf("%s %s\n", fact[0]+":"+strings.Repeat(" ", width-len(fact[0])), fact[1])
	}
	for _, section := range sections {
		if len(section.Entries) == 0 {
			continue
		}
		color := section.Color
		if color == nil {
			color = func(text string) string { return text }
		}
		// Sort by name so repeated runs produce comparable summaries even
		// though jobs finish in nondeterministic order.
		entries := slices.Clone(section.Entries)
		slices.SortFunc(entries, func(a, b summaryEntry) int { return strings.Compare(a.Name, b.Name) })
		p.Blank()
		p.Printf("%s (%d):\n", color(section.Label), len(entries))
		for _, entry := range entries {
			name := entry.Name
			if entry.Path != "" {
				name = fmt.Sprintf("%s (%s)", entry.Name, entry.Path)
			}
			p.Printf("  - %s: %s\n", p.bold(name), entry.Reason)
		}
	}
}

// plural renders a count with the matching noun.
func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// printResult closes a command with a single sentence, the way a reader scans
// a CI log from the bottom up.
func printResult(p *Printer, color func(string) string, headline string, counts []string, elapsed time.Duration) {
	p.Blank()
	line := color(headline)
	if len(counts) > 0 {
		line += ": " + strings.Join(counts, ", ")
	}
	if elapsed > 0 {
		line += p.dim(fmt.Sprintf(" in %s", formatElapsed(elapsed)))
	}
	p.Println(line)
}
