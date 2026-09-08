package internal

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/irevolve/bear/internal/config"
)

func historyConfig() *config.Config {
	return &config.Config{Targets: map[string]config.Target{"local": {Steps: []config.Step{{Name: "deploy", Run: "true"}}}}}
}

func historySave(t *testing.T, root string, lock *config.LockFile) {
	t.Helper()
	if err := lock.Save(filepath.Join(root, "bear.lock.yml")); err != nil {
		t.Fatal(err)
	}
}

func historyPlan(t *testing.T, root string, opts PlanOptions) *Plan {
	t.Helper()
	plan, err := CreatePlanWithOptions(root, historyConfig(), opts)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func historyDeploys(plan *Plan) string {
	var names []string
	for _, action := range plan.Actions {
		if action.Action == ActionDeploy {
			names = append(names, action.Artifact.Artifact.Name)
		}
	}
	return strings.Join(names, ",")
}

func TestPlannerConsumerHistoryAcrossCycles(t *testing.T) {
	for _, library := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled dependency", true: "library"}[library], func(t *testing.T) {
			root := detectorRepo(t)
			if library {
				detectorWrite(t, root, "source/bear.lib.yml", "name: source\n")
			} else {
				detectorWrite(t, root, "source/bear.artifact.yml", "name: source\ntarget: local\n")
			}
			detectorWrite(t, root, "middle/bear.lib.yml", "name: middle\ndepends: [source]\n")
			detectorWrite(t, root, "api/bear.artifact.yml", "name: api\ntarget: local\nenvironments: [dev, prd]\ndepends: [middle]\n")
			initial := detectorCommit(t, root)
			plan := historyPlan(t, root, PlanOptions{Environment: "dev"})
			if historyDeploys(plan) != "api" {
				t.Fatalf("initial deployment: %+v", plan)
			}
			// Simulate only successful deploy actions updating the lock.
			lock := plan.LockFile
			lock.UpdateArtifact("dev", "api", initial, "local", "")
			historySave(t, root, lock)
			for cycle := 0; cycle < 2; cycle++ {
				plan = historyPlan(t, root, PlanOptions{Environment: "dev"})
				if plan.ToDeploy != 0 {
					t.Fatalf("unchanged cycle %d redeployed consumer: %+v", cycle, plan)
				}
			}
			plan = historyPlan(t, root, PlanOptions{Environment: "prd"})
			if historyDeploys(plan) != "api" {
				t.Fatal("dev deployment suppressed first prd deployment")
			}
			detectorWrite(t, root, "source/code.txt", "dependency change\n")
			current := detectorCommit(t, root)
			// Filter does not discard dependency paths from change detection.
			plan = historyPlan(t, root, PlanOptions{Environment: "dev", Artifacts: []string{"api"}})
			if historyDeploys(plan) != "api" || plan.ToValidate != 1 {
				t.Fatalf("filtered transitive dependency not detected: %+v", plan)
			}
			for _, action := range plan.Actions {
				if len(action.ChangedFiles) != 1 || action.ChangedFiles[0] != "source/code.txt" {
					t.Fatalf("missing dependency evidence: %+v", action)
				}
			}
			lock.UpdateArtifact("dev", "api", current, "local", "")
			historySave(t, root, lock)
			if plan = historyPlan(t, root, PlanOptions{Environment: "dev"}); plan.ToDeploy != 0 {
				t.Fatalf("dependency with no history caused repeated deployment: %+v", plan)
			}
		})
	}
}

func TestPlannerDifferentConsumerBaselinesAndPins(t *testing.T) {
	root := detectorRepo(t)
	detectorWrite(t, root, "dep/bear.artifact.yml", "name: dep\ntarget: local\nenvironments: [dev]\n")
	for _, name := range []string{"new", "old"} {
		detectorWrite(t, root, name+"/bear.artifact.yml", "name: "+name+"\ntarget: local\nenvironments: [dev, prd]\ndepends: [dep]\n")
	}
	baseline := detectorCommit(t, root)
	detectorWrite(t, root, "dep/code.txt", "changed\n")
	current := detectorCommit(t, root)
	lock := &config.LockFile{}
	lock.UpdateArtifactPinned("dev", "dep", baseline, "local", "")
	lock.UpdateArtifact("dev", "old", baseline, "local", "")
	lock.UpdateArtifact("dev", "new", current, "local", "")
	historySave(t, root, lock)
	plan := historyPlan(t, root, PlanOptions{Environment: "dev"})
	if historyDeploys(plan) != "old" {
		t.Fatalf("consumer baseline/pinned dependency: %+v", plan)
	}
	// A pinned consumer cannot be promoted by changed dependencies.
	lock.UpdateArtifactPinned("dev", "old", baseline, "local", "")
	historySave(t, root, lock)
	if plan = historyPlan(t, root, PlanOptions{Environment: "dev"}); plan.ToDeploy != 0 {
		t.Fatalf("dependency overrode consumer pin: %+v", plan)
	}
	// Force must deploy even when the pinned source equals HEAD, without mutating history.
	lock.UpdateArtifactPinned("dev", "old", current, "local", "")
	historySave(t, root, lock)
	plan = historyPlan(t, root, PlanOptions{Environment: "dev", Force: true, Artifacts: []string{"old"}})
	if historyDeploys(plan) != "old" || !plan.LockFile.IsPinned("dev", "old") {
		t.Fatalf("force should schedule, not clear pin: %+v", plan)
	}
	lock.UpdateArtifactPinned("prd", "dep", current, "local", "")
	historySave(t, root, lock)
	plan = historyPlan(t, root, PlanOptions{Environment: "prd", Force: true, Artifacts: []string{"dep"}})
	if plan.ToDeploy != 0 || !plan.LockFile.IsPinned("prd", "dep") {
		t.Fatalf("force bypassed environment gate or cleared pin: %+v", plan)
	}
}

func TestPlannerRootAndNestedRename(t *testing.T) {
	for _, nested := range []bool{false, true} {
		t.Run(map[bool]string{false: "root", true: "nested"}[nested], func(t *testing.T) {
			root := detectorRepo(t)
			workspace := root
			if nested {
				workspace = filepath.Join(root, "workspace")
			}
			detectorWrite(t, workspace, "bear.artifact.yml", "name: root\ntarget: local\nenvironments: [dev]\n")
			detectorWrite(t, workspace, "source/bear.artifact.yml", "name: source\ntarget: local\nenvironments: [dev]\n")
			detectorWrite(t, workspace, "dest/bear.artifact.yml", "name: dest\ntarget: local\nenvironments: [dev]\n")
			detectorWrite(t, workspace, "source/name with\ttab-\u00e9.txt", "rename me\n")
			baseline := detectorCommit(t, root)
			lock := &config.LockFile{}
			for _, name := range []string{"root", "source", "dest"} {
				lock.UpdateArtifact("dev", name, baseline, "local", "")
			}
			historySave(t, workspace, lock)
			detectorGit(t, workspace, "mv", "source/name with\ttab-\u00e9.txt", "dest/new name.txt")
			detectorCommit(t, root)
			plan := historyPlan(t, workspace, PlanOptions{Environment: "dev"})
			if plan.ToDeploy != 3 {
				t.Fatalf("rename must affect root, source, and destination: %+v", plan)
			}
			// Changes outside a nested workspace must not affect its root artifact.
			if nested {
				current := GetCurrentCommit(root)
				for _, name := range []string{"root", "source", "dest"} {
					lock.UpdateArtifact("dev", name, current, "local", "")
				}
				historySave(t, workspace, lock)
				detectorWrite(t, root, "outside.txt", "unrelated\n")
				detectorCommit(t, root)
				if plan = historyPlan(t, workspace, PlanOptions{Environment: "dev"}); plan.ToDeploy != 0 {
					t.Fatalf("outside changes affected nested workspace: %+v", plan)
				}
			}
		})
	}
}

func TestPlannerErrors(t *testing.T) {
	for _, tc := range []struct {
		name, second, want string
	}{
		{"missing", "name: second\ndepends: [missing]\n", "missing"},
		{"cycle", "name: second\ndepends: [first]\n", "cycl"},
		{"duplicate", "name: first\n", "duplicate"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := detectorRepo(t)
			detectorWrite(t, root, "first/bear.lib.yml", "name: first\ndepends: [second]\n")
			detectorWrite(t, root, "second/bear.lib.yml", tc.second)
			detectorCommit(t, root)
			_, err := CreatePlanWithOptions(root, historyConfig(), PlanOptions{Environment: "dev", Artifacts: []string{"unselected"}})
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Fatalf("expected %s error before filtering, got %v", tc.want, err)
			}
		})
	}
	root := t.TempDir()
	detectorWrite(t, root, "bear.artifact.yml", "name: api\ntarget: local\nenvironments: [dev]\n")
	if _, err := CreatePlanWithOptions(root, historyConfig(), PlanOptions{Environment: "dev"}); err == nil {
		t.Fatal("plan without Git repository succeeded")
	}
	root = detectorRepo(t)
	detectorWrite(t, root, "api/bear.artifact.yml", "name: api\ntarget: local\nenvironments: [dev]\n")
	detectorCommit(t, root)
	lock := &config.LockFile{}
	lock.UpdateArtifact("dev", "api", "missing-history", "local", "")
	historySave(t, root, lock)
	if _, err := CreatePlanWithOptions(root, historyConfig(), PlanOptions{Environment: "dev"}); err == nil {
		t.Fatal("invalid deployment baseline was silently accepted")
	}
	if _, err := CreatePlanWithOptions(root, historyConfig(), PlanOptions{Environment: "dev", PinCommit: "missing-pin"}); err == nil {
		t.Fatal("invalid pin was silently accepted")
	}
}

func TestPlannerRejectsInvalidTargetsBeforeSelection(t *testing.T) {
	for _, target := range []struct {
		name, yaml, want string
	}{
		{"missing", "", "nonblank name and target"},
		{"empty", "target: ''\n", "nonblank name and target"},
		{"blank", "target: '  '\n", "nonblank name and target"},
		{"unknown", "target: nonexistent\n", "references unknown target"},
	} {
		for _, policy := range []struct{ name, yaml string }{
			{"enabled", "environments: [dev]\n"},
			{"disabled", "environments: [prd]\n"},
			{"absent", ""},
		} {
			t.Run(target.name+"/"+policy.name, func(t *testing.T) {
				root := detectorRepo(t)
				detectorWrite(t, root, "invalid/bear.artifact.yml", "name: invalid\n"+target.yaml+policy.yaml)
				detectorWrite(t, root, "valid/bear.artifact.yml", "name: valid\ntarget: local\nenvironments: [dev]\n")
				commit := detectorCommit(t, root)
				for _, mode := range []struct {
					name string
					opts PlanOptions
				}{
					{"normal", PlanOptions{Environment: "dev"}},
					{"pin", PlanOptions{Environment: "dev", PinCommit: commit}},
					{"filtered", PlanOptions{Environment: "dev", Artifacts: []string{"valid"}}},
					{"filtered pin", PlanOptions{Environment: "dev", Artifacts: []string{"valid"}, PinCommit: commit}},
				} {
					t.Run(mode.name, func(t *testing.T) {
						plan, err := CreatePlanWithOptions(root, historyConfig(), mode.opts)
						if err == nil || !strings.Contains(err.Error(), target.want) || plan != nil {
							t.Fatalf("expected target error %q and no plan, got %+v, %v", target.want, plan, err)
						}
					})
				}
			})
		}
	}
}

func TestPlannerLibraryNeedsNoTarget(t *testing.T) {
	root := detectorRepo(t)
	detectorWrite(t, root, "bear.lib.yml", "name: library\n")
	commit := detectorCommit(t, root)
	for _, pin := range []string{"", commit} {
		plan, err := CreatePlanWithOptions(root, &config.Config{}, PlanOptions{Environment: "dev", PinCommit: pin})
		if err != nil {
			t.Fatal(err)
		}
		if plan.ToValidate != 1 || plan.ToDeploy != 0 {
			t.Fatalf("library should remain validation-only: %+v", plan)
		}
	}
}

func TestPlannerRootIgnoresBearStateAcrossCycles(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(map[bool]string{false: "no commit", true: "committed"}[committed], func(t *testing.T) {
			root := detectorRepo(t)
			detectorWrite(t, root, ".gitignore", "")
			detectorWrite(t, root, "bear.artifact.yml", "name: root\ntarget: local\nenvironments: [dev]\n")
			sourceCommit := detectorCommit(t, root)
			plan := historyPlan(t, root, PlanOptions{Environment: "dev"})
			if historyDeploys(plan) != "root" {
				t.Fatalf("expected initial root deployment: %+v", plan)
			}
			lock := plan.LockFile
			for cycle := 0; cycle < 2; cycle++ {
				lock.UpdateArtifact("dev", "root", sourceCommit, "local", fmt.Sprintf("v%d", cycle))
				historySave(t, root, lock)
				if committed {
					// Keep the deployment baseline at the source commit, not the lock commit.
					detectorCommit(t, root)
				}
				plan = historyPlan(t, root, PlanOptions{Environment: "dev"})
				if plan.ToDeploy != 0 || plan.ToValidate != 0 || plan.TotalChanges != 0 {
					t.Fatalf("cycle %d lock update scheduled work: %+v", cycle, plan)
				}
				for _, path := range []string{".bear/marker", "nested/.bear/marker", "nested/bear.lock.yml"} {
					detectorWrite(t, root, path, fmt.Sprintf("state cycle %d\n", cycle))
				}
				if committed {
					detectorCommit(t, root)
				}
				plan = historyPlan(t, root, PlanOptions{Environment: "dev"})
				if plan.ToDeploy != 0 || plan.ToValidate != 0 || plan.TotalChanges != 0 {
					t.Fatalf("cycle %d Bear markers scheduled work: %+v", cycle, plan)
				}
			}
			detectorWrite(t, root, "source.go", "real source change\n")
			if committed {
				detectorCommit(t, root)
			}
			plan = historyPlan(t, root, PlanOptions{Environment: "dev"})
			if historyDeploys(plan) != "root" || plan.TotalChanges != 1 {
				t.Fatalf("real source change was suppressed: %+v", plan)
			}
		})
	}
}
