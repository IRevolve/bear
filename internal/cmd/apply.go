package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/IRevolve/Bear/internal/config"
	"github.com/IRevolve/Bear/internal/detector"
	"github.com/IRevolve/Bear/internal/planner"
)

func Apply(configPath string, dryRun bool) error {
	return ApplyWithOptions(configPath, Options{DryRun: dryRun})
}

func ApplyWithOptions(configPath string, opts Options) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	rootPath := filepath.Dir(configPath)
	if rootPath == "." {
		rootPath, _ = os.Getwd()
	}

	planOpts := planner.PlanOptions{
		Artifacts:      opts.Artifacts,
		RollbackCommit: opts.RollbackCommit,
	}

	plan, err := planner.CreatePlanWithOptions(rootPath, cfg, planOpts)
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

	currentCommit := detector.GetCurrentCommit(rootPath)
	deployVersion := currentCommit

	// Bei Rollback verwenden wir den Rollback-Commit für die Version
	if opts.RollbackCommit != "" {
		deployVersion = opts.RollbackCommit
		fmt.Println()
		fmt.Println("⚠️  ROLLBACK MODE")
		fmt.Printf("   Rolling back to commit: %s\n", opts.RollbackCommit[:min(8, len(opts.RollbackCommit))])
	}

	fmt.Println()
	fmt.Println("Bear Apply")
	fmt.Println("==========")
	fmt.Println()

	// Gruppiere Aktionen
	var validates, deploys []planner.PlannedAction
	for _, action := range plan.Actions {
		switch action.Action {
		case planner.ActionValidate:
			validates = append(validates, action)
		case planner.ActionDeploy:
			deploys = append(deploys, action)
		}
	}

	// Phase 1: Validation
	if len(validates) > 0 {
		fmt.Printf("📋 Phase 1: Validating %d artifact(s)...\n\n", len(validates))

		for _, v := range validates {
			fmt.Printf("  🔍 %s\n", v.Artifact.Artifact.Name)

			if opts.DryRun {
				fmt.Printf("     [dry-run] Would execute %d steps\n", len(v.Steps))
				continue
			}

			for _, step := range v.Steps {
				fmt.Printf("     → %s\n", step.Name)

				if err := executeStep(step, v.Artifact.Path, v.Artifact.Artifact.Params, cfg); err != nil {
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

			if opts.DryRun {
				fmt.Printf("     [dry-run] Would execute %d steps\n", len(d.Steps))
				continue
			}

			// Merge Target-Defaults mit Artifact-Params
			params := mergeParams(cfg, d.Artifact.Artifact.Target, d.Artifact.Artifact.Params)
			params["NAME"] = d.Artifact.Artifact.Name
			params["VERSION"] = deployVersion[:min(7, len(deployVersion))]

			for _, step := range d.Steps {
				fmt.Printf("     → %s\n", step.Name)

				if err := executeStep(step, d.Artifact.Path, params, cfg); err != nil {
					fmt.Printf("     ❌ Failed: %v\n", err)
					return fmt.Errorf("deployment failed for %s: %w", d.Artifact.Artifact.Name, err)
				}
				fmt.Printf("     ✓ Done\n")
			}

			// Update Lock-Datei nach erfolgreichem Deployment
			// Bei Rollback speichern wir den Rollback-Commit
			plan.LockFile.UpdateArtifact(
				d.Artifact.Artifact.Name,
				deployVersion,
				d.Artifact.Artifact.Target,
				deployVersion[:min(7, len(deployVersion))],
			)

			fmt.Println()
		}

		// Speichere Lock-Datei
		if !opts.DryRun {
			if err := plan.LockFile.Save(plan.LockPath); err != nil {
				return fmt.Errorf("error saving lock file: %w", err)
			}
			fmt.Printf("📝 Lock file updated: %s\n\n", plan.LockPath)
		}

		fmt.Println("✅ All deployments completed!")
	}

	fmt.Println()
	fmt.Println("─────────────────────────────────")
	fmt.Printf("Applied: %d validated, %d deployed\n", len(validates), len(deploys))

	return nil
}

func executeStep(step config.Step, workDir string, params map[string]string, cfg *config.Config) error {
	// Ersetze Parameter in der Run-Anweisung
	command := step.Run
	for key, value := range params {
		command = strings.ReplaceAll(command, "$"+key, value)
		command = strings.ReplaceAll(command, "${"+key+"}", value)
	}

	// Führe den Befehl aus
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func mergeParams(cfg *config.Config, targetName string, artifactParams map[string]string) map[string]string {
	params := make(map[string]string)

	// Finde Target-Template und übernehme Defaults
	for _, t := range cfg.Targets {
		if t.Name == targetName {
			for k, v := range t.Defaults {
				params[k] = v
			}
			break
		}
	}

	// Überschreibe mit Artifact-Params
	for k, v := range artifactParams {
		params[k] = v
	}

	return params
}
