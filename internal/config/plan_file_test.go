package config

import (
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestPlanEnvironmentRoundTrip(t *testing.T) {
	for _, environment := range []string{"dev", "int", "prd"} {
		t.Run("environment="+environment, func(t *testing.T) {
			root := t.TempDir()
			plan := NewPlanFile(strings.Repeat("a", 40))
			plan.Environment = environment
			plan.SourceFingerprint = strings.Repeat("b", 64)
			plan.Pinned = true
			plan.Changed = 2
			plan.ToDeploy = 1
			plan.TotalSkips = 1
			plan.Artifacts = []PlanArtifact{{Name: "allowed", Path: "allowed", Target: "local", Environments: []string{"dev", "int", "prd"}, Vars: map[string]string{"ENVIRONMENT": environment}, Action: "deploy", BuildSteps: []Step{{Name: "build", Run: "true"}}, Steps: []Step{{Name: "deploy", Run: "true"}}, Pinned: true, PinCommit: plan.Commit}}
			plan.Skipped = []PlanSkipped{{Name: "disabled", Path: "disabled", Reason: "deployment not enabled for environment int"}}
			if err := WritePlan(root, plan); err != nil {
				t.Fatal(err)
			}
			got, err := ReadPlan(root)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, plan) {
				t.Errorf("round trip changed plan: got %+v, want %+v", got, plan)
			}
			data, err := os.ReadFile(PlanFilePath(root))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains("\n"+string(data), "\nenvironment: "+environment+"\n") {
				t.Errorf("unexpected environment serialization: %s", data)
			}
			// NewPlanFile always stamps the current schema version, and it is
			// always written out (never omitted), even when it happens to be 0.
			if plan.Version != CurrentPlanFileVersion {
				t.Errorf("NewPlanFile did not set Version: got %d, want %d", plan.Version, CurrentPlanFileVersion)
			}
			if !strings.Contains(string(data), "version: "+strconv.Itoa(plan.Version)+"\n") {
				t.Errorf("missing version serialization: %s", data)
			}
			// build_steps is serialized alongside steps for a deploying artifact.
			if !strings.Contains(string(data), "build_steps:") {
				t.Errorf("missing build_steps serialization: %s", data)
			}
			if !strings.Contains(string(data), "\nchanged: 2\n") {
				t.Errorf("missing changed count serialization: %s", data)
			}
			// A skip records exactly what its summary line prints: the artifact,
			// where it lives, and why it was not deployed.
			block := skippedBlock(t, data)
			for _, field := range []string{"name: disabled", "path: disabled", "reason: deployment not enabled for environment int"} {
				if !strings.Contains(block, field) {
					t.Errorf("skip is missing %q: %s", field, block)
				}
			}
			// The commit and target describe the run, not the skip, and are
			// reported once in the plan header instead.
			for _, field := range []string{"target:", "commit:"} {
				if strings.Contains(block, field) {
					t.Errorf("skip still records %q: %s", field, block)
				}
			}
			// Optional skip details stay out of the file when unset.
			plan.Skipped = []PlanSkipped{{Name: "disabled", Reason: "no changes detected"}}
			if err := WritePlan(root, plan); err != nil {
				t.Fatal(err)
			}
			bare, err := os.ReadFile(PlanFilePath(root))
			if err != nil {
				t.Fatal(err)
			}
			block = skippedBlock(t, bare)
			if strings.Contains(block, "path:") {
				t.Errorf("empty skip path serialized: %s", block)
			}
			if got, err := ReadPlan(root); err != nil || !reflect.DeepEqual(got, plan) {
				t.Errorf("bare skip round trip: got %+v, want %+v (%v)", got, plan, err)
			}
			// build_steps is omitted entirely when an artifact's language has none.
			plan.Artifacts[0].BuildSteps = nil
			if err := WritePlan(root, plan); err != nil {
				t.Fatal(err)
			}
			noBuild, err := os.ReadFile(PlanFilePath(root))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(noBuild), "build_steps:") {
				t.Errorf("empty build_steps serialized: %s", noBuild)
			}
			if got, err := ReadPlan(root); err != nil || !reflect.DeepEqual(got, plan) {
				t.Errorf("no-build-steps round trip: got %+v, want %+v (%v)", got, plan, err)
			}
		})
	}
}

// TestReadPlanMissingVersionIsZero documents that a plan file written by a
// binary that predates the version field (or any hand-edited/malformed file
// missing the "version" key) reads back as Version 0, not the current
// schema version. Callers (apply's preflight) rely on this to distinguish
// "no version marker" from a real, current version.
func TestReadPlanMissingVersionIsZero(t *testing.T) {
	root := t.TempDir()
	plan := NewPlanFile(strings.Repeat("a", 40))
	plan.Environment = "dev"
	plan.SourceFingerprint = strings.Repeat("b", 64)
	if err := WritePlan(root, plan); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(PlanFilePath(root))
	if err != nil {
		t.Fatal(err)
	}
	// Simulate an old plan file by stripping the version line entirely.
	stripped := strings.Replace(string(data), "version: "+strconv.Itoa(plan.Version)+"\n", "", 1)
	if stripped == string(data) {
		t.Fatalf("failed to strip version line from: %s", data)
	}
	if err := os.WriteFile(PlanFilePath(root), []byte(stripped), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPlan(root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 0 {
		t.Errorf("plan file missing version key: got Version %d, want 0", got.Version)
	}
}

// skippedBlock returns only the "skipped:" section of a serialized plan so
// assertions cannot accidentally match an artifact field.
func skippedBlock(t *testing.T, data []byte) string {
	t.Helper()
	start := strings.Index(string(data), "\nskipped:\n")
	if start < 0 {
		t.Fatalf("plan has no skipped section: %s", data)
	}
	block := string(data)[start+len("\nskipped:\n"):]
	if end := strings.Index(block, "\nchanged:"); end >= 0 {
		block = block[:end]
	}
	return block
}
