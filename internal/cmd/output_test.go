package cmd

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

// A phase is a plain bold heading. Banners competed with the Bear header and
// with each other, so a CI log now reads as one column of text.
func TestPhaseHeaderIsAPlainHeading(t *testing.T) {
	var out bytes.Buffer
	NewPrinterWithWriter(&out).PhaseHeader("Deploying 2 artifacts to prd")
	if out.String() != "\nDeploying 2 artifacts to prd\n\n" {
		t.Fatalf("phase header = %q", out.String())
	}
	for _, decoration := range []string{"━", "─", "=", "#"} {
		if strings.Contains(out.String(), decoration) {
			t.Errorf("phase header still decorated with %q: %q", decoration, out.String())
		}
	}
}

// Both plan and apply open with the same branding, so the two halves of one
// workflow look like one tool.
func TestBearHeaderBrandsEachCommand(t *testing.T) {
	for _, test := range []struct{ command, want string }{
		{"Plan", "\nBear Plan\n─────────\n"},
		{"Apply", "\nBear Apply\n──────────\n"},
		{"Doctor", "\nBear Doctor\n───────────\n"},
	} {
		var out bytes.Buffer
		NewPrinterWithWriter(&out).BearHeader(test.command)
		if out.String() != test.want {
			t.Errorf("BearHeader(%q) = %q, want %q", test.command, out.String(), test.want)
		}
	}
}

// A hint closes a command at the same indent as the result sentence, and
// captured failure output sits one step in under the job line that reported it.
func TestHintAndErrorBoxIndentation(t *testing.T) {
	var out bytes.Buffer
	p := NewPrinterWithWriter(&out)
	p.Hint("Run 'bear apply' to execute this plan.")
	if out.String() != "\nRun 'bear apply' to execute this plan.\n" {
		t.Fatalf("hint = %q", out.String())
	}
	out.Reset()
	p.ErrorBox("first\nsecond\n")
	if out.String() != "    first\n    second\n" {
		t.Fatalf("error box = %q", out.String())
	}
	out.Reset()
	p.ErrorBox("")
	if out.String() != "" {
		t.Fatalf("empty error box printed %q", out.String())
	}
}

// The bespoke summary printers were replaced by one shared summary. Keep them
// gone so command output cannot drift back into per-command formats.
func TestPrinterHasNoBespokeSummaryHelpers(t *testing.T) {
	printer := reflect.TypeOf(&Printer{})
	for _, name := range []string{"Summary", "SummaryValidated", "SummaryDeploy", "SummaryDeployed", "SummaryFailed", "SummarySkipped"} {
		if _, ok := printer.MethodByName(name); ok {
			t.Errorf("Printer.%s was reintroduced; use printEnvironmentSummary instead", name)
		}
	}
}
