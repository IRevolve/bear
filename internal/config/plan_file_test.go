package config

import (
	"os"
	"reflect"
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
			plan.Validated = 2
			plan.ToDeploy = 1
			plan.TotalSkips = 1
			plan.Artifacts = []PlanArtifact{{Name: "allowed", Path: "allowed", Target: "local", Environments: []string{"dev", "int", "prd"}, Vars: map[string]string{"ENVIRONMENT": environment}, Action: "deploy", Steps: []Step{{Name: "deploy", Run: "true"}}, Pinned: true, PinCommit: plan.Commit}}
			for _, name := range []string{"allowed", "disabled"} {
				plan.Validations = append(plan.Validations, PlanValidation{Name: name, Path: name, Vars: map[string]string{"ENVIRONMENT": environment}, Steps: []Step{{Name: "validate", Run: "true"}}})
			}
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
		})
	}
}

// skippedBlock returns only the "skipped:" section of a serialized plan so
// assertions cannot accidentally match an artifact or validation field.
func skippedBlock(t *testing.T, data []byte) string {
	t.Helper()
	start := strings.Index(string(data), "\nskipped:\n")
	if start < 0 {
		t.Fatalf("plan has no skipped section: %s", data)
	}
	block := string(data)[start+len("\nskipped:\n"):]
	if end := strings.Index(block, "\nvalidated:"); end >= 0 {
		block = block[:end]
	}
	return block
}
