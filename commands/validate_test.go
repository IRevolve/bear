package commands

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/irevolve/bear/internal/config"
	"github.com/spf13/pflag"
)

// validateFixture writes a project whose only validation step records the
// ENVIRONMENT it ran with, so a test can prove which artifacts the command
// selected and what it handed them. It is not a Git repository.
func validateFixture(t *testing.T) (root, markers string) {
	t.Helper()
	root, markers = t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "bear.config.yml"), []byte(`name: test
environments: [dev, int, prd]
languages:
  test:
    detection:
      files: [bear.artifact.yml]
    steps:
      - name: record
        run: |
          printf '%s' "${ENVIRONMENT-unset}" > "$BEAR_TEST_OUTPUT/$(basename "$PWD").validated"
targets:
  local:
    steps:
      - name: deploy
        run: 'true'
`), 0644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"api", "web", "worker"} {
		dir := filepath.Join(root, name)
		if err := os.Mkdir(dir, 0755); err != nil {
			t.Fatal(err)
		}
		content := fmt.Sprintf("name: %s\ntarget: local\nenvironments: [dev, int, prd]\n", name)
		if err := os.WriteFile(filepath.Join(dir, "bear.artifact.yml"), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return root, markers
}

// captureValidateOutput redirects the printer's stdout so a command test reads
// what a CI log would show instead of scattering it through the test output.
func captureValidateOutput(t *testing.T, run func() error) (string, error) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	previous := os.Stdout
	os.Stdout = f
	defer func() { os.Stdout = previous }()
	runErr := run()
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	return string(data), runErr
}

func TestValidateCommand(t *testing.T) {
	for _, tt := range []struct {
		name        string
		args        []string
		wantError   string
		validated   []string
		environment string
	}{
		{name: "no arguments validates everything", validated: []string{"api", "web", "worker"}, environment: "ambient"},
		{name: "single artifact", args: []string{"web"}, validated: []string{"web"}, environment: "ambient"},
		{name: "multiple artifacts", args: []string{"worker", "api"}, validated: []string{"api", "worker"}, environment: "ambient"},
		{name: "repeated artifact", args: []string{"api", "api"}, validated: []string{"api"}, environment: "ambient"},
		{name: "unknown artifact", args: []string{"nope"}, wantError: `unknown artifact "nope"`},
		{name: "unknown artifact beside a known one", args: []string{"api", "nope"}, wantError: `unknown artifact "nope"`},
		// There is no environment argument: a positional is always an artifact.
		{name: "environment is not a positional", args: []string{"int"}, wantError: `unknown artifact "int"`},
		{name: "environment flag", args: []string{"--environment", "int"}, validated: []string{"api", "web", "worker"}, environment: "int"},
		{name: "undeclared environment flag", args: []string{"--environment", "production"}, wantError: `unknown environment "production": bear.config.yml declares dev, int, prd`},
		{name: "malformed environment flag", args: []string{"--environment", "PRD"}, wantError: `invalid environment name "PRD"`},
		{name: "empty environment flag injects nothing", args: []string{"--environment", ""}, validated: []string{"api", "web", "worker"}, environment: "ambient"},
		{name: "concurrency", args: []string{"--concurrency", "1"}, validated: []string{"api", "web", "worker"}, environment: "ambient"},
		{name: "interspersed flags", args: []string{"--concurrency", "1", "web", "--environment", "prd", "api", "-v"}, validated: []string{"api", "web"}, environment: "prd"},
		{name: "deployment flag pin", args: []string{"--pin", "HEAD"}, wantError: "unknown flag: --pin"},
		{name: "deployment flag no-commit", args: []string{"--no-commit"}, wantError: "unknown flag: --no-commit"},
		{name: "deployment flag git-remote", args: []string{"--git-remote", "origin"}, wantError: "unknown flag: --git-remote"},
		{name: "change detection flag", args: []string{"--base-ref", "main"}, wantError: "unknown flag: --base-ref"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for _, flags := range []*pflag.FlagSet{rootCmd.PersistentFlags(), validateCmd.Flags()} {
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
			var usage bytes.Buffer
			oldOut, oldErr := rootCmd.OutOrStdout(), rootCmd.ErrOrStderr()
			rootCmd.SetOut(&usage)
			rootCmd.SetErr(&usage)
			t.Cleanup(func() {
				rootCmd.SetOut(oldOut)
				rootCmd.SetErr(oldErr)
				rootCmd.SetArgs(nil)
			})
			root, markers := validateFixture(t)
			t.Setenv("BEAR_TEST_OUTPUT", markers)
			// The job environment is inherited, never injected, without the flag.
			t.Setenv("ENVIRONMENT", "ambient")
			rootCmd.SetArgs(append([]string{"validate", "-d", root}, tt.args...))
			output, err := captureValidateOutput(t, func() error {
				_, err := rootCmd.ExecuteC()
				return err
			})
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("expected %q, got %v\n%s%s", tt.wantError, err, output, &usage)
				}
			} else if err != nil {
				t.Fatalf("%v\n%s", err, output)
			}
			entries, readErr := os.ReadDir(markers)
			if readErr != nil {
				t.Fatal(readErr)
			}
			var validated []string
			for _, entry := range entries {
				name := strings.TrimSuffix(entry.Name(), ".validated")
				validated = append(validated, name)
				data, err := os.ReadFile(filepath.Join(markers, entry.Name()))
				if err != nil {
					t.Fatal(err)
				}
				if string(data) != tt.environment {
					t.Errorf("artifact %s ran with ENVIRONMENT %q, want %q", name, data, tt.environment)
				}
			}
			slices.Sort(validated)
			if !slices.Equal(validated, tt.validated) {
				t.Fatalf("validated %v, want %v\n%s", validated, tt.validated, output)
			}
			if tt.wantError == "" {
				for _, text := range []string{"Bear Validate", "Validating ", "Validation complete: "} {
					if !strings.Contains(output, text) {
						t.Errorf("validate output missing %q: %s", text, output)
					}
				}
			}
			// Validation is deployment-free whatever the arguments were.
			if config.PlanExists(root) {
				t.Error("validate wrote a plan file")
			}
			if _, err := os.Stat(filepath.Join(root, "bear.lock.yml")); !os.IsNotExist(err) {
				t.Errorf("validate touched the lock file: %v", err)
			}
			if _, err := os.Stat(filepath.Join(root, ".git")); !os.IsNotExist(err) {
				t.Errorf("validate created a repository: %v", err)
			}
		})
	}
}

func TestValidateCommandMissingConfig(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(func() { rootCmd.SetArgs(nil) })
	rootCmd.SetArgs([]string{"validate", "-d", root})
	if _, err := rootCmd.ExecuteC(); err == nil || !strings.Contains(err.Error(), "config file not found") {
		t.Fatalf("expected a missing-config error, got %v", err)
	}
	// Refusing early leaves no workspace behind in an unrelated directory.
	if _, err := os.Stat(filepath.Join(root, ".bear")); !os.IsNotExist(err) {
		t.Fatalf("missing config still created a workspace: %v", err)
	}
}
