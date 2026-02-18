package internal

import (
	"os/exec"
	"path/filepath"
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
	Artifacts []string // Only consider these artifacts
	PinCommit string   // Pin to this commit
	Force     bool     // Ignore pinned artifacts
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

	// Filter artifacts if Artifacts are specified
	if len(opts.Artifacts) > 0 {
		artifacts = filterArtifacts(artifacts, opts.Artifacts)
	}

	// Pin mode: Deploy all targeted artifacts to specific commit
	if opts.PinCommit != "" {
		return createPinPlan(artifacts, cfg, lockFile, lockPath, opts.PinCommit), nil
	}

	// Get current commit
	currentCommit := GetCurrentCommit(rootPath)

	// Get uncommitted/untracked changes (same for all artifacts)
	uncommittedFiles, _ := GetUncommittedChanges(rootPath)
	plan := &Plan{
		TotalChanges: len(uncommittedFiles),
		LockFile:     lockFile,
		LockPath:     lockPath,
	}

	for _, artifact := range artifacts {
		relPath, _ := filepath.Rel(rootPath, artifact.Path)

		// Check if artifact is pinned (e.g. after rollback)
		// --force ignores pins
		if !opts.Force && lockFile.IsPinned(artifact.Artifact.Name) {
			plan.Actions = append(plan.Actions, PlannedAction{
				Artifact: artifact,
				Action:   ActionSkip,
				Reason:   "pinned (use --force to override)",
			})
			plan.ToSkip++
			continue
		}

		// 1. Check uncommitted changes
		affected, files := isArtifactAffected(relPath, uncommittedFiles)

		// 2. Check changes since last deployment
		lastDeployed := lockFile.GetLastDeployedCommit(artifact.Artifact.Name)
		if lastDeployed != "" && lastDeployed != currentCommit {
			// Get changes between lastDeployed and current HEAD
			commitChanges, err := GetChangedFilesBetweenCommits(rootPath, lastDeployed, "HEAD")
			if err != nil {
				// Commit not found (e.g. fictitious commit in lock file) - mark as changed
				affected = true
				files = append(files, relPath+" (deployed commit not found)")
			} else {
				commitAffected, commitFiles := isArtifactAffected(relPath, commitChanges)
				if commitAffected {
					affected = true
					files = append(files, commitFiles...)
					plan.TotalChanges += len(commitChanges)
				}
			}
		} else if lastDeployed == "" {
			// Never deployed - mark as changed if staged/committed
			cmd := exec.Command("git", "ls-files", relPath)
			cmd.Dir = rootPath
			if output, err := cmd.Output(); err == nil && len(output) > 0 {
				affected = true
				files = []string{relPath + " (new artifact)"}
			}
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
				Reason:       "files changed",
				Steps:        validationSteps,
				ChangedFiles: files,
			})
			plan.ToValidate++

			// If it's a deployable artifact (not a library), add deploy action
			if !artifact.Artifact.IsLib && len(deploySteps) > 0 {
				plan.Actions = append(plan.Actions, PlannedAction{
					Artifact:     artifact,
					Action:       ActionDeploy,
					Reason:       "artifact changed",
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

	// Add dependent artifacts
	plan.addDependentArtifacts(artifacts, cfg)

	return plan, nil
}

func isArtifactAffected(artifactPath string, changedFiles []ChangedFile) (bool, []string) {
	var affected []string

	for _, f := range changedFiles {
		if strings.HasPrefix(f.Path, artifactPath+"/") || f.Path == artifactPath {
			affected = append(affected, f.Path)
		}
	}

	return len(affected) > 0, affected
}

func (p *Plan) addDependentArtifacts(artifacts []DiscoveredArtifact, cfg *config.Config) {
	// Collect names of changed artifacts (validated or deployed)
	changedNames := make(map[string]bool)
	for _, action := range p.Actions {
		if action.Action == ActionDeploy || action.Action == ActionValidate {
			changedNames[action.Artifact.Artifact.Name] = true
		}
	}

	// Iterate multiple times to find transitive dependencies
	changed := true
	for changed {
		changed = false
		for i, action := range p.Actions {
			if action.Action == ActionSkip {
				for _, dep := range action.Artifact.Artifact.Depends {
					if changedNames[dep] {
						// Find validation steps for the language
						validationSteps := getValidationSteps(cfg, action.Artifact.Language)

						p.Actions[i].Action = ActionValidate
						p.Actions[i].Reason = "dependency '" + dep + "' changed"
						p.Actions[i].Steps = validationSteps
						p.ToSkip--
						p.ToValidate++

						// Add deploy action (only for non-libraries)
						if !action.Artifact.Artifact.IsLib {
							if t, ok := cfg.Targets[action.Artifact.Artifact.Target]; ok {
								p.Actions = append(p.Actions, PlannedAction{
									Artifact: action.Artifact,
									Action:   ActionDeploy,
									Reason:   "dependency '" + dep + "' changed",
									Steps:    t.Steps,
								})
								p.ToDeploy++
							}
						}

						changedNames[action.Artifact.Artifact.Name] = true
						changed = true
						break
					}
				}
			}
		}
	}
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
