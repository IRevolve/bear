package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/irevolve/bear/internal"
	"github.com/irevolve/bear/internal/config"
)

// noValidationSteps labels an artifact whose language configures nothing to
// run. Reporting it keeps a silently unvalidated artifact visible without
// turning a deliberate configuration into a failure.
const noValidationSteps = "no validation steps"

// ValidateWithOptions runs every selected artifact's validation steps against
// the current working tree. It is deliberately free of deployment semantics: no
// environment policy, no change detection, no deployment history, no plan file,
// no lock file and no Git. That makes it safe for merge-request CI, including a
// shallow clone or an export that is not a repository at all.
func ValidateWithOptions(configPath string, opts Options) (retErr error) {
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
	// The config is read-only and nothing in bear rewrites it, so it is safe to
	// read before taking the workspace lock. Reading it first lets an undeclared
	// environment be rejected before the workspace directory is created.
	cfg, err := internal.Load(configPath)
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}
	// An environment only names the variable injected into the steps. Reject an
	// undeclared one before touching the workspace so a typo has no side effects.
	if opts.Environment != "" {
		if err := config.ValidateEnvironmentIn(cfg, opts.Environment); err != nil {
			return err
		}
	}
	// Validation reads the working tree while a plan or apply may be rewriting
	// it, so it takes the workspace lock. It needs no repository lock: it runs
	// no Git and publishes nothing.
	release, err := acquireWorkspaceLock(rootPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := release(); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("release workspace lock: %w", err))
		}
	}()

	graph, err := internal.LoadGraph(rootPath, cfg)
	if err != nil {
		return fmt.Errorf("error loading artifacts: %w", err)
	}
	artifacts, err := selectArtifacts(graph, opts.Artifacts)
	if err != nil {
		return err
	}

	names := make([]string, len(artifacts))
	paths := make([]string, len(artifacts))
	vars := make([]map[string]string, len(artifacts))
	steps := make([][]config.Step, len(artifacts))
	for i, artifact := range artifacts {
		names[i] = artifact.Artifact.Name
		paths[i] = graph.Paths[names[i]]
		// Same precedence as a plan's validation, so a step behaves identically
		// in a merge request and in a deployment.
		vars[i] = mergeVars(cfg, artifact.Artifact.Target, artifact.Language, artifact.Artifact.Vars, opts.Environment)
		steps[i] = internal.ValidationSteps(cfg, artifact.Language)
	}

	p.BearHeader("Validate")
	p.PhaseHeader("Validating " + plural(len(artifacts), "artifact", "artifacts"))

	pt := NewProgressTracker(p, names)
	pt.SetOperation("validate")
	if opts.Verbose {
		pt.UsePlainOutput()
	}
	pt.Start()

	errs := RunParallel(ctx, opts.Concurrency, len(artifacts), func(ctx context.Context, i int) error {
		pt.MarkRunning(i)
		if len(steps[i]) == 0 {
			pt.MarkStep(i, noValidationSteps, 1, 1)
			pt.MarkDone(i)
			return nil
		}
		var combinedOutput TailBuffer

		for stepIndex, step := range steps[i] {
			pt.MarkStep(i, step.Name, stepIndex+1, len(steps[i]))
			var output io.Writer = &combinedOutput
			if opts.Verbose {
				output = io.MultiWriter(&combinedOutput, pt.StepWriter(i, step.Name))
			}
			path, execErr := safeArtifactPath(rootPath, paths[i])
			if execErr == nil {
				execErr = ExecuteStep(ctx, step.Run, path, vars[i], output, output)
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

	var failed []summaryEntry
	for i, err := range errs {
		if err != nil {
			pt.MarkFailed(i, err, "")
			failed = append(failed, summaryEntry{Name: names[i], Path: paths[i], Reason: err.Error()})
		}
	}
	pt.Stop()

	// A passing run says so in one sentence. A failing one reopens the summary
	// with the failures only, the way apply reports a failed deployment.
	if len(failed) > 0 {
		p.Blank()
		printEnvironmentSummary(p, summaryHeader{}, summarySection{Label: "failed", Color: p.red, Entries: failed})
		counts := []string{
			plural(len(artifacts)-len(failed), "passed", "passed"),
			plural(len(failed), "failed", "failed"),
		}
		printResult(p, p.red, "Validation failed", counts, pt.TotalElapsed())
		return fmt.Errorf("validation failed: %w", errors.Join(CollectErrors(errs)...))
	}

	printResult(p, p.green, "Validation complete", []string{plural(len(artifacts), "artifact", "artifacts")}, pt.TotalElapsed())
	return nil
}

// selectArtifacts narrows the graph to the named artifacts, in discovery order.
// Dependencies are never added: a merge request asks for exactly what it named.
// An unmatched name is an error, because a typo must fail CI rather than
// quietly validate nothing.
func selectArtifacts(graph *internal.Graph, names []string) ([]internal.DiscoveredArtifact, error) {
	if len(names) == 0 {
		return graph.Artifacts, nil
	}
	wanted := make(map[string]bool, len(names))
	for _, name := range names {
		wanted[name] = true
	}
	var selected []internal.DiscoveredArtifact
	for _, artifact := range graph.Artifacts {
		if wanted[artifact.Artifact.Name] {
			delete(wanted, artifact.Artifact.Name)
			selected = append(selected, artifact)
		}
	}
	if len(wanted) > 0 {
		missing := make([]string, 0, len(wanted))
		for name := range wanted {
			missing = append(missing, fmt.Sprintf("%q", name))
		}
		slices.Sort(missing)
		return nil, fmt.Errorf("unknown artifact %s", strings.Join(missing, ", "))
	}
	return selected, nil
}
