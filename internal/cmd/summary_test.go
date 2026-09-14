package cmd

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/irevolve/bear/internal/config"
)

// A summary is a rule, aligned facts, then one labelled section per outcome.
// Sections carry their count and are sorted by name, so two runs of the same
// plan produce identical summaries even though jobs finish in any order.
func TestPrintEnvironmentSummaryAlignsFactsAndSortsSections(t *testing.T) {
	var out bytes.Buffer
	p := NewPrinterWithWriter(&out)
	header := summaryHeader{Environment: "prd", Facts: [][2]string{
		{"Artifacts", "web, api"},
		{"Pinned", shortCommit("abc1234def5678")},
		{"Changes", plural(2, "file", "files")},
	}}
	printEnvironmentSummary(p, header,
		summarySection{Label: "deploy", Color: p.cyan, Entries: []summaryEntry{
			{Name: "web", Path: "svc/web", Reason: "changed"},
			{Name: "api", Path: "svc/api", Reason: "changed"},
		}},
		// An outcome nobody hit is omitted rather than printed as "(0)".
		summarySection{Label: "failed", Color: p.red},
		summarySection{Label: "skip", Color: p.dim, Entries: []summaryEntry{
			{Name: "zeta", Reason: "up to date"},
			{Name: "alpha", Reason: "up to date"},
		}},
	)
	// One artifact is one line. The commit and target are facts about the whole
	// run, reported once in the header, never repeated under each entry.
	want := strings.Join([]string{
		summaryRule,
		"Environment: prd",
		"Artifacts:   web, api",
		"Pinned:      abc1234",
		"Changes:     2 files",
		"",
		"deploy (2):",
		"  - api (svc/api): changed",
		"  - web (svc/web): changed",
		"",
		"skip (2):",
		"  - alpha: up to date",
		"  - zeta: up to date",
		"",
	}, "\n")
	if out.String() != want {
		t.Fatalf("summary =\n%s\nwant\n%s", out.String(), want)
	}
	// The caller keeps its own facts; the header must not be rewritten in place.
	if len(header.Facts) != 3 || header.Facts[0][0] != "Artifacts" {
		t.Fatalf("summary mutated the caller's header: %+v", header.Facts)
	}
}

// Nothing to report means nothing is printed, so an empty run does not end with
// a bare rule.
func TestPrintEnvironmentSummarySkipsEmptyReport(t *testing.T) {
	var out bytes.Buffer
	p := NewPrinterWithWriter(&out)
	printEnvironmentSummary(p, summaryHeader{}, summarySection{Label: "skip", Color: p.dim})
	if out.String() != "" {
		t.Fatalf("empty summary printed %q", out.String())
	}
	// An environment alone is still worth reporting, even with no entries.
	printEnvironmentSummary(p, summaryHeader{Environment: "dev"})
	if out.String() != summaryRule+"\nEnvironment: dev\n" {
		t.Fatalf("environment-only summary = %q", out.String())
	}
}

// Every command ends with one sentence a reader can find by scanning a CI log
// from the bottom up.
func TestPrintResultClosingSentence(t *testing.T) {
	for _, test := range []struct {
		name     string
		headline string
		counts   []string
		elapsed  time.Duration
		want     string
	}{
		{
			name:     "plan",
			headline: "Plan complete",
			counts:   []string{plural(3, "validated", "validated"), plural(1, "to deploy", "to deploy"), plural(1, "skipped", "skipped")},
			want:     "\nPlan complete: 3 validated, 1 to deploy, 1 skipped\n",
		},
		{
			name:     "apply",
			headline: "Apply complete",
			counts:   []string{plural(1, "deployed", "deployed"), plural(1, "skipped", "skipped")},
			elapsed:  76 * time.Second,
			want:     "\nApply complete: 1 deployed, 1 skipped in 1m16s\n",
		},
		{
			name:     "apply failed",
			headline: "Apply failed",
			counts:   []string{plural(0, "deployed", "deployed"), plural(1, "failed", "failed"), plural(1, "skipped", "skipped")},
			elapsed:  time.Millisecond,
			want:     "\nApply failed: 0 deployed, 1 failed, 1 skipped in 0s\n",
		},
		{
			name:     "validation",
			headline: "Validation complete",
			counts:   []string{plural(3, "artifact", "artifacts")},
			elapsed:  2 * time.Second,
			want:     "\nValidation complete: 3 artifacts in 2s\n",
		},
		{
			name:     "no counts",
			headline: "Plan complete",
			want:     "\nPlan complete\n",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out bytes.Buffer
			p := NewPrinterWithWriter(&out)
			printResult(p, p.green, test.headline, test.counts, test.elapsed)
			if out.String() != test.want {
				t.Fatalf("result = %q, want %q", out.String(), test.want)
			}
		})
	}
}

func TestPlural(t *testing.T) {
	for _, test := range []struct {
		n    int
		want string
	}{{0, "0 jobs"}, {1, "1 job"}, {2, "2 jobs"}} {
		if got := plural(test.n, "job", "jobs"); got != test.want {
			t.Errorf("plural(%d) = %q, want %q", test.n, got, test.want)
		}
	}
}

// A summary entry answers three questions and no more: which artifact, where
// it lives and why it is in this section. Everything a run shares — the commit,
// the pin, the target — is a header fact, so an entry is one line even for a
// pinned artifact that carries all of it.
func TestSummaryEntriesCarryOnlyNamePathReasonAndLibTag(t *testing.T) {
	artifact := config.PlanArtifact{
		Name: "api", Path: "svc/api", Reason: "changed", Target: "local",
		Pinned: true, PinCommit: "fedcba9876543210",
	}
	want := summaryEntry{Name: "api", Path: "svc/api", Reason: "changed"}
	if got := deployEntry(artifact); got != want {
		t.Errorf("deployEntry = %+v, want %+v", got, want)
	}
	// Guard the shape itself: a reintroduced Commit/Target field would silently
	// bring back the per-artifact second line.
	if fields := reflect.TypeOf(summaryEntry{}).NumField(); fields != 4 {
		t.Errorf("summaryEntry has %d fields, want 4 (Name, Path, Reason, IsLib)", fields)
	}
	skipped := config.PlanSkipped{Name: "web", Path: "svc/web", Reason: "not enabled"}
	wantSkip := summaryEntry{Name: "web", Path: "svc/web", Reason: "not enabled"}
	if got := skipEntry(skipped); got != wantSkip {
		t.Errorf("skipEntry = %+v, want %+v", got, wantSkip)
	}
	// A library carries its tag through skipEntry, since it never has a
	// deploy entry to be confused with.
	lib := config.PlanSkipped{Name: "shared", Path: "libs/shared", Reason: "new artifact", IsLib: true}
	wantLib := summaryEntry{Name: "shared", Path: "libs/shared", Reason: "new artifact", IsLib: true}
	if got := skipEntry(lib); got != wantLib {
		t.Errorf("skipEntry(library) = %+v, want %+v", got, wantLib)
	}
	entries := planSkipEntries([]config.PlanSkipped{skipped})
	if len(entries) != 1 || entries[0] != wantSkip {
		t.Errorf("planSkipEntries = %+v, want [%+v]", entries, wantSkip)
	}
	if got := planSkipEntries(nil); len(got) != 0 {
		t.Errorf("planSkipEntries(nil) = %+v", got)
	}
	// Rendered, the richest possible artifact still occupies exactly one line.
	var out bytes.Buffer
	p := NewPrinterWithWriter(&out)
	printEnvironmentSummary(p, summaryHeader{Environment: "prd", Facts: [][2]string{{"Pinned", shortCommit(artifact.PinCommit)}}},
		summarySection{Label: "deploy", Color: p.cyan, Entries: []summaryEntry{deployEntry(artifact)}},
	)
	wantOutput := summaryRule + "\nEnvironment: prd\nPinned:      fedcba9\n\ndeploy (1):\n  - api (svc/api): changed\n"
	if out.String() != wantOutput {
		t.Errorf("entry rendering =\n%s\nwant\n%s", out.String(), wantOutput)
	}
	// A library's line is prefixed with a "lib" tag, kept out of the bold
	// artifact name so nested ANSI resets can't clip the styling.
	out.Reset()
	printEnvironmentSummary(p, summaryHeader{Environment: "int"},
		summarySection{Label: "skip", Color: p.dim, Entries: []summaryEntry{wantLib}},
	)
	wantLibOutput := summaryRule + "\nEnvironment: int\n\nskip (1):\n  - lib shared (libs/shared): new artifact\n"
	if out.String() != wantLibOutput {
		t.Errorf("library entry rendering =\n%s\nwant\n%s", out.String(), wantLibOutput)
	}
}

// The source is trimmed once for the header fact, so a reader compares a short
// commit instead of forty characters.
func TestShortCommitTrimsForTheHeader(t *testing.T) {
	for _, test := range []struct{ in, want string }{{"", ""}, {"abc", "abc"}, {"0123456789abcdef", "0123456"}} {
		if got := shortCommit(test.in); got != test.want {
			t.Errorf("shortCommit(%q) = %q, want %q", test.in, got, test.want)
		}
	}
}
