package internal

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/irevolve/bear/internal/config"
)

// ActionType defines the type of action
type ActionType string

const (
	ActionValidate ActionType = "validate" // Lint, Test, Build
	ActionDeploy   ActionType = "deploy"   // Deployment
	ActionSkip     ActionType = "skip"     // No changes
)

// PlannedAction represents a planned action
type PlannedAction struct {
	Artifact     DiscoveredArtifact
	Action       ActionType
	Reason       string
	Steps        []config.Step
	ChangedFiles []string
	PinCommit    string // If set, this commit will be deployed (pin)
}

// Plan contains all planned actions
type Plan struct {
	Environment  string
	Actions      []PlannedAction
	TotalChanges int
	ToValidate   int
	ToDeploy     int
	ToSkip       int
	LockFile     *config.LockFile
	LockPath     string
}

// PlanOptions contains options for plan creation
type PlanOptions struct {
	Environment string   // Explicit deployment environment; never inferred
	Artifacts   []string // Only consider these artifacts
	PinCommit   string   // Pin to this commit
	Force       bool     // Ignore pinned artifacts
}

// getValidationSteps returns all validation steps for a given language
func getValidationSteps(cfg *config.Config, language string) []config.Step {
	if lang, ok := cfg.Languages[language]; ok {
		return lang.Steps
	}
	return nil
}

// CreatePlanWithOptions creates a plan with extended options
func CreatePlanWithOptions(rootPath string, cfg *config.Config, opts PlanOptions) (*Plan, error) {
	if err := config.ValidateEnvironment(opts.Environment); err != nil {
		return nil, err
	}
	// Load lock file
	lockPath := filepath.Join(rootPath, "bear.lock.yml")
	lockFile, err := config.LoadLock(lockPath)
	if err != nil {
		return nil, err
	}

	// Scan all artifacts
	artifacts, err := ScanArtifacts(rootPath, cfg)
	if err != nil {
		return nil, err
	}

	// Build and validate the full graph before selecting output artifacts.
	byName := make(map[string]DiscoveredArtifact, len(artifacts))
	paths := make(map[string]string, len(artifacts))
	for _, artifact := range artifacts {
		name := artifact.Artifact.Name
		if !artifact.Artifact.IsLib {
			target := artifact.Artifact.Target
			if strings.TrimSpace(target) == "" {
				return nil, fmt.Errorf("artifact %q has no target defined", name)
			}
			if _, exists := cfg.Targets[target]; !exists {
				return nil, fmt.Errorf("artifact %q references unknown target %q", name, target)
			}
		}
		if _, exists := byName[name]; exists {
			return nil, fmt.Errorf("duplicate artifact %q", name)
		}
		byName[name] = artifact
		path, err := filepath.Rel(rootPath, artifact.Path)
		if err != nil {
			return nil, err
		}
		paths[name] = filepath.ToSlash(path)
	}
	closures := make(map[string][]string, len(artifacts))
	visiting := make(map[string]bool)
	var visit func(string) ([]string, error)
	visit = func(name string) ([]string, error) {
		if visiting[name] {
			return nil, fmt.Errorf("dependency cycle involving %q", name)
		}
		if closure, ok := closures[name]; ok {
			return closure, nil
		}
		artifact, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("missing dependency %q", name)
		}
		visiting[name] = true
		closure := []string{name}
		seen := map[string]bool{name: true}
		for _, dep := range artifact.Artifact.Depends {
			dependencies, err := visit(dep)
			if err != nil {
				return nil, err
			}
			for _, dependency := range dependencies {
				if !seen[dependency] {
					seen[dependency] = true
					closure = append(closure, dependency)
				}
			}
		}
		visiting[name] = false
		closures[name] = closure
		return closure, nil
	}
	for _, artifact := range artifacts {
		if _, err := visit(artifact.Artifact.Name); err != nil {
			return nil, err
		}
	}
	artifacts = filterArtifacts(artifacts, opts.Artifacts)

	currentCommit, err := ResolveCommit(rootPath, "HEAD")
	if err != nil {
		return nil, err
	}

	// Pin mode: Deploy all targeted artifacts to specific commit
	if opts.PinCommit != "" {
		pinCommit, err := ResolveCommit(rootPath, opts.PinCommit)
		if err != nil {
			return nil, err
		}
		plan := createPinPlan(artifacts, cfg, lockFile, lockPath, pinCommit)
		plan.applyEnvironment(opts.Environment)
		return plan, nil
	}

	// Get uncommitted/untracked changes (same for all artifacts)
	uncommittedFiles, err := GetUncommittedChanges(rootPath)
	if err != nil {
		return nil, err
	}
	plan := &Plan{
		LockFile: lockFile,
		LockPath: lockPath,
	}
	diffs := make(map[string][]ChangedFile)
	changedPaths := make(map[string]bool)
	for _, file := range uncommittedFiles {
		changedPaths[file.Path] = true
	}

	for _, artifact := range artifacts {
		name := artifact.Artifact.Name
		relPath := paths[name]

		// Check if artifact is pinned (e.g. after rollback)
		// --force ignores pins
		pinned := lockFile.IsPinned(opts.Environment, name)
		if !opts.Force && pinned {
			plan.Actions = append(plan.Actions, PlannedAction{
				Artifact: artifact,
				Action:   ActionSkip,
				Reason:   "pinned (use --force to override)",
			})
			plan.ToSkip++
			continue
		}

		// Every dependency is compared against the consumer's deployment baseline,
		// never against its own (possibly absent or newer) deployment history.
		changes := append([]ChangedFile(nil), uncommittedFiles...)
		lastDeployed := lockFile.GetLastDeployedCommit(opts.Environment, name)
		if lastDeployed != "" && lastDeployed != currentCommit {
			commitChanges, cached := diffs[lastDeployed]
			if !cached {
				commitChanges, err = GetChangedFilesBetweenCommits(rootPath, lastDeployed, currentCommit)
				if err != nil {
					return nil, fmt.Errorf("artifact %q deployment history in %s: %w", name, opts.Environment, err)
				}
				diffs[lastDeployed] = commitChanges
			}
			changes = append(changes, commitChanges...)
		}
		affected := lastDeployed == "" || (opts.Force && pinned)
		reason := "files changed"
		var files []string
		seen := make(map[string]bool)
		for _, dependency := range closures[name] {
			hit, matches := isArtifactAffected(paths[dependency], changes)
			if hit {
				affected = true
				if dependency != name {
					reason = "dependency '" + dependency + "' changed"
				}
			}
			for _, file := range matches {
				changedPaths[file] = true
				if !seen[file] {
					seen[file] = true
					files = append(files, file)
				}
			}
		}
		if lastDeployed == "" {
			reason = "new artifact"
			if len(files) == 0 {
				files = []string{relPath + " (new artifact)"}
			}
		} else if opts.Force && pinned {
			reason = "pinned (forced deployment)"
		}

		if affected {
			// Find the validation steps for the language
			validationSteps := getValidationSteps(cfg, artifact.Language)

			// Find deploy steps from target (only for non-libraries)
			var deploySteps []config.Step
			if !artifact.Artifact.IsLib {
				if t, ok := cfg.Targets[artifact.Artifact.Target]; ok {
					deploySteps = t.Steps
				}
			}

			plan.Actions = append(plan.Actions, PlannedAction{
				Artifact:     artifact,
				Action:       ActionValidate,
				Reason:       reason,
				Steps:        validationSteps,
				ChangedFiles: files,
			})
			plan.ToValidate++

			// If it's a deployable artifact (not a library), add deploy action
			if !artifact.Artifact.IsLib && len(deploySteps) > 0 {
				plan.Actions = append(plan.Actions, PlannedAction{
					Artifact:     artifact,
					Action:       ActionDeploy,
					Reason:       reason,
					Steps:        deploySteps,
					ChangedFiles: files,
				})
				plan.ToDeploy++
			}
		} else {
			plan.Actions = append(plan.Actions, PlannedAction{
				Artifact: artifact,
				Action:   ActionSkip,
				Reason:   "no changes detected",
			})
			plan.ToSkip++
		}
	}

	plan.TotalChanges = len(changedPaths)
	plan.applyEnvironment(opts.Environment)

	return plan, nil
}

// Apply the deployment gate only after change detection or pin creation.
func (p *Plan) applyEnvironment(environment string) {
	p.Environment = environment
	for i := range p.Actions {
		action := &p.Actions[i]
		if action.Action != ActionDeploy {
			continue
		}
		environments := action.Artifact.Artifact.Environments
		if slices.Contains(environments, environment) {
			continue
		}
		action.Action = ActionSkip
		action.Reason = fmt.Sprintf("deployment not enabled for environment %s", environment)
		if len(environments) == 0 {
			action.Reason = "no deployment environments configured"
		}
		action.Steps = nil
		p.ToDeploy--
		p.ToSkip++
	}
}

func isArtifactAffected(artifactPath string, changedFiles []ChangedFile) (bool, []string) {
	var affected []string
	artifactPath = filepath.ToSlash(filepath.Clean(artifactPath))

	for _, f := range changedFiles {
		if artifactPath == "." || strings.HasPrefix(f.Path, artifactPath+"/") || f.Path == artifactPath {
			affected = append(affected, f.Path)
		}
	}

	return len(affected) > 0, affected
}

// filterArtifacts filters artifacts by the specified names
func filterArtifacts(artifacts []DiscoveredArtifact, names []string) []DiscoveredArtifact {
	if len(names) == 0 {
		return artifacts
	}

	nameMap := make(map[string]bool)
	for _, n := range names {
		nameMap[n] = true
	}

	var filtered []DiscoveredArtifact
	for _, a := range artifacts {
		if nameMap[a.Artifact.Name] {
			filtered = append(filtered, a)
		}
	}

	return filtered
}

// createPinPlan creates a plan for pinning artifacts to a specific commit
func createPinPlan(artifacts []DiscoveredArtifact, cfg *config.Config, lockFile *config.LockFile, lockPath string, pinCommit string) *Plan {
	plan := &Plan{
		LockFile: lockFile,
		LockPath: lockPath,
	}

	shortCommit := pinCommit
	if len(shortCommit) > 8 {
		shortCommit = shortCommit[:8]
	}

	for _, artifact := range artifacts {
		// Find the validation steps for the language
		validationSteps := getValidationSteps(cfg, artifact.Language)

		// Validation action
		plan.Actions = append(plan.Actions, PlannedAction{
			Artifact:  artifact,
			Action:    ActionValidate,
			Reason:    "pin to " + shortCommit,
			Steps:     validationSteps,
			PinCommit: pinCommit,
		})
		plan.ToValidate++

		// Deploy action only for non-libraries
		if !artifact.Artifact.IsLib {
			var deploySteps []config.Step
			if t, ok := cfg.Targets[artifact.Artifact.Target]; ok {
				deploySteps = t.Steps
			}

			if len(deploySteps) > 0 {
				plan.Actions = append(plan.Actions, PlannedAction{
					Artifact:  artifact,
					Action:    ActionDeploy,
					Reason:    "pin to " + shortCommit,
					Steps:     deploySteps,
					PinCommit: pinCommit,
				})
				plan.ToDeploy++
			}
		}
	}

	return plan
}
