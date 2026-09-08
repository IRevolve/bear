package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/irevolve/bear/internal/config"
)

func TestApplyPreflightsEntireEnvironmentSnapshot(t *testing.T) {
	for _, scenario := range []string{"old plan", "missing environment", "invalid environment", "missing allowlist", "empty allowlist", "nonmatching allowlist", "missing vars", "missing ENVIRONMENT", "mismatched ENVIRONMENT"} {
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
				case "old plan":
					plan.Environment = ""
					for i := range plan.Artifacts {
						plan.Artifacts[i].Environments = nil
					}
					wantError = "invalid environment"
				case "missing environment":
					plan.Environment = ""
					wantError = "invalid environment"
				case "invalid environment":
					plan.Environment = "production"
					wantError = "invalid environment"
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
				err = ApplyWithOptions(filepath.Join(root, "bear.config.yml"), Options{Environment: "int", NoCommit: true, Concurrency: 1})
				if err == nil || !strings.Contains(err.Error(), wantError) || !strings.Contains(err.Error(), "run 'bear plan <environment>' again") {
					t.Fatalf("expected actionable %q error, got %v", wantError, err)
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
