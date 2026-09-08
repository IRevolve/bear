package commands

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/irevolve/bear/internal/config"
	"github.com/spf13/pflag"
)

func TestPlanCommand(t *testing.T) {
	for _, tt := range []struct {
		name           string
		args           []string
		wantError      string
		preservePlan   bool
		missingConfig  bool
		validationOnly bool
		environment    string
		artifacts      []string
		pin            string
	}{
		{name: "no arguments", wantError: "requires at least 1 arg(s)", preservePlan: true},
		{name: "flags only", args: []string{"--concurrency", "1"}, wantError: "requires at least 1 arg(s)", preservePlan: true},
		{name: "invalid environment", args: []string{"production"}, wantError: `error creating plan: invalid environment "production"`},
		{name: "empty environment", args: []string{""}, wantError: `error creating plan: invalid environment ""`},
		{name: "artifact is not environment", args: []string{"api"}, wantError: `error creating plan: invalid environment "api"`},
		{name: "dev", args: []string{"dev"}, environment: "dev", artifacts: []string{"api", "web", "worker"}},
		{name: "int", args: []string{"int"}, environment: "int", artifacts: []string{"api", "web", "worker"}},
		{name: "prd", args: []string{"prd"}, environment: "prd", artifacts: []string{"api", "web", "worker"}},
		{name: "single artifact", args: []string{"dev", "web"}, environment: "dev", artifacts: []string{"web"}},
		{name: "multiple artifacts", args: []string{"int", "worker", "api"}, environment: "int", artifacts: []string{"api", "worker"}},
		{name: "interspersed flags", args: []string{"--concurrency", "1", "prd", "worker", "--pin", "HEAD~1", "api", "--force"}, environment: "prd", artifacts: []string{"api", "worker"}, pin: "HEAD~1"},
		{name: "removed flag", args: []string{"--environment", "dev"}, wantError: "unknown flag: --environment", preservePlan: true},
		{name: "removed flag after environment", args: []string{"dev", "--environment", "int"}, wantError: "unknown flag: --environment", preservePlan: true},
		{name: "missing config", args: []string{"dev"}, wantError: "error loading config", missingConfig: true},
		{name: "validation only", args: []string{"int"}, environment: "int", validationOnly: true},
		{name: "validation only missing environment", wantError: "requires at least 1 arg(s)", preservePlan: true, validationOnly: true},
		{name: "validation only empty environment", args: []string{""}, wantError: `error creating plan: invalid environment ""`, validationOnly: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, flags := range []*pflag.FlagSet{rootCmd.PersistentFlags(), planCmd.Flags()} {
				flags.VisitAll(func(flag *pflag.Flag) {
					value, changed := flag.Value.String(), flag.Changed
					t.Cleanup(func() {
						if err := flag.Value.Set(value); err != nil {
							t.Error(err)
						}
						flag.Changed = changed
					})
				})
			}
			var output bytes.Buffer
			oldOut, oldErr := rootCmd.OutOrStdout(), rootCmd.ErrOrStderr()
			rootCmd.SetOut(&output)
			rootCmd.SetErr(&output)
			t.Cleanup(func() {
				rootCmd.SetOut(oldOut)
				rootCmd.SetErr(oldErr)
				rootCmd.SetArgs(nil)
			})
			t.Setenv("ENVIRONMENT", "dev")
			t.Setenv("BEAR_ENVIRONMENT", "dev")
			root := t.TempDir()
			if !tt.missingConfig {
				if err := os.WriteFile(filepath.Join(root, "bear.config.yml"), []byte("name: test\nlanguages:\n  test:\n    detection:\n      files: [bear.artifact.yml]\n    steps: []\ntargets:\n  local:\n    steps:\n      - name: deploy\n        run: 'true'\n"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range []string{"api", "web", "worker"} {
				dir := filepath.Join(root, name)
				if err := os.Mkdir(dir, 0755); err != nil {
					t.Fatal(err)
				}
				content := fmt.Sprintf("name: %s\ntarget: local\n", name)
				if !tt.validationOnly {
					content += "environments: [dev, int, prd]\n"
				}
				if err := os.WriteFile(filepath.Join(dir, "bear.artifact.yml"), []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".bear/\nbear.lock.yml\n"), 0644); err != nil {
				t.Fatal(err)
			}
			git := func(args ...string) string {
				t.Helper()
				command := exec.Command("git", args...)
				command.Dir = root
				out, err := command.CombinedOutput()
				if err != nil {
					t.Fatalf("git %v: %v\n%s", args, err, out)
				}
				return strings.TrimSpace(string(out))
			}
			git("init")
			git("add", ".")
			git("-c", "user.name=Bear Test", "-c", "user.email=bear@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "fixture")
			wantCommit, wantPin := git("rev-parse", "HEAD"), ""
			if tt.pin != "" {
				wantPin = wantCommit
				if err := os.WriteFile(filepath.Join(root, "revision.txt"), []byte("newer source\n"), 0644); err != nil {
					t.Fatal(err)
				}
				git("add", ".")
				git("-c", "user.name=Bear Test", "-c", "user.email=bear@example.invalid", "-c", "commit.gpgsign=false", "commit", "-m", "newer fixture")
			}
			if err := config.WritePlan(root, config.NewPlanFile("stale")); err != nil {
				t.Fatal(err)
			}
			stale, err := os.ReadFile(config.PlanFilePath(root))
			if err != nil {
				t.Fatal(err)
			}
			rootCmd.SetArgs(append([]string{"plan", "-d", root}, tt.args...))
			_, err = rootCmd.ExecuteC()
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("expected %q, got %v\n%s", tt.wantError, err, &output)
				}
				if tt.preservePlan {
					data, err := os.ReadFile(config.PlanFilePath(root))
					if err != nil || !bytes.Equal(data, stale) {
						t.Fatalf("syntax error changed saved plan: %v", err)
					}
				} else if config.PlanExists(root) {
					t.Fatal("planning error retained stale plan")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			plan, err := config.ReadPlan(root)
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, artifact := range plan.Artifacts {
				names = append(names, artifact.Name)
				if artifact.Vars["ENVIRONMENT"] != tt.environment || artifact.PinCommit != wantPin || artifact.Pinned != (tt.pin != "") {
					t.Errorf("options not passed to artifact: %+v", artifact)
				}
				if artifact.Path != artifact.Name || !slices.Equal(artifact.Environments, []string{"dev", "int", "prd"}) {
					t.Errorf("invalid artifact snapshot: %+v", artifact)
				}
			}
			if plan.Commit != wantCommit || len(plan.SourceFingerprint) != 64 || plan.Pinned != (tt.pin != "") {
				t.Fatalf("missing source evidence: %+v", plan)
			}
			slices.Sort(names)
			wantValidations := len(tt.artifacts)
			if tt.validationOnly {
				wantValidations = 3
			}
			if plan.Environment != tt.environment || plan.ToDeploy != len(tt.artifacts) || plan.Validated != wantValidations || len(plan.Validations) != wantValidations || !slices.Equal(names, tt.artifacts) {
				t.Fatalf("arguments not passed to planner: %+v, artifacts: %v", plan, names)
			}
		})
	}
}
