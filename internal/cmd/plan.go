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

	p.BearHeader("Plan")

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
		p.Blank()
		if len(opts.Artifacts) > 0 && len(skips) == 0 {
			p.Printf("No artifacts found matching: %v\n", opts.Artifacts)
		} else {
			p.Println("No changes detected. Nothing to plan.")
		}
		skip := make([]summaryEntry, 0, len(skips))
		for _, s := range skips {
			skip = append(skip, skipEntry(planSkip(s)))
		}
		p.Blank()
		printEnvironmentSummary(p, summaryHeader{Environment: plan.Environment}, summarySection{Label: "skip", Color: p.dim, Entries: skip})
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
		p.PhaseHeader("Validating " + plural(len(validates), "artifact", "artifacts"))

		// Build task names for progress tracker
		valTaskNames := make([]string, len(validates))
		for i, v := range validates {
			valTaskNames[i] = v.Artifact.Artifact.Name
		}

		pt := NewProgressTracker(p, valTaskNames)
		pt.SetOperation("validate")
		if opts.Verbose {
			pt.UsePlainOutput()
		}
		pt.Start()

		errs := RunParallel(ctx, opts.Concurrency, len(validates), func(ctx context.Context, i int) error {
			v := planFile.Validations[i]
			pt.MarkRunning(i)
			var combinedOutput TailBuffer

			for stepIndex, step := range v.Steps {
				pt.MarkStep(i, step.Name, stepIndex+1, len(v.Steps))
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

		printResult(p, p.green, "Validation complete", []string{plural(len(validates), "artifact", "artifacts")}, pt.TotalElapsed())
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
		planFile.Skipped = append(planFile.Skipped, planSkip(s))
		planFile.TotalSkips++
	}

	if err := config.WritePlan(rootPath, planFile); err != nil {
		return fmt.Errorf("error writing plan file: %w", err)
	}

	// Phase 3: Show the validated plan
	printValidatedPlan(p, plan, planFile, opts)

	return nil
}

func printValidatedPlan(p *Printer, plan *internal.Plan, planFile *config.PlanFile, opts Options) {
	header := summaryHeader{Environment: planFile.Environment}
	// The source is identical for every deployment, so report it once here
	// instead of repeating it under each artifact.
	commit := [2]string{"Commit", shortCommit(planFile.Commit)}
	if planFile.Pinned {
		commit[0] = "Pinned"
	}
	header.Facts = append(header.Facts, commit)
	if len(opts.Artifacts) > 0 {
		header.Facts = append(header.Facts, [2]string{"Artifacts", strings.Join(opts.Artifacts, ", ")})
	}
	if plan.TotalChanges > 0 {
		header.Facts = append(header.Facts, [2]string{"Changes", plural(plan.TotalChanges, "file", "files")})
	}

	deploy := make([]summaryEntry, 0, len(planFile.Artifacts))
	for _, artifact := range planFile.Artifacts {
		deploy = append(deploy, deployEntry(artifact))
	}
	p.Blank()
	printEnvironmentSummary(p, header,
		summarySection{Label: "deploy", Color: p.cyan, Entries: deploy},
		summarySection{Label: "skip", Color: p.dim, Entries: planSkipEntries(planFile.Skipped)},
	)

	// The closing sentence mirrors apply, so both commands end the same way.
	counts := []string{
		plural(planFile.Validated, "validated", "validated"),
		plural(planFile.ToDeploy, "to deploy", "to deploy"),
	}
	if planFile.TotalSkips > 0 {
		counts = append(counts, plural(planFile.TotalSkips, "skipped", "skipped"))
	}
	printResult(p, p.cyan, "Plan complete", counts, 0)

	if planFile.ToDeploy > 0 {
		p.Hint("Run 'bear apply' to execute this plan.")
	}
}

// planSkip records why an artifact is not deployed.
func planSkip(action internal.PlannedAction) config.PlanSkipped {
	return config.PlanSkipped{
		Name:   action.Artifact.Artifact.Name,
		Path:   action.Artifact.Path,
		Reason: action.Reason,
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
