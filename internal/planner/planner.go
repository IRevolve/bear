package planner

import (
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/IRevolve/Bear/internal/config"
	"github.com/IRevolve/Bear/internal/detector"
	"github.com/IRevolve/Bear/internal/scanner"
)

// ActionType definiert die Art der Aktion
type ActionType string

const (
	ActionValidate ActionType = "validate" // Lint, Test, Build
	ActionDeploy   ActionType = "deploy"   // Deployment
	ActionSkip     ActionType = "skip"     // Keine Änderungen
)

// PlannedAction repräsentiert eine geplante Aktion
type PlannedAction struct {
	Artifact       scanner.DiscoveredArtifact
	Action         ActionType
	Reason         string
	Steps          []config.Step
	ChangedFiles   []string
	RollbackCommit string // Wenn gesetzt, wird dieser Commit deployed (Rollback)
}

// Plan enthält alle geplanten Aktionen
type Plan struct {
	Actions      []PlannedAction
	TotalChanges int
	ToValidate   int
	ToDeploy     int
	ToSkip       int
	LockFile     *config.LockFile
	LockPath     string
}

// PlanOptions enthält Optionen für die Plan-Erstellung
type PlanOptions struct {
	Artifacts      []string // Nur diese Artefakte berücksichtigen
	RollbackCommit string   // Rollback zu diesem Commit
}

// CreatePlan erstellt einen Ausführungsplan basierend auf Änderungen
func CreatePlan(rootPath string, cfg *config.Config) (*Plan, error) {
	return CreatePlanWithOptions(rootPath, cfg, PlanOptions{})
}

// CreatePlanWithOptions erstellt einen Plan mit erweiterten Optionen
func CreatePlanWithOptions(rootPath string, cfg *config.Config, opts PlanOptions) (*Plan, error) {
	// Lade Lock-Datei
	lockPath := filepath.Join(rootPath, "bear.lock.yml")
	lockFile, err := config.LoadLock(lockPath)
	if err != nil {
		return nil, err
	}

	// Scanne alle Artefakte
	artifacts, err := scanner.ScanArtifacts(rootPath, cfg)
	if err != nil {
		return nil, err
	}

	// Filtere Artefakte wenn Artifacts angegeben wurden
	if len(opts.Artifacts) > 0 {
		artifacts = filterArtifacts(artifacts, opts.Artifacts)
	}

	// Rollback-Modus: Alle targeted Artefakte deployen
	if opts.RollbackCommit != "" {
		return createRollbackPlan(artifacts, cfg, lockFile, lockPath, opts.RollbackCommit), nil
	}

	// Hole aktuellen Commit
	currentCommit := detector.GetCurrentCommit(rootPath)

	// Hole uncommitted/untracked changes (für alle Artifacts gleich)
	uncommittedFiles, _ := detector.GetUncommittedChanges(rootPath)
	uncommittedDirs := detector.GetAffectedDirs(uncommittedFiles)

	plan := &Plan{
		TotalChanges: len(uncommittedFiles),
		LockFile:     lockFile,
		LockPath:     lockPath,
	}

	for _, artifact := range artifacts {
		relPath, _ := filepath.Rel(rootPath, artifact.Path)

		// 1. Prüfe uncommitted changes
		affected, files := isArtifactAffected(relPath, uncommittedFiles, uncommittedDirs)

		// 2. Prüfe Änderungen seit dem letzten Deployment
		lastDeployed := lockFile.GetLastDeployedCommit(artifact.Artifact.Name)
		if lastDeployed != "" && lastDeployed != currentCommit {
			// Hole Änderungen zwischen lastDeployed und aktuellem HEAD
			commitChanges, err := detector.GetChangedFilesBetweenCommits(rootPath, lastDeployed, "HEAD")
			if err != nil {
				// Commit nicht gefunden (z.B. fiktiver Commit in Lock-Datei) - als geändert markieren
				affected = true
				files = append(files, relPath+" (deployed commit not found)")
			} else {
				commitAffected, commitFiles := isArtifactAffected(relPath, commitChanges, detector.GetAffectedDirs(commitChanges))
				if commitAffected {
					affected = true
					files = append(files, commitFiles...)
					plan.TotalChanges += len(commitChanges)
				}
			}
		} else if lastDeployed == "" {
			// Noch nie deployed - als geändert markieren wenn staged/committed
			cmd := exec.Command("git", "ls-files", relPath)
			cmd.Dir = rootPath
			if output, err := cmd.Output(); err == nil && len(output) > 0 {
				affected = true
				files = []string{relPath + " (new artifact)"}
			}
		}

		if affected {
			// Finde die Validation-Steps für die Sprache
			var validationSteps []config.Step
			for _, lang := range cfg.Languages {
				if lang.Name == artifact.Language {
					validationSteps = append(validationSteps, lang.Validation.Setup...)
					validationSteps = append(validationSteps, lang.Validation.Lint...)
					validationSteps = append(validationSteps, lang.Validation.Test...)
					validationSteps = append(validationSteps, lang.Validation.Build...)
					break
				}
			}

			// Finde Deploy-Steps vom Target (nur für Nicht-Libraries)
			var deploySteps []config.Step
			if !artifact.Artifact.IsLib {
				for _, t := range cfg.Targets {
					if t.Name == artifact.Artifact.Target {
						deploySteps = t.Deploy
						break
					}
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

			// Wenn es ein deploybares Artefakt ist (keine Library), füge Deploy-Aktion hinzu
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

	// Füge abhängige Artefakte hinzu
	plan.addDependentArtifacts(artifacts, cfg)

	return plan, nil
}

func isArtifactAffected(artifactPath string, changedFiles []detector.ChangedFile, affectedDirs map[string]bool) (bool, []string) {
	var affected []string

	for _, f := range changedFiles {
		if strings.HasPrefix(f.Path, artifactPath+"/") || f.Path == artifactPath {
			affected = append(affected, f.Path)
		}
	}

	return len(affected) > 0, affected
}

func createFullPlan(artifacts []scanner.DiscoveredArtifact, cfg *config.Config, reason string) *Plan {
	plan := &Plan{}

	for _, artifact := range artifacts {
		plan.Actions = append(plan.Actions, PlannedAction{
			Artifact: artifact,
			Action:   ActionValidate,
			Reason:   reason,
		})
		plan.ToValidate++
	}

	return plan
}

func createEmptyPlan(artifacts []scanner.DiscoveredArtifact) *Plan {
	plan := &Plan{}

	for _, artifact := range artifacts {
		plan.Actions = append(plan.Actions, PlannedAction{
			Artifact: artifact,
			Action:   ActionSkip,
			Reason:   "no changes detected",
		})
		plan.ToSkip++
	}

	return plan
}

func (p *Plan) addDependentArtifacts(artifacts []scanner.DiscoveredArtifact, cfg *config.Config) {
	// Sammle Namen der geänderten Artefakte (validiert oder deployed)
	changedNames := make(map[string]bool)
	for _, action := range p.Actions {
		if action.Action == ActionDeploy || action.Action == ActionValidate {
			changedNames[action.Artifact.Artifact.Name] = true
		}
	}

	// Iteriere mehrfach um transitive Dependencies zu finden
	changed := true
	for changed {
		changed = false
		for i, action := range p.Actions {
			if action.Action == ActionSkip {
				for _, dep := range action.Artifact.Artifact.DependsOn {
					if changedNames[dep] {
						// Finde Validation-Steps für die Sprache
						var validationSteps []config.Step
						for _, lang := range cfg.Languages {
							if lang.Name == action.Artifact.Language {
								validationSteps = append(validationSteps, lang.Validation.Setup...)
								validationSteps = append(validationSteps, lang.Validation.Lint...)
								validationSteps = append(validationSteps, lang.Validation.Test...)
								validationSteps = append(validationSteps, lang.Validation.Build...)
								break
							}
						}

						p.Actions[i].Action = ActionValidate
						p.Actions[i].Reason = "dependency '" + dep + "' changed"
						p.Actions[i].Steps = validationSteps
						p.ToSkip--
						p.ToValidate++

						// Füge Deploy-Aktion hinzu (nur für Nicht-Libraries)
						if !action.Artifact.Artifact.IsLib {
							for _, t := range cfg.Targets {
								if t.Name == action.Artifact.Artifact.Target {
									p.Actions = append(p.Actions, PlannedAction{
										Artifact: action.Artifact,
										Action:   ActionDeploy,
										Reason:   "dependency '" + dep + "' changed",
										Steps:    t.Deploy,
									})
									p.ToDeploy++
									break
								}
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

// filterArtifacts filtert Artefakte nach den angegebenen Namen
func filterArtifacts(artifacts []scanner.DiscoveredArtifact, targets []string) []scanner.DiscoveredArtifact {
	if len(targets) == 0 {
		return artifacts
	}

	targetMap := make(map[string]bool)
	for _, t := range targets {
		targetMap[t] = true
	}

	var filtered []scanner.DiscoveredArtifact
	for _, a := range artifacts {
		if targetMap[a.Artifact.Name] {
			filtered = append(filtered, a)
		}
	}

	return filtered
}

// createRollbackPlan erstellt einen Plan für ein Rollback auf einen bestimmten Commit
func createRollbackPlan(artifacts []scanner.DiscoveredArtifact, cfg *config.Config, lockFile *config.LockFile, lockPath string, rollbackCommit string) *Plan {
	plan := &Plan{
		LockFile: lockFile,
		LockPath: lockPath,
	}

	shortCommit := rollbackCommit
	if len(shortCommit) > 8 {
		shortCommit = shortCommit[:8]
	}

	for _, artifact := range artifacts {
		// Finde die Validation-Steps für die Sprache
		var validationSteps []config.Step
		for _, lang := range cfg.Languages {
			if lang.Name == artifact.Language {
				validationSteps = append(validationSteps, lang.Validation.Setup...)
				validationSteps = append(validationSteps, lang.Validation.Lint...)
				validationSteps = append(validationSteps, lang.Validation.Test...)
				validationSteps = append(validationSteps, lang.Validation.Build...)
				break
			}
		}

		// Validation-Aktion
		plan.Actions = append(plan.Actions, PlannedAction{
			Artifact:       artifact,
			Action:         ActionValidate,
			Reason:         "rollback to " + shortCommit,
			Steps:          validationSteps,
			RollbackCommit: rollbackCommit,
		})
		plan.ToValidate++

		// Deploy-Aktion nur für Nicht-Libraries
		if !artifact.Artifact.IsLib {
			var deploySteps []config.Step
			for _, t := range cfg.Targets {
				if t.Name == artifact.Artifact.Target {
					deploySteps = t.Deploy
					break
				}
			}

			if len(deploySteps) > 0 {
				plan.Actions = append(plan.Actions, PlannedAction{
					Artifact:       artifact,
					Action:         ActionDeploy,
					Reason:         "rollback to " + shortCommit,
					Steps:          deploySteps,
					RollbackCommit: rollbackCommit,
				})
				plan.ToDeploy++
			}
		}
	}

	return plan
}
