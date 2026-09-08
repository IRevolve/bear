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
			plan.Skipped = []PlanSkipped{{Name: "disabled", Reason: "deployment not enabled for environment int"}}
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
		})
	}
}
