package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/irevolve/bear/internal"
	"github.com/irevolve/bear/internal/config"
)

func PlanWithOptions(configPath string, opts Options) (retErr error) {
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	p := NewPrinter()

	rootPath, err := filepath.Abs(filepath.Dir(configPath))
	if err != nil {
		return fmt.Errorf("error resolving project directory: %w", err)
	}
	release, err := acquireWorkspaceLock(rootPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := release(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("release workspace lock: %w", err))
		}
	}()

	if err := config.RemovePlan(rootPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("error removing previous plan: %w", err)
	}

	cfg, err := internal.Load(configPath)
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}
	releaseRepository, err := acquireRepositoryLock(ctx, rootPath)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, releaseRepository()) }()

	planOpts := internal.PlanOptions{
		Environment: opts.Environment,
		Artifacts:   opts.Artifacts,
		PinCommit:   opts.PinCommit,
		Force:       opts.Force,
	}

	plan, err := internal.CreatePlanWithOptions(rootPath, cfg, planOpts)
	if err != nil {
		return fmt.Errorf("error creating plan: %w", err)
	}
	if plan.LockFile != nil && len(plan.LockFile.Artifacts) > 0 {
		p.Println("Warning: legacy lock artifacts have no environment; they are ignored and will not be migrated automatically.")
	}

	// Select policy from the current workspace, then execute only the selected source.
	sourceRoot := rootPath
	var pinnedCommit string
	if opts.PinCommit != "" {
		var cleanup func() error
		sourceRoot, pinnedCommit, cleanup, err = prepareSource(ctx, rootPath, opts.PinCommit)
		if err != nil {
			return fmt.Errorf("prepare pinned source: %w", err)
		}
		defer func() {
			if err := cleanup(); err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("clean up pinned source: %w", err))
			}
		}()
	}
	currentCommit, _, dirty, err := sourceState(ctx, sourceRoot)
	if err != nil {
		return fmt.Errorf("check plan source: %w", err)
	}
	if pinnedCommit != "" && currentCommit != pinnedCommit {
		return fmt.Errorf("pinned source HEAD changed before validation")
	}
	if dirty && (opts.PinCommit != "" || plan.ToDeploy > 0) {
		return fmt.Errorf("plan requires clean source before validation; commit or remove source changes first")
	}

	for i := range plan.Actions {
		action := &plan.Actions[i]
		if pinnedCommit != "" {
			action.PinCommit = pinnedCommit
		}
		rel, err := filepath.Rel(rootPath, action.Artifact.Path)
		if err != nil {
			return fmt.Errorf("artifact %s path: %w", action.Artifact.Artifact.Name, err)
		}
		action.Artifact.Path = filepath.ToSlash(rel)
		if action.Action != internal.ActionSkip {
			path, err := safeArtifactPath(sourceRoot, action.Artifact.Path)
			if err != nil {
				return err
			}
			info, err := os.Stat(path)
			if err != nil {
				return fmt.Errorf("artifact %s source directory: %w", action.Artifact.Artifact.Name, err)
			}
			if !info.IsDir() {
				return fmt.Errorf("artifact %s source path is not a directory", action.Artifact.Artifact.Name)
			}
		}
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
		if plan.Environment != "" {
			p.Printf("Environment: %s\n", plan.Environment)
		}
		for _, s := range skips {
			p.Printf("  %s: %s\n", s.Artifact.Artifact.Name, s.Reason)
		}
		if len(opts.Artifacts) > 0 && len(skips) == 0 {
			p.Printf("No artifacts found matching: %v\n", opts.Artifacts)
		} else {
			p.Println("No changes detected. Nothing to plan.")
		}
		return nil
	}

	repo, _, err := sourceRepository(ctx, sourceRoot)
	if err != nil {
		return err
	}
	trackedDiff := func() ([]byte, error) {
		return sourceGit(ctx, repo, "-c", "core.fileMode=true", "diff", "--no-ext-diff", "--no-textconv", "--binary", "HEAD", "--", ".", ":(exclude,glob)**/.bear/**", ":(exclude,glob)**/bear.lock.yml")
	}
	beforeDiff, err := trackedDiff()
	if err != nil {
		return fmt.Errorf("check tracked source before validation: %w", err)
	}
	planFile := config.NewPlanFile(currentCommit)
	planFile.Environment = plan.Environment
	planFile.Pinned = opts.PinCommit != ""
	planFile.Validated = len(validates)
	for _, v := range validates {
		planFile.Validations = append(planFile.Validations, config.PlanValidation{
			Name:  v.Artifact.Artifact.Name,
			Path:  v.Artifact.Path,
			Vars:  mergeVars(cfg, v.Artifact.Artifact.Target, v.Artifact.Language, v.Artifact.Artifact.Vars, plan.Environment),
			Steps: v.Steps,
		})
	}

	// Phase 1: Validate all changed artifacts in parallel
	if len(validates) > 0 {
		p.PhaseHeader(fmt.Sprintf("Validating %d artifact(s)", len(validates)))

		// Build task names for progress tracker
		valTaskNames := make([]string, len(validates))
		for i, v := range validates {
			valTaskNames[i] = v.Artifact.Artifact.Name
		}

		pt := NewProgressTracker(p, fmt.Sprintf("Validating %d artifact(s)", len(validates)), valTaskNames)
		if opts.Verbose {
			pt.UsePlainOutput()
		}
		pt.Start()

		errs := RunParallel(ctx, opts.Concurrency, len(validates), func(ctx context.Context, i int) error {
			v := planFile.Validations[i]
			pt.MarkRunning(i)
			var combinedOutput TailBuffer

			for stepIndex, step := range v.Steps {
				pt.MarkStep(i, fmt.Sprintf("validate / %s (%d/%d)", step.Name, stepIndex+1, len(v.Steps)))
				var output io.Writer = &combinedOutput
				if opts.Verbose {
					output = io.MultiWriter(&combinedOutput, pt.StepWriter(i, step.Name))
				}
				path, execErr := safeArtifactPath(sourceRoot, v.Path)
				if execErr == nil {
					execErr = ExecuteStep(ctx, step.Run, path, v.Vars, output, output)
				}
				if execErr != nil {
					execErr = fmt.Errorf("%s: %w", step.Name, execErr)
					pt.MarkFailed(i, execErr, combinedOutput.String())
					return execErr
				}
			}

			pt.MarkDone(i)
			return nil
		})

		// Check for failures
		var failures []string
		for i, err := range errs {
			if err != nil {
				pt.MarkFailed(i, err, "")
				failures = append(failures, valTaskNames[i])
			}
		}
		pt.Stop()

		if len(CollectErrors(errs)) > 0 {
			p.Blank()
			p.Printf("  %s\n", p.red(fmt.Sprintf("Validation failed for: %s", strings.Join(failures, ", "))))
			p.Hint(fmt.Sprintf("Fix the errors above and run 'bear plan %s' again.", plan.Environment))
			return fmt.Errorf("validation failed: %w", errors.Join(CollectErrors(errs)...))
		}

		p.Blank()
		p.Printf("  %s %s\n", p.green("All validations passed!"), p.dim(fmt.Sprintf("⏱ %s", formatDuration(pt.TotalElapsed()))))
	}

	// Generated untracked output is allowed and included in the saved fingerprint.
	// Tracked source and HEAD must remain unchanged, including for validation-only plans.
	afterCommit, fingerprint, _, err := sourceState(ctx, sourceRoot)
	if err != nil {
		return fmt.Errorf("check source after validation: %w", err)
	}
	if afterCommit != currentCommit {
		return fmt.Errorf("HEAD changed during validation")
	}
	afterDiff, err := trackedDiff()
	if err != nil {
		return fmt.Errorf("check tracked source after validation: %w", err)
	}
	if !bytes.Equal(beforeDiff, afterDiff) {
		return fmt.Errorf("tracked source changed during validation")
	}
	planFile.SourceFingerprint = fingerprint

	// Phase 2: Write plan file
	for _, d := range deploys {
		vars := mergeVars(cfg, d.Artifact.Artifact.Target, d.Artifact.Language, d.Artifact.Artifact.Vars, plan.Environment)
		vars["NAME"] = d.Artifact.Artifact.Name
		vars["VERSION"] = currentCommit[:min(7, len(currentCommit))]

		pa := config.PlanArtifact{
			Name:         d.Artifact.Artifact.Name,
			Path:         d.Artifact.Path,
			Language:     d.Artifact.Language,
			Target:       d.Artifact.Artifact.Target,
			Environments: slices.Clone(d.Artifact.Artifact.Environments),
			Action:       "deploy",
			Reason:       d.Reason,
			ChangedFiles: d.ChangedFiles,
			Vars:         vars,
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
	if planFile.Environment != "" {
		p.Printf("  Environment: %s\n", planFile.Environment)
		p.Blank()
	}

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
			p.Printf("  %s %s\n", p.bold("~"), p.bold(d.Name))
			p.Detail("Path:  ", d.Path)
			p.Detail("Target:", d.Target)
			p.Detail("Reason:", d.Reason)

			if plan.LockFile != nil {
				lastCommit := plan.LockFile.GetLastDeployedCommit(planFile.Environment, d.Name)
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
		p.Printf("  %s\n", p.dim("Skipped:"))
		p.Blank()
		for _, s := range planFile.Skipped {
			p.Printf("  %s: %s\n", p.dim(s.Name), s.Reason)
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

func mergeVars(cfg *config.Config, targetName string, langName string, artifactVars map[string]string, environment string) map[string]string {
	vars := make(map[string]string)

	// 1. Language vars (lowest priority)
	if lang, ok := cfg.Languages[langName]; ok {
		for k, v := range lang.Vars {
			vars[k] = v
		}
	}

	// 2. Target vars
	if t, ok := cfg.Targets[targetName]; ok {
		for k, v := range t.Vars {
			vars[k] = v
		}
	}

	// 3. Artifact vars
	for k, v := range artifactVars {
		vars[k] = v
	}

	if environment != "" {
		vars["ENVIRONMENT"] = environment
	}

	return vars
}
