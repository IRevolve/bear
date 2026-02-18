package cmd

import (
	"bytes"
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
	ctx := context.Background()
	p := NewPrinter()

	rootPath := filepath.Dir(configPath)
	if rootPath == "." {
		rootPath, _ = os.Getwd()
	}

	// Read plan file — it must exist
	if !config.PlanExists(rootPath) {
		return fmt.Errorf("no plan found. Run 'bear plan' first")
	}

	planFile, err := config.ReadPlan(rootPath)
	if err != nil {
		return fmt.Errorf("error reading plan file: %w", err)
	}

	if len(planFile.Artifacts) == 0 {
		p.Println("Plan contains no artifacts to deploy.")
		config.RemovePlan(rootPath)
		return nil
	}

	// Check if HEAD has moved since plan was created
	currentCommit := internal.GetCurrentCommit(rootPath)
	if currentCommit != "" && planFile.Commit != "" && currentCommit != planFile.Commit {
		p.Warning(fmt.Sprintf("HEAD has moved since plan was created (plan: %s, current: %s)",
			planFile.Commit[:min(7, len(planFile.Commit))],
			currentCommit[:min(7, len(currentCommit))]))
		p.Blank()
	}

	deployVersion := planFile.Commit

	p.BearHeader("Apply")

	// Load lock file for updates
	lockPath := filepath.Join(rootPath, "bear.lock.yml")
	lockFile, err := config.LoadLock(lockPath)
	if err != nil {
		return fmt.Errorf("error loading lock file: %w", err)
	}

	// Deploy all artifacts in parallel
	p.PhaseHeader(fmt.Sprintf("Deploying %d artifact(s)", len(planFile.Artifacts)))

	type deployResult struct {
		name   string
		output string
		err    error
	}
	results := make([]deployResult, len(planFile.Artifacts))

	errs := RunParallel(ctx, opts.Concurrency, len(planFile.Artifacts), func(ctx context.Context, i int) error {
		artifact := planFile.Artifacts[i]
		var combinedOutput bytes.Buffer

		for _, step := range artifact.Steps {
			var stdout, stderr bytes.Buffer
			execErr := ExecuteStep(ctx, step.Run, artifact.Path, artifact.Vars, &stdout, &stderr)

			if opts.Verbose {
				combinedOutput.WriteString(fmt.Sprintf("  → %s\n", step.Name))
				combinedOutput.Write(stdout.Bytes())
				combinedOutput.Write(stderr.Bytes())
			}

			if execErr != nil {
				combinedOutput.Write(stdout.Bytes())
				combinedOutput.Write(stderr.Bytes())
				results[i] = deployResult{
					name:   artifact.Name,
					output: combinedOutput.String(),
					err:    fmt.Errorf("%s: %w", step.Name, execErr),
				}
				return execErr
			}
		}

		results[i] = deployResult{
			name:   artifact.Name,
			output: combinedOutput.String(),
		}
		return nil
	})

	// Print results in order and update lock file for successful deployments
	var failures []string
	deployed := 0
	for i, res := range results {
		artifact := planFile.Artifacts[i]
		if res.err != nil {
			p.FailureWithOutput(fmt.Sprintf("%s → %s — %s", res.name, artifact.Target, res.err), res.output)
			failures = append(failures, res.name)
		} else {
			p.Success(fmt.Sprintf("%s → %s", res.name, artifact.Target))
			if opts.Verbose && res.output != "" {
				p.ErrorBox(res.output)
			}
			deployed++

			// Update lock file
			version := deployVersion[:min(7, len(deployVersion))]
			if artifact.Pinned {
				pinCommit := artifact.PinCommit
				if pinCommit == "" {
					pinCommit = deployVersion
				}
				lockFile.UpdateArtifactPinned(artifact.Name, pinCommit, artifact.Target, version)
			} else {
				lockFile.UpdateArtifact(artifact.Name, deployVersion, artifact.Target, version)
			}
		}
	}

	failedErrs := CollectErrors(errs)
	if len(failedErrs) > 0 {
		p.Blank()
		p.Printf("  %s\n", p.red(fmt.Sprintf("Deployment failed for: %s", strings.Join(failures, ", "))))
	}

	// Save lock file (even if some failed, save successful ones)
	if deployed > 0 {
		if err := lockFile.Save(lockPath); err != nil {
			return fmt.Errorf("error saving lock file: %w", err)
		}
		p.Blank()
		p.Printf("  %s %s\n", p.dim("Lock file updated:"), p.dim(lockPath))

		// Auto-commit (default behavior, disabled with --no-commit)
		if !opts.NoCommit {
			var deployedArtifacts []config.PlanArtifact
			for i, a := range planFile.Artifacts {
				if results[i].err == nil {
					deployedArtifacts = append(deployedArtifacts, a)
				}
			}
			if err := commitLockFile(rootPath, lockPath, deployedArtifacts); err != nil {
				p.Warning(fmt.Sprintf("Failed to commit lock file: %v", err))
			} else {
				p.Printf("  %s\n", p.dim("Lock file committed with [skip ci]"))
			}
		}
	}

	// Remove plan file after apply
	config.RemovePlan(rootPath)

	// Summary
	parts := []string{}
	if deployed > 0 {
		parts = append(parts, p.SummaryDeployed(deployed))
	}
	if len(failures) > 0 {
		parts = append(parts, p.SummaryFailed(len(failures)))
	}
	if planFile.TotalSkips > 0 {
		parts = append(parts, p.SummarySkipped(planFile.TotalSkips))
	}
	p.Summary(parts...)

	if len(failedErrs) > 0 {
		return fmt.Errorf("deployment failed for %d artifact(s)", len(failedErrs))
	}

	return nil
}

// commitLockFile commits the lock file with [skip ci] to prevent CI loops
func commitLockFile(rootPath, lockPath string, deployed []config.PlanArtifact) error {
	var names []string
	for _, d := range deployed {
		names = append(names, d.Name)
	}
	msg := fmt.Sprintf("chore(bear): update lock file [skip ci]\n\nDeployed: %s", strings.Join(names, ", "))

	addCmd := exec.Command("git", "add", lockPath)
	addCmd.Dir = rootPath
	if err := addCmd.Run(); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}

	commitCmd := exec.Command("git", "commit", "-m", msg)
	commitCmd.Dir = rootPath
	if err := commitCmd.Run(); err != nil {
		return fmt.Errorf("git commit failed: %w", err)
	}

	pushCmd := exec.Command("git", "push")
	pushCmd.Dir = rootPath
	if err := pushCmd.Run(); err != nil {
		return fmt.Errorf("git push failed: %w", err)
	}

	return nil
}
