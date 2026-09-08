package internal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/irevolve/bear/internal/config"
)

func TestCreatePlanEnvironmentPolicy(t *testing.T) {
	for _, tt := range []struct {
		name      string
		opts      PlanOptions
		policy    string
		pinned    bool
		wantError string
		deploys   int
		validates int
		skips     int
	}{
		{name: "no policy", opts: PlanOptions{Environment: "int"}, validates: 1, skips: 1},
		{name: "empty policy", policy: "[]", opts: PlanOptions{Environment: "int"}, validates: 1, skips: 1},
		{name: "missing environment without policy", wantError: "invalid environment"},
		{name: "missing environment with empty policy", policy: "[]", wantError: "invalid environment"},
		{name: "dev allowed", policy: "[dev]", opts: PlanOptions{Environment: "dev"}, deploys: 1, validates: 1},
		{name: "int allowed", policy: "[int, prd]", opts: PlanOptions{Environment: "int"}, deploys: 1, validates: 1},
		{name: "prd allowed", policy: "[int, prd]", opts: PlanOptions{Environment: "prd"}, deploys: 1, validates: 1},
		{name: "int not enabled", policy: "[dev]", opts: PlanOptions{Environment: "int"}, validates: 1, skips: 1},
		{name: "prd not enabled", policy: "[dev]", opts: PlanOptions{Environment: "prd"}, validates: 1, skips: 1},
		{name: "missing environment", policy: "[int]", wantError: "invalid environment"},
		{name: "missing environment even pinned", policy: "[int]", pinned: true, wantError: "invalid environment"},
		{name: "invalid environment without policy", opts: PlanOptions{Environment: "production"}, wantError: "invalid environment"},
		{name: "invalid policy", policy: "[production]", opts: PlanOptions{Environment: "int"}, wantError: "environments"},
		{name: "unselected policy", policy: "[int]", opts: PlanOptions{Environment: "int", Artifacts: []string{"other"}}},
		{name: "empty selection requires environment", opts: PlanOptions{Artifacts: []string{"other"}}, wantError: "invalid environment"},
		{name: "pin disabled", policy: "[dev]", opts: PlanOptions{Environment: "int", PinCommit: "abc1234"}, validates: 1, skips: 1},
		{name: "pin allowed", policy: "[dev]", opts: PlanOptions{Environment: "dev", PinCommit: "abc1234"}, validates: 1, deploys: 1},
		{name: "pin requires environment", policy: "[int]", opts: PlanOptions{PinCommit: "abc1234"}, wantError: "invalid environment"},
		{name: "force disabled", policy: "[dev]", pinned: true, opts: PlanOptions{Environment: "int", Force: true}, validates: 1, skips: 1},
		{name: "force allowed", policy: "[dev]", pinned: true, opts: PlanOptions{Environment: "dev", Force: true}, validates: 1, deploys: 1},
		{name: "force pin disabled", policy: "[dev]", pinned: true, opts: PlanOptions{Environment: "int", Force: true, PinCommit: "abc1234"}, validates: 1, skips: 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := detectorRepo(t)
			content := "name: api\ntarget: local\n"
			if tt.policy != "" {
				content += "environments: " + tt.policy + "\n"
			}
			if err := os.WriteFile(filepath.Join(root, "bear.artifact.yml"), []byte(content), 0644); err != nil {
				t.Fatal(err)
			}
			commit := detectorCommit(t, root)
			if tt.opts.PinCommit != "" {
				tt.opts.PinCommit = commit
			}
			if tt.pinned {
				lock := &config.LockFile{}
				lock.UpdateArtifactPinned(tt.opts.Environment, "api", commit, "local", "")
				if err := lock.Save(filepath.Join(root, "bear.lock.yml")); err != nil {
					t.Fatal(err)
				}
			}
			cfg := &config.Config{Targets: map[string]config.Target{"local": {Steps: []config.Step{{Name: "deploy", Run: "true"}}}}}
			plan, err := CreatePlanWithOptions(root, cfg, tt.opts)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("expected %q error, got %v", tt.wantError, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if plan.Environment != tt.opts.Environment || plan.ToDeploy != tt.deploys || plan.ToValidate != tt.validates || plan.ToSkip != tt.skips {
				t.Fatalf("unexpected plan: %+v", plan)
			}
			for _, action := range plan.Actions {
				if action.Action == ActionSkip && tt.skips > 0 {
					wantReason := "deployment not enabled for environment " + tt.opts.Environment
					if tt.policy == "" || tt.policy == "[]" {
						wantReason = "no deployment environments configured"
					}
					if action.Reason != wantReason || len(action.Steps) != 0 {
						t.Errorf("disabled action retained steps or lost reason: %+v", action)
					}
				}
			}
		})
	}
}

func TestEnvironmentGatePreservesTransitiveDependencies(t *testing.T) {
	for _, policy := range []struct {
		name         string
		environments []string
	}{
		{name: "absent"},
		{name: "empty", environments: []string{}},
		{name: "nonmatching", environments: []string{"dev"}},
	} {
		t.Run(policy.name, func(t *testing.T) {
			for _, library := range []bool{false, true} {
				t.Run(map[bool]string{false: "disabled source", true: "library source"}[library], func(t *testing.T) {
					cfg := &config.Config{
						Languages: map[string]config.Language{"test": {Steps: []config.Step{{Name: "validate", Run: "true"}}}},
						Targets:   map[string]config.Target{"local": {Steps: []config.Step{{Name: "deploy", Run: "true"}}}},
					}
					root := detectorRepo(t)
					policyYAML := ""
					if policy.environments != nil {
						policyYAML = "environments: [" + strings.Join(policy.environments, ", ") + "]\n"
					}
					sourceFile := "source/bear.artifact.yml"
					sourceContent := "name: source\ntarget: local\n" + policyYAML
					if library {
						sourceFile = "source/bear.lib.yml"
						sourceContent = "name: source\n"
					}
					detectorWrite(t, root, sourceFile, sourceContent)
					detectorWrite(t, root, "middle/bear.artifact.yml", "name: middle\ntarget: local\ndepends: [source]\n"+policyYAML)
					detectorWrite(t, root, "leaf/bear.artifact.yml", "name: leaf\ntarget: local\ndepends: [middle]\nenvironments: [int]\n")
					baseline := detectorCommit(t, root)
					lock := &config.LockFile{}
					for _, name := range []string{"source", "middle", "leaf"} {
						lock.UpdateArtifact("int", name, baseline, "local", "")
					}
					if err := lock.Save(filepath.Join(root, "bear.lock.yml")); err != nil {
						t.Fatal(err)
					}
					detectorWrite(t, root, "source/code.txt", "changed\n")
					detectorCommit(t, root)
					plan, err := CreatePlanWithOptions(root, cfg, PlanOptions{Environment: "int"})
					if err != nil {
						t.Fatal(err)
					}
					wantSkips := 2
					if library {
						wantSkips = 1
					}
					if plan.ToValidate != 3 || plan.ToDeploy != 1 || plan.ToSkip != wantSkips {
						t.Fatalf("dependency counts: %+v", plan)
					}
					for _, action := range plan.Actions {
						if action.Action == ActionDeploy && action.Artifact.Artifact.Name != "leaf" {
							t.Errorf("disabled dependency deployed: %+v", action)
						}
						if action.Action == ActionSkip {
							reason := "deployment not enabled for environment int"
							if len(policy.environments) == 0 {
								reason = "no deployment environments configured"
							}
							if action.Reason != reason || len(action.Steps) != 0 {
								t.Errorf("invalid gated action: %+v", action)
							}
						}
					}
					plan.applyEnvironment("int")
					if plan.ToValidate != 3 || plan.ToDeploy != 1 || plan.ToSkip != wantSkips {
						t.Fatalf("gate is not idempotent: %+v", plan)
					}
				})
			}
		})
	}
}
