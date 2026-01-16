package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/irevolve/bear/internal"
	"github.com/irevolve/bear/internal/config"
)

func ApplyWithOptions(configPath string, opts Options) error {
	// Create context for cancellation support
	ctx := context.Background()

	cfg, err := internal.Load(configPath)
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	rootPath := filepath.Dir(configPath)
	if rootPath == "." {
		rootPath, _ = os.Getwd()
	}

	planOpts := internal.PlanOptions{
		Artifacts: opts.Artifacts,
		PinCommit: opts.PinCommit,
		Force:     opts.Force,
	}

	plan, err := internal.CreatePlanWithOptions(rootPath, cfg, planOpts)
	if err != nil {
		return fmt.Errorf("error creating plan: %w", err)
	}

	if plan.ToValidate == 0 && plan.ToDeploy == 0 {
		if len(opts.Artifacts) > 0 {
			fmt.Printf("No artifacts found matching: %v\n", opts.Artifacts)
		} else {
			fmt.Println("No changes detected. Nothing to apply.")
		}
		return nil
	}

	currentCommit := internal.GetCurrentCommit(rootPath)
	deployVersion := currentCommit

	// For pin, use the pinned commit for the version
	if opts.PinCommit != "" {
		deployVersion = opts.PinCommit
		fmt.Println()
		fmt.Println("📌 PIN MODE")
		fmt.Printf("   Pinning to commit: %s\n", opts.PinCommit[:min(8, len(opts.PinCommit))])
	}

	fmt.Println()
	fmt.Println("Bear Apply")
	fmt.Println("==========")
	fmt.Println()

	// Gruppiere Aktionen
	var validates, deploys []internal.PlannedAction
	for _, action := range plan.Actions {
		switch action.Action {
		case internal.ActionValidate:
			validates = append(validates, action)
		case internal.ActionDeploy:
			deploys = append(deploys, action)
		}
	}

	// Phase 1: Validation
	if len(validates) > 0 {
		fmt.Printf("📋 Phase 1: Validating %d artifact(s)...\n\n", len(validates))

		for _, v := range validates {
			fmt.Printf("  🔍 %s\n", v.Artifact.Artifact.Name)

			for _, step := range v.Steps {
				fmt.Printf("     → %s\n", step.Name)

				if err := executeStep(ctx, step, v.Artifact.Path, v.Artifact.Artifact.Params); err != nil {
					fmt.Printf("     ❌ Failed: %v\n", err)
					return fmt.Errorf("validation failed for %s: %w", v.Artifact.Artifact.Name, err)
				}
				fmt.Printf("     ✓ Done\n")
			}
			fmt.Println()
		}

		fmt.Println("✅ All validations passed!")
		fmt.Println()
	}

	// Phase 2: Deployment
	if len(deploys) > 0 {
		fmt.Printf("🚀 Phase 2: Deploying %d artifact(s)...\n\n", len(deploys))

		for _, d := range deploys {
			fmt.Printf("  📦 %s → %s\n", d.Artifact.Artifact.Name, d.Artifact.Artifact.Target)

			// Merge target defaults with artifact params
			params := mergeParams(cfg, d.Artifact.Artifact.Target, d.Artifact.Artifact.Params)
			params["NAME"] = d.Artifact.Artifact.Name
			params["VERSION"] = deployVersion[:min(7, len(deployVersion))]

			for _, step := range d.Steps {
				fmt.Printf("     → %s\n", step.Name)

				if err := executeStep(ctx, step, d.Artifact.Path, params); err != nil {
					fmt.Printf("     ❌ Failed: %v\n", err)
					return fmt.Errorf("deployment failed for %s: %w", d.Artifact.Artifact.Name, err)
				}
				fmt.Printf("     ✓ Done\n")
			}

			// Update lock file after successful deployment
			// For pin, the artifact is pinned (unless with --force)
			if opts.PinCommit != "" && !opts.Force {
				plan.LockFile.UpdateArtifactPinned(
					d.Artifact.Artifact.Name,
					deployVersion,
					d.Artifact.Artifact.Target,
					deployVersion[:min(7, len(deployVersion))],
				)
			} else {
				// Normal update or --force: pin is removed
				plan.LockFile.UpdateArtifact(
					d.Artifact.Artifact.Name,
					deployVersion,
					d.Artifact.Artifact.Target,
					deployVersion[:min(7, len(deployVersion))],
				)
			}

			fmt.Println()
		}

		// Save lock file
		if err := plan.LockFile.Save(plan.LockPath); err != nil {
			return fmt.Errorf("error saving lock file: %w", err)
		}
		fmt.Printf("📝 Lock file updated: %s\n", plan.LockPath)

		// Automatically commit with [skip ci]
		if opts.Commit {
			if err := commitLockFile(rootPath, plan.LockPath, deploys); err != nil {
				fmt.Printf("⚠️  Warning: Failed to commit lock file: %v\n", err)
			} else {
				fmt.Println("📤 Lock file committed with [skip ci]")
			}
		}
		fmt.Println()

		fmt.Println("✅ All deployments completed!")
	}

	fmt.Println()
	fmt.Println("─────────────────────────────────")
	fmt.Printf("Applied: %d validated, %d deployed\n", len(validates), len(deploys))

	return nil
}

func executeStep(ctx context.Context, step config.Step, workDir string, params map[string]string) error {
	// Replace parameters in the run command
	command := step.Run
	for key, value := range params {
		command = strings.ReplaceAll(command, "$"+key, value)
		command = strings.ReplaceAll(command, "${"+key+"}", value)
	}

	// Detect shell based on OS
	shell, shellArg := getShell()

	// Execute the command with context for cancellation support
	cmd := exec.CommandContext(ctx, shell, shellArg, command)
	cmd.Dir = workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// getShell returns the appropriate shell and argument for the current OS
func getShell() (string, string) {
	if isWindows() {
		return "cmd", "/C"
	}
	return "sh", "-c"
}

// isWindows checks if the current OS is Windows
func isWindows() bool {
	return os.PathSeparator == '\\' && os.PathListSeparator == ';'
}

func mergeParams(cfg *config.Config, targetName string, artifactParams map[string]string) map[string]string {
	params := make(map[string]string)

	// Find target template and apply defaults
	for _, t := range cfg.Targets {
		if t.Name == targetName {
			for k, v := range t.Defaults {
				params[k] = v
			}
			break
		}
	}

	// Override with artifact params
	for k, v := range artifactParams {
		params[k] = v
	}

	return params
}

// commitLockFile commits the lock file with [skip ci] to prevent CI loops
func commitLockFile(rootPath, lockPath string, deploys []internal.PlannedAction) error {
	// Build commit message
	var names []string
	for _, d := range deploys {
		names = append(names, d.Artifact.Artifact.Name)
	}
	msg := fmt.Sprintf("chore(bear): update lock file [skip ci]\n\nDeployed: %s", strings.Join(names, ", "))

	// Git add
	addCmd := exec.Command("git", "add", lockPath)
	addCmd.Dir = rootPath
	if err := addCmd.Run(); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}

	// Git commit
	commitCmd := exec.Command("git", "commit", "-m", msg)
	commitCmd.Dir = rootPath
	if err := commitCmd.Run(); err != nil {
		return fmt.Errorf("git commit failed: %w", err)
	}

	// Git push
	pushCmd := exec.Command("git", "push")
	pushCmd.Dir = rootPath
	if err := pushCmd.Run(); err != nil {
		return fmt.Errorf("git push failed: %w", err)
	}

	return nil
}
