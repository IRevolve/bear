package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/irevolve/bear/internal/config"
)

func ApplyWithOptions(configPath string, opts Options) (retErr error) {
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
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

	planFile, err := config.ReadPlan(rootPath)
	if os.IsNotExist(err) {
		return fmt.Errorf("no plan found. Run 'bear plan <environment>' first (dev, int, or prd)")
	}
	if err != nil {
		return fmt.Errorf("error reading plan file: %w", err)
	}
	if planFile.Environment != "" {
		p.Printf("Environment: %s\n", planFile.Environment)
	}
	for _, s := range planFile.Skipped {
		p.Printf("  Skipped %s: %s\n", s.Name, s.Reason)
	}
	if len(planFile.Artifacts) == 0 {
		p.Println("Plan contains no artifacts to deploy.")
		return config.RemovePlan(rootPath)
	}

	// Preflight the entire snapshot before any subprocess or history update.
	// Saved permissions are evidence, not instructions to reload current config.
	if err := config.ValidateEnvironment(planFile.Environment); err != nil {
		return fmt.Errorf("unsafe saved plan: %w; run 'bear plan <environment>' again (dev, int, or prd)", err)
	}
	for _, artifact := range planFile.Artifacts {
		if !slices.Contains(artifact.Environments, planFile.Environment) {
			return fmt.Errorf("unsafe saved plan: artifact %q has no saved deployment permission for environment %s; run 'bear plan <environment>' again (dev, int, or prd)", artifact.Name, planFile.Environment)
		}
		if artifact.Vars["ENVIRONMENT"] != planFile.Environment {
			return fmt.Errorf("unsafe saved plan: artifact %q ENVIRONMENT does not match selected environment %s; run 'bear plan <environment>' again (dev, int, or prd)", artifact.Name, planFile.Environment)
		}
	}
	if planFile.Commit == "" || planFile.SourceFingerprint == "" {
		return fmt.Errorf("unsafe saved plan: missing source commit or fingerprint; run 'bear plan <environment>' again")
	}
	seen := make(map[string]bool)
	for _, artifact := range planFile.Artifacts {
		if seen[artifact.Name] {
			return fmt.Errorf("unsafe saved plan: duplicate artifact %q", artifact.Name)
		}
		seen[artifact.Name] = true
		if artifact.Pinned && (!planFile.Pinned || artifact.PinCommit != "" && artifact.PinCommit != planFile.Commit) {
			return fmt.Errorf("unsafe saved plan: artifact %q pin does not match approved source", artifact.Name)
		}
	}
	for _, validation := range planFile.Validations {
		if validation.Vars["ENVIRONMENT"] != planFile.Environment {
			return fmt.Errorf("unsafe saved plan: validation %q ENVIRONMENT does not match selected environment %s; run 'bear plan <environment>' again", validation.Name, planFile.Environment)
		}
	}

	releaseRepository, err := acquireRepositoryLock(ctx, rootPath)
	if err != nil {
		return err
	}
	defer func() { retErr = errors.Join(retErr, releaseRepository()) }()
	lockPath := filepath.Join(rootPath, "bear.lock.yml")
	lockFile, err := config.LoadLock(lockPath)
	if err != nil {
		return fmt.Errorf("error loading lock file: %w", err)
	}
	if len(lockFile.Artifacts) > 0 {
		p.Warning("Legacy lock history is retained for migration, not used as environment deployment history.")
	}
	var pending []int
	for i, artifact := range planFile.Artifacts {
		if !artifact.Completed {
			pending = append(pending, i)
			continue
		}
		commit := planFile.Commit
		if artifact.Pinned && artifact.PinCommit != "" {
			commit = artifact.PinCommit
		}
		entry, ok := lockFile.GetArtifact(planFile.Environment, artifact.Name)
		if !ok || entry.Commit != commit || entry.Target != artifact.Target || entry.Pinned != artifact.Pinned || entry.Version != commit[:min(7, len(commit))] || entry.Timestamp == "" {
			return fmt.Errorf("completed artifact %q does not match saved lock history; inspect deployment status before replanning", artifact.Name)
		}
	}

	sourceRoot := rootPath
	if planFile.Pinned && len(pending) > 0 {
		var commit string
		var cleanup func() error
		sourceRoot, commit, cleanup, err = prepareSource(ctx, rootPath, planFile.Commit)
		if err != nil {
			return fmt.Errorf("prepare pinned source: %w", err)
		}
		defer func() {
			if err := cleanup(); err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("clean up pinned source: %w", err))
			}
		}()
		if commit != planFile.Commit {
			return fmt.Errorf("pinned source commit does not match saved plan; run 'bear plan <environment>' again")
		}
	}
	for _, artifact := range planFile.Artifacts {
		if _, err := safeArtifactPath(sourceRoot, artifact.Path); err != nil {
			return fmt.Errorf("unsafe saved plan: artifact %q: %w; run 'bear plan <environment>' again", artifact.Name, err)
		}
	}
	for _, validation := range planFile.Validations {
		if _, err := safeArtifactPath(sourceRoot, validation.Path); err != nil {
			return fmt.Errorf("unsafe saved plan: validation %q: %w; run 'bear plan <environment>' again", validation.Name, err)
		}
	}

	runSteps := func(ctx context.Context, pt *ProgressTracker, i int, phase, path string, vars map[string]string, steps []config.Step) error {
		pt.MarkRunning(i)
		for stepIndex, step := range steps {
			pt.MarkStep(i, fmt.Sprintf("%s / %s (%d/%d)", phase, step.Name, stepIndex+1, len(steps)))
			var tail TailBuffer
			var output io.Writer = &tail
			if opts.Verbose {
				output = io.MultiWriter(&tail, pt.StepWriter(i, step.Name))
			}
			// Validation/setup may have replaced a path since preflight.
			workDir, err := safeArtifactPath(sourceRoot, path)
			if err == nil {
				err = ctx.Err()
			}
			if err == nil {
				err = ExecuteStep(ctx, step.Run, workDir, vars, output, output)
			}
			if err != nil {
				err = fmt.Errorf("%s: %w", step.Name, err)
				pt.MarkFailed(i, err, tail.String())
				return err
			}
		}
		return nil
	}

	p.BearHeader("Apply")
	if planFile.Pinned && len(pending) > 0 && len(planFile.Validations) > 0 {
		names := make([]string, len(planFile.Validations))
		for i, validation := range planFile.Validations {
			names[i] = validation.Name
		}
		pt := NewProgressTracker(p, fmt.Sprintf("Validating %d artifact(s)", len(names)), names)
		if opts.Verbose {
			pt.UsePlainOutput()
		}
		pt.Start()
		errs := RunParallel(ctx, opts.Concurrency, len(names), func(ctx context.Context, i int) error {
			v := planFile.Validations[i]
			if err := runSteps(ctx, pt, i, "validate", v.Path, v.Vars, v.Steps); err != nil {
				return err
			}
			pt.MarkDone(i)
			return nil
		})
		for i, err := range errs {
			if err != nil {
				pt.MarkFailed(i, err, "")
			}
		}
		pt.Stop()
		if err := errors.Join(errs...); err != nil {
			return fmt.Errorf("pinned validation failed; plan retained: %w", err)
		}
		p.Summary(p.SummaryValidated(len(names)), p.dim("total "+formatDuration(pt.TotalElapsed())))
	}
	// Fully checkpointed retries run no source commands. They only publish history;
	// the Git helper still refuses unrelated commits or an out-of-sync remote.
	if len(pending) > 0 {
		commit, fingerprint, _, err := sourceState(ctx, sourceRoot)
		if err != nil {
			return fmt.Errorf("verify approved source: %w", err)
		}
		if commit != planFile.Commit || fingerprint != planFile.SourceFingerprint {
			return fmt.Errorf("source commit or fingerprint does not match saved plan; run 'bear plan <environment>' again")
		}
		// Check every deployment path again after pinned setup, before any deploy.
		for _, artifact := range planFile.Artifacts {
			if _, err := safeArtifactPath(sourceRoot, artifact.Path); err != nil {
				return err
			}
		}
	}

	names := make([]string, len(pending))
	for i, index := range pending {
		a := planFile.Artifacts[index]
		names[i] = fmt.Sprintf("%s -> %s", a.Name, a.Target)
	}
	pt := NewProgressTracker(p, fmt.Sprintf("Deploying %d artifact(s)", len(pending)), names)
	if opts.Verbose {
		pt.UsePlainOutput()
	}
	pt.Start()
	deployCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var mu sync.Mutex
	var persistenceErr error
	deployed := 0
	checkpointed := make([]bool, len(pending))
	errs := RunParallel(deployCtx, opts.Concurrency, len(pending), func(ctx context.Context, i int) error {
		index := pending[i]
		// WritePlan reads the entire artifact slice under mu, so copy under mu too.
		mu.Lock()
		artifact := planFile.Artifacts[index]
		mu.Unlock()
		if err := runSteps(ctx, pt, i, "deploy", artifact.Path, artifact.Vars, artifact.Steps); err != nil {
			return err
		}
		mu.Lock()
		defer mu.Unlock()
		if persistenceErr != nil {
			return fmt.Errorf("deployment status uncertain for %q after checkpoint failure: %w", artifact.Name, persistenceErr)
		}
		commit := planFile.Commit
		if artifact.Pinned && artifact.PinCommit != "" {
			commit = artifact.PinCommit
		}
		version := commit[:min(7, len(commit))]
		if artifact.Pinned {
			lockFile.UpdateArtifactPinned(planFile.Environment, artifact.Name, commit, artifact.Target, version)
		} else {
			lockFile.UpdateArtifact(planFile.Environment, artifact.Name, commit, artifact.Target, version)
		}
		// External deployment cannot be rolled back by restoring local files. Save
		// history first; a failed checkpoint requires manual status reconciliation.
		if err := lockFile.Save(lockPath); err != nil {
			persistenceErr = fmt.Errorf("deployment status uncertain for %q: error saving lock file: %w; inspect deployment before retrying", artifact.Name, err)
		} else {
			planFile.Artifacts[index].Completed = true
			if err := config.WritePlan(rootPath, planFile); err != nil {
				persistenceErr = fmt.Errorf("deployment status uncertain for %q: error checkpointing plan: %w; lock history saved, inspect deployment before retrying", artifact.Name, err)
			}
		}
		if persistenceErr != nil {
			cancel()
			pt.MarkFailed(i, persistenceErr, "")
			return persistenceErr
		}
		deployed++
		checkpointed[i] = true
		pt.MarkDone(i)
		return nil
	})
	var failures []string
	for i, err := range errs {
		// RunParallel may observe cancellation after a durable checkpoint.
		if checkpointed[i] {
			errs[i] = nil
			continue
		}
		if err != nil {
			failures = append(failures, planFile.Artifacts[pending[i]].Name)
			pt.MarkFailed(i, err, "")
		}
	}
	pt.Stop()
	parts := []string{p.SummaryDeployed(deployed)}
	if len(failures) > 0 {
		parts = append(parts, p.SummaryFailed(len(failures)))
	}
	if skipped := planFile.TotalSkips + len(planFile.Artifacts) - len(pending); skipped > 0 {
		parts = append(parts, p.SummarySkipped(skipped))
	}
	p.Summary(append(parts, p.dim("total "+formatDuration(pt.TotalElapsed())))...)
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("deployment failed for %s; plan retained with completed checkpoints: %w", strings.Join(failures, ", "), err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !opts.NoCommit {
		if err := commitLockFile(ctx, rootPath, lockPath, planFile.Artifacts, opts.GitRemote, opts.GitBranch); err != nil {
			return fmt.Errorf("deployment completed but Git publication failed; completed plan retained (no redeployment on retry): %w", err)
		}
		p.Printf("  %s\n", p.dim("Lock file committed with [skip ci]"))
	}
	if err := config.RemovePlan(rootPath); err != nil {
		return fmt.Errorf("error removing completed plan: %w", err)
	}
	return nil
}
