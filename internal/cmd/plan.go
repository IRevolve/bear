package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/irevolve/bear/internal"
)

func PlanWithOptions(configPath string, opts Options) error {
	cfg, err := internal.Load(configPath)
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	rootPath := filepath.Dir(configPath)
	if rootPath == "." {
		rootPath, _ = os.Getwd()
	}

	planOpts := internal.PlanOptions{
		Artifacts:      opts.Artifacts,
		RollbackCommit: opts.RollbackCommit,
		Force:          opts.Force,
	}

	plan, err := internal.CreatePlanWithOptions(rootPath, cfg, planOpts)
	if err != nil {
		return fmt.Errorf("error creating plan: %w", err)
	}

	printPlan(plan, rootPath, opts)

	return nil
}

func printPlan(plan *internal.Plan, rootPath string, opts Options) {
	fmt.Println()
	if opts.RollbackCommit != "" {
		fmt.Println("Bear Rollback Plan")
		fmt.Println("==================")
	} else {
		fmt.Println("Bear Execution Plan")
		fmt.Println("===================")
	}
	fmt.Println()

	if len(opts.Artifacts) > 0 {
		fmt.Printf("🎯 Artifacts: %s\n\n", strings.Join(opts.Artifacts, ", "))
	}

	if opts.RollbackCommit != "" {
		fmt.Printf("⏪ Rolling back to: %s\n\n", opts.RollbackCommit)
	}

	if plan.TotalChanges > 0 {
		fmt.Printf("📊 %d file(s) changed\n\n", plan.TotalChanges)
	}

	// Group by action type
	var validates, deploys, skips []internal.PlannedAction

	for _, action := range plan.Actions {
		switch action.Action {
		case internal.ActionValidate:
			validates = append(validates, action)
		case internal.ActionDeploy:
			deploys = append(deploys, action)
		case internal.ActionSkip:
			skips = append(skips, action)
		}
	}

	// Show validations
	if len(validates) > 0 {
		fmt.Println("🔍 To Validate:")
		fmt.Println()
		for _, v := range validates {
			relPath, _ := filepath.Rel(rootPath, v.Artifact.Path)
			fmt.Printf("  + %s\n", v.Artifact.Artifact.Name)
			fmt.Printf("    Path:     %s\n", relPath)
			fmt.Printf("    Language: %s\n", v.Artifact.Language)
			fmt.Printf("    Reason:   %s\n", v.Reason)
			if len(v.ChangedFiles) > 0 && len(v.ChangedFiles) <= 5 {
				fmt.Printf("    Changed:  %s\n", strings.Join(v.ChangedFiles, ", "))
			} else if len(v.ChangedFiles) > 5 {
				fmt.Printf("    Changed:  %d files\n", len(v.ChangedFiles))
			}
			if len(v.Steps) > 0 {
				fmt.Printf("    Steps:    %d\n", len(v.Steps))
				for _, step := range v.Steps {
					fmt.Printf("              - %s\n", step.Name)
				}
			}
			fmt.Println()
		}
	}

	// Show deployments
	if len(deploys) > 0 {
		fmt.Println("🚀 To Deploy:")
		fmt.Println()
		for _, d := range deploys {
			relPath, _ := filepath.Rel(rootPath, d.Artifact.Path)
			fmt.Printf("  ~ %s\n", d.Artifact.Artifact.Name)
			fmt.Printf("    Path:   %s\n", relPath)
			fmt.Printf("    Target: %s\n", d.Artifact.Artifact.Target)
			fmt.Printf("    Reason: %s\n", d.Reason)

			// Show last deployed commit from lock file
			if plan.LockFile != nil {
				lastCommit := plan.LockFile.GetLastDeployedCommit(d.Artifact.Artifact.Name)
				if lastCommit != "" {
					fmt.Printf("    Last:   %s\n", lastCommit[:min(7, len(lastCommit))])
				} else {
					fmt.Printf("    Last:   (never deployed)\n")
				}
			}

			if len(d.Steps) > 0 {
				fmt.Printf("    Steps:  %d\n", len(d.Steps))
				for _, step := range d.Steps {
					fmt.Printf("            - %s\n", step.Name)
				}
			}
			fmt.Println()
		}
	}

	// Show skips (compact)
	if len(skips) > 0 {
		fmt.Println("⏭️  Unchanged (will skip):")
		fmt.Println()
		for _, s := range skips {
			lastCommit := ""
			isPinned := false
			if plan.LockFile != nil {
				lastCommit = plan.LockFile.GetLastDeployedCommit(s.Artifact.Artifact.Name)
				isPinned = plan.LockFile.IsPinned(s.Artifact.Artifact.Name)
			}

			if isPinned {
				fmt.Printf("  - %s 📌 PINNED (version: %s)\n", s.Artifact.Artifact.Name, lastCommit[:min(7, len(lastCommit))])
			} else if lastCommit != "" {
				fmt.Printf("  - %s (deployed: %s)\n", s.Artifact.Artifact.Name, lastCommit[:min(7, len(lastCommit))])
			} else {
				fmt.Printf("  - %s\n", s.Artifact.Artifact.Name)
			}
		}
		fmt.Println()
	}

	// Summary
	fmt.Println("─────────────────────────────────")
	fmt.Println()
	fmt.Printf("Plan: %d to validate, %d to deploy, %d unchanged\n",
		plan.ToValidate, plan.ToDeploy, plan.ToSkip)
	fmt.Println()

	if plan.ToValidate > 0 || plan.ToDeploy > 0 {
		fmt.Println("Run 'bear apply' to execute this plan.")
	} else {
		fmt.Println("No changes detected. Nothing to do.")
	}
}
