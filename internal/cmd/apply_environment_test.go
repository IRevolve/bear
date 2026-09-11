package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/irevolve/bear/internal/config"
)

func TestApplyPreflightsEntireEnvironmentSnapshot(t *testing.T) {
	for _, scenario := range []string{"unversioned plan", "old plan", "missing environment", "malformed environment", "missing allowlist", "empty allowlist", "nonmatching allowlist", "missing vars", "missing ENVIRONMENT", "mismatched ENVIRONMENT"} {
		for _, existingLock := range []bool{false, true} {
			t.Run(scenario+map[bool]string{false: "/no lock", true: "/existing lock"}[existingLock], func(t *testing.T) {
				root := t.TempDir()
				writeEnvironmentFixture(t, filepath.Join(root, ".gitignore"), ".bear/\nbear.lock.yml\nbin/\nsubprocess\ndeployed\n")
				environmentGit(t, root, "init")
				commitEnvironmentFixture(t, root)
				commit, fingerprint, dirty, err := sourceState(context.Background(), root)
				if err != nil || dirty {
					t.Fatalf("fixture source state: dirty=%v, %v", dirty, err)
				}
				// Catch even the HEAD-check subprocess, not just deployment steps.
				git := filepath.Join(root, "bin", "git")
				writeEnvironmentFixture(t, git, "#!/bin/sh\ntouch \"$SUBPROCESS_MARKER\"\n")
				if err := os.Chmod(git, 0755); err != nil {
					t.Fatal(err)
				}
				t.Setenv("PATH", filepath.Dir(git)+string(os.PathListSeparator)+os.Getenv("PATH"))
				t.Setenv("SUBPROCESS_MARKER", filepath.Join(root, "subprocess"))
				t.Setenv("ENVIRONMENT", "int")
				plan := config.NewPlanFile(commit)
				plan.SourceFingerprint = fingerprint
				plan.Environment = "int"
				plan.ToDeploy = 2
				for _, name := range []string{"valid-first", "invalid-last"} {
					plan.Artifacts = append(plan.Artifacts, config.PlanArtifact{
						Name: name, Path: ".", Action: "deploy", Target: "local",
						Environments: []string{"int"}, Vars: map[string]string{"ENVIRONMENT": "int"},
						Steps: []config.Step{{Name: "deploy", Run: "touch deployed"}},
					})
				}
				invalid := &plan.Artifacts[1]
				wantError := "saved deployment permission"
				switch scenario {
				case "unversioned plan":
					// A plan written by a binary that predates the schema version
					// field (or any hand-edited file missing "version") must be
					// rejected before anything else is even considered, so this
					// also breaks the environment to prove ordering.
					plan.Version = 0
					plan.Environment = ""
					for i := range plan.Artifacts {
						plan.Artifacts[i].Environments = nil
					}
					wantError = fmt.Sprintf("schema version 0 (want %d)", config.CurrentPlanFileVersion)
				case "old plan":
					plan.Environment = ""
					for i := range plan.Artifacts {
						plan.Artifacts[i].Environments = nil
					}
					wantError = "invalid environment name"
				case "missing environment":
					plan.Environment = ""
					wantError = "invalid environment name"
				case "malformed environment":
					// Only the syntax of a saved environment is checked. An
					// undeclared but well-formed name is covered separately.
					plan.Environment = "Production"
					wantError = "invalid environment name"
				case "missing allowlist":
					invalid.Environments = nil
				case "empty allowlist":
					invalid.Environments = []string{}
				case "nonmatching allowlist":
					invalid.Environments = []string{"dev", "prd"}
				case "missing vars":
					invalid.Vars = nil
					wantError = "ENVIRONMENT does not match"
				case "missing ENVIRONMENT":
					invalid.Vars = map[string]string{"NAME": invalid.Name}
					wantError = "ENVIRONMENT does not match"
				case "mismatched ENVIRONMENT":
					invalid.Vars["ENVIRONMENT"] = "prd"
					wantError = "ENVIRONMENT does not match"
				}
				if err := config.WritePlan(root, plan); err != nil {
					t.Fatal(err)
				}
				beforePlan, err := os.ReadFile(config.PlanFilePath(root))
				if err != nil {
					t.Fatal(err)
				}
				lockPath := filepath.Join(root, "bear.lock.yml")
				originalLock := []byte("environments:\n  int:\n    existing:\n      commit: original\n      pinned: true\n")
				if existingLock {
					writeEnvironmentFixture(t, lockPath, string(originalLock))
				}
				// No current config/artifact files: apply must use only the saved evidence.
				output, err := captureEnvironmentOutput(t, func() error {
					return ApplyWithOptions(filepath.Join(root, "bear.config.yml"), Options{Environment: "int", NoCommit: true, Concurrency: 1})
				})
				if err == nil || !strings.Contains(err.Error(), wantError) || !strings.Contains(err.Error(), "run 'bear plan <environment>' again") {
					t.Fatalf("expected actionable %q error, got %v", wantError, err)
				}
				// A refused plan is reported by the error alone. Apply prints no
				// header, no environment and no summary for work it will not do.
				if output != "" {
					t.Errorf("rejected plan printed %q", output)
				}
				for _, marker := range []string{"subprocess", "deployed"} {
					if _, err := os.Stat(filepath.Join(root, marker)); !os.IsNotExist(err) {
						t.Errorf("preflight ran %s: %v", marker, err)
					}
				}
				afterLock, err := os.ReadFile(lockPath)
				if existingLock {
					if err != nil || !bytes.Equal(afterLock, originalLock) {
						t.Errorf("lock modified: %s, %v", afterLock, err)
					}
				} else if !os.IsNotExist(err) {
					t.Errorf("lock created: %v", err)
				}
				afterPlan, err := os.ReadFile(config.PlanFilePath(root))
				if err != nil || !bytes.Equal(afterPlan, beforePlan) {
					t.Errorf("rejected plan modified: %v", err)
				}
			})
		}
	}
}

// An approved plan is a snapshot. Apply never rereads bear.config.yml to decide
// where to deploy, so retiring or renaming an environment does not invalidate an
// approved plan. Only the saved allowlist and the saved ENVIRONMENT still gate it.
func TestApplyIgnoresCurrentEnvironmentDeclaration(t *testing.T) {
	for _, tt := range []struct {
		name      string
		allowlist []string
		wantError string
	}{
		{name: "undeclared environment still applies", allowlist: []string{"preprd"}},
		{name: "saved allowlist still gates", allowlist: []string{"prd"}, wantError: `artifact "allowed" has no saved deployment permission for environment preprd`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			// The project declares dev, int and prd; the plan says preprd.
			root, path := environmentFixture(t)
			commit, fingerprint, dirty, err := sourceState(context.Background(), root)
			if err != nil || dirty {
				t.Fatalf("fixture source state: dirty=%v, %v", dirty, err)
			}
			plan := config.NewPlanFile(commit)
			plan.SourceFingerprint = fingerprint
			plan.Environment = "preprd"
			plan.ToDeploy = 1
			plan.Artifacts = []config.PlanArtifact{{
				Name: "allowed", Path: "allowed", Language: "test", Target: "local",
				Environments: tt.allowlist, Action: "deploy",
				Vars:  map[string]string{"ENVIRONMENT": "preprd"},
				Steps: []config.Step{{Name: "deploy", Run: `touch "$BEAR_TEST_OUTPUT/allowed/deployed"`}},
			}}
			if err := config.WritePlan(root, plan); err != nil {
				t.Fatal(err)
			}
			deployed := filepath.Join(root, "allowed", "deployed")
			output, err := captureEnvironmentOutput(t, func() error {
				return ApplyWithOptions(path, Options{NoCommit: true, Concurrency: 1})
			})
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("expected %q, got %v\n%s", tt.wantError, err, output)
				}
				if _, err := os.Stat(deployed); !os.IsNotExist(err) {
					t.Fatalf("rejected plan deployed: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("%v\n%s", err, output)
			}
			if !strings.Contains(output, "Deploying 1 artifact to preprd") {
				t.Errorf("apply did not deploy to the approved environment: %s", output)
			}
			if _, err := os.Stat(deployed); err != nil {
				t.Fatalf("approved deployment did not run: %v", err)
			}
			// History is keyed by the approved environment, undeclared or not.
			lock, err := config.LoadLock(filepath.Join(root, "bear.lock.yml"))
			if err != nil {
				t.Fatal(err)
			}
			entry, ok := lock.GetArtifact("preprd", "allowed")
			if !ok || entry.Commit != commit {
				t.Fatalf("history not keyed under preprd: %+v", lock.Environments)
			}
			if len(lock.Environments) != 1 {
				t.Errorf("apply wrote history for another environment: %+v", lock.Environments)
			}
			if config.PlanExists(root) {
				t.Error("apply did not consume plan")
			}
		})
	}
}

// A project is free to name its own environments. This drives a set that shares
// nothing with the old dev/int/prd default all the way through plan and apply.
func TestCustomEnvironmentSetPlanApply(t *testing.T) {
	root, path := environmentFixtureWith(t, []string{"preprd", "prd"})
	// disabled allows prd only, so planning preprd must skip it.
	writeEnvironmentFixture(t, filepath.Join(root, "disabled", "bear.artifact.yml"), "name: disabled\ntarget: local\nenvironments: [prd]\n")
	commitEnvironmentFixture(t, root)
	opts := Options{Environment: "preprd", NoCommit: true, Concurrency: 1}
	output, err := captureEnvironmentOutput(t, func() error { return PlanWithOptions(path, opts) })
	if err != nil {
		t.Fatalf("%v\n%s", err, output)
	}
	for _, text := range []string{
		"Environment: preprd",
		"deploy (1):",
		"- allowed (allowed): ",
		"skip (1):",
		"- disabled (disabled): deployment not enabled for environment preprd",
	} {
		if !strings.Contains(output, text) {
			t.Errorf("plan output missing %q: %s", text, output)
		}
	}
	plan, err := config.ReadPlan(root)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Environment != "preprd" || plan.ToDeploy != 1 || plan.Changed != 2 || plan.TotalSkips != 1 {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if !reflect.DeepEqual(plan.Artifacts[0].Environments, []string{"preprd", "prd"}) || plan.Artifacts[0].Vars["ENVIRONMENT"] != "preprd" {
		t.Fatalf("unexpected snapshot: %+v", plan.Artifacts[0])
	}
	output, err = captureEnvironmentOutput(t, func() error { return ApplyWithOptions(path, opts) })
	if err != nil {
		t.Fatalf("%v\n%s", err, output)
	}
	if !strings.Contains(output, "Deploying 1 artifact to preprd") {
		t.Errorf("apply output missing the environment: %s", output)
	}
	if _, err := os.Stat(filepath.Join(root, "allowed", "deployed")); err != nil {
		t.Fatalf("allowed deployment did not run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "disabled", "deployed")); !os.IsNotExist(err) {
		t.Fatalf("skipped artifact deployed: %v", err)
	}
	// The lock file keys history under the configured name, nothing else.
	lock, err := config.LoadLock(filepath.Join(root, "bear.lock.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(lock.Environments) != 1 || len(lock.Environments["preprd"]) != 1 {
		t.Fatalf("history not keyed under preprd only: %+v", lock.Environments)
	}
	if entry := lock.Environments["preprd"]["allowed"]; entry.Commit != plan.Commit || entry.Target != "local" {
		t.Fatalf("preprd history: %+v", entry)
	}
	// A second plan against an unchanged source has nothing left to deploy.
	output, err = captureEnvironmentOutput(t, func() error { return PlanWithOptions(path, opts) })
	if err != nil {
		t.Fatalf("%v\n%s", err, output)
	}
	if !strings.Contains(output, "- allowed (allowed): no changes detected") {
		t.Errorf("preprd history was not used as the baseline: %s", output)
	}
	// prd has no history yet, so the same artifact is new there.
	output, err = captureEnvironmentOutput(t, func() error {
		return PlanWithOptions(path, Options{Environment: "prd", NoCommit: true, Concurrency: 1})
	})
	if err != nil {
		t.Fatalf("%v\n%s", err, output)
	}
	prd, err := config.ReadPlan(root)
	if err != nil {
		t.Fatal(err)
	}
	if prd.Environment != "prd" || prd.ToDeploy != 2 {
		t.Fatalf("preprd history leaked into prd: %+v", prd)
	}
}
