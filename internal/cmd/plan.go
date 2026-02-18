package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/irevolve/bear/internal"
	"github.com/irevolve/bear/internal/config"
)

func PlanWithOptions(configPath string, opts Options) error {
	ctx := context.Background()
	p := NewPrinter()

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

	// Group actions
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

	if len(validates) == 0 && len(deploys) == 0 {
		if len(opts.Artifacts) > 0 {
			p.Printf("No artifacts found matching: %v\n", opts.Artifacts)
		} else {
			p.Println("No changes detected. Nothing to plan.")
		}
		return nil
	}

	currentCommit := internal.GetCurrentCommit(rootPath)
	deployVersion := currentCommit
	if opts.PinCommit != "" {
		deployVersion = opts.PinCommit
	}

	// Phase 1: Validate all changed artifacts in parallel
	if len(validates) > 0 {
		p.PhaseHeader(fmt.Sprintf("Validating %d artifact(s)", len(validates)))

		type valResult struct {
			name   string
			output string
			err    error
		}
		results := make([]valResult, len(validates))

		errs := RunParallel(ctx, opts.Concurrency, len(validates), func(ctx context.Context, i int) error {
			v := validates[i]
			var combinedOutput bytes.Buffer

			for _, step := range v.Steps {
				var stdout, stderr bytes.Buffer
				execErr := ExecuteStep(ctx, step.Run, v.Artifact.Path, v.Artifact.Artifact.Params, &stdout, &stderr)

				if opts.Verbose {
					combinedOutput.WriteString(fmt.Sprintf("  → %s\n", step.Name))
					combinedOutput.Write(stdout.Bytes())
					combinedOutput.Write(stderr.Bytes())
				}

				if execErr != nil {
					combinedOutput.Write(stdout.Bytes())
					combinedOutput.Write(stderr.Bytes())
					results[i] = valResult{
						name:   v.Artifact.Artifact.Name,
						output: combinedOutput.String(),
						err:    fmt.Errorf("%s: %w", step.Name, execErr),
					}
					return execErr
				}
			}

			results[i] = valResult{
				name:   v.Artifact.Artifact.Name,
				output: combinedOutput.String(),
			}
			return nil
		})

		// Print results in order
		var failures []string
		for i, res := range results {
			_ = i
			if res.err != nil {
				p.FailureWithOutput(fmt.Sprintf("%s — %s", res.name, res.err), res.output)
				failures = append(failures, res.name)
			} else {
				p.Success(res.name)
				if opts.Verbose && res.output != "" {
					p.ErrorBox(res.output)
				}
			}
		}

		if len(CollectErrors(errs)) > 0 {
			p.Blank()
			p.Printf("  %s\n", p.red(fmt.Sprintf("Validation failed for: %s", strings.Join(failures, ", "))))
			p.Hint("Fix the errors above and run 'bear plan' again.")
			return fmt.Errorf("validation failed")
		}

		p.Blank()
		p.Printf("  %s\n", p.green("All validations passed!"))
	}

	// Phase 2: Write plan file
	planFile := config.NewPlanFile(currentCommit)
	planFile.Validated = len(validates)

	for _, d := range deploys {
		params := mergeParams(cfg, d.Artifact.Artifact.Target, d.Artifact.Artifact.Params)
		params["NAME"] = d.Artifact.Artifact.Name
		params["VERSION"] = deployVersion[:min(7, len(deployVersion))]

		pa := config.PlanArtifact{
			Name:         d.Artifact.Artifact.Name,
			Path:         d.Artifact.Path,
			Language:     d.Artifact.Language,
			Target:       d.Artifact.Artifact.Target,
			Action:       "deploy",
			Reason:       d.Reason,
			ChangedFiles: d.ChangedFiles,
			Params:       params,
			Steps:        d.Steps,
			IsLib:        d.Artifact.Artifact.IsLib,
		}

		if d.PinCommit != "" {
			pa.Pinned = true
			pa.PinCommit = d.PinCommit
		}

		planFile.Artifacts = append(planFile.Artifacts, pa)
		planFile.ToDeploy++
	}

	for _, s := range skips {
		planFile.Skipped = append(planFile.Skipped, config.PlanSkipped{
			Name:   s.Artifact.Artifact.Name,
			Reason: s.Reason,
		})
		planFile.TotalSkips++
	}

	if err := config.WritePlan(rootPath, planFile); err != nil {
		return fmt.Errorf("error writing plan file: %w", err)
	}

	// Phase 3: Show the validated plan
	printValidatedPlan(p, plan, planFile, rootPath, opts)

	return nil
}

func printValidatedPlan(p *Printer, plan *internal.Plan, planFile *config.PlanFile, rootPath string, opts Options) {
	p.PhaseHeader("Plan")

	if len(opts.Artifacts) > 0 {
		p.Printf("  Artifacts: %s\n", strings.Join(opts.Artifacts, ", "))
		p.Blank()
	}

	if opts.PinCommit != "" {
		p.Printf("  %s Pinning to: %s\n", p.yellow("📌"), opts.PinCommit[:min(8, len(opts.PinCommit))])
		p.Blank()
	}

	if plan.TotalChanges > 0 {
		p.Printf("  %s\n", p.dim(fmt.Sprintf("%d file(s) changed", plan.TotalChanges)))
		p.Blank()
	}

	// Show deployments
	if len(planFile.Artifacts) > 0 {
		p.Printf("  %s\n", p.cyan("To Deploy:"))
		p.Blank()
		for _, d := range planFile.Artifacts {
			relPath, _ := filepath.Rel(rootPath, d.Path)
			p.Printf("  %s %s\n", p.bold("~"), p.bold(d.Name))
			p.Detail("Path:  ", relPath)
			p.Detail("Target:", d.Target)
			p.Detail("Reason:", d.Reason)

			if plan.LockFile != nil {
				lastCommit := plan.LockFile.GetLastDeployedCommit(d.Name)
				if lastCommit != "" {
					p.Detail("Last:  ", lastCommit[:min(7, len(lastCommit))])
				} else {
					p.Detail("Last:  ", "(never deployed)")
				}
			}

			if len(d.Steps) > 0 {
				p.Detail("Steps: ", fmt.Sprintf("%d", len(d.Steps)))
				for _, step := range d.Steps {
					p.Printf("             %s\n", p.dim("- "+step.Name))
				}
			}
			p.Blank()
		}
	}

	// Show skips (compact)
	if len(planFile.Skipped) > 0 {
		p.Printf("  %s\n", p.dim("Unchanged (will skip):"))
		p.Blank()
		for _, s := range planFile.Skipped {
			reason := ""
			if strings.Contains(s.Reason, "pinned") {
				reason = " 📌"
			}
			p.Printf("  %s %s%s\n", p.dim("–"), p.dim(s.Name), reason)
		}
		p.Blank()
	}

	// Summary
	parts := []string{}
	if planFile.Validated > 0 {
		parts = append(parts, p.SummaryValidated(planFile.Validated))
	}
	if planFile.ToDeploy > 0 {
		parts = append(parts, p.SummaryDeploy(planFile.ToDeploy))
	}
	if planFile.TotalSkips > 0 {
		parts = append(parts, p.SummarySkipped(planFile.TotalSkips))
	}
	p.Summary(parts...)

	if planFile.ToDeploy > 0 {
		p.Hint("Run 'bear apply' to execute this plan.")
	}
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
