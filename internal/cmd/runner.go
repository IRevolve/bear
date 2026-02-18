package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
)

const defaultConcurrency = 10

// StepResult holds the result of a single step execution
type StepResult struct {
	StepName string
	Output   string
	Err      error
}

// ArtifactResult holds all step results for one artifact
type ArtifactResult struct {
	Name    string
	Results []StepResult
	Err     error
}

// RunParallel runs a function for each item in parallel with the given concurrency limit.
// The function f receives the index and must return an error.
// Results are collected and returned in order.
func RunParallel(ctx context.Context, concurrency int, count int, f func(ctx context.Context, i int) error) []error {
	if concurrency <= 0 {
		concurrency = defaultConcurrency
	}

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	errs := make([]error, count)
	var mu sync.Mutex

	for i := 0; i < count; i++ {
		i := i
		g.Go(func() error {
			err := f(ctx, i)
			if err != nil {
				mu.Lock()
				errs[i] = err
				mu.Unlock()
			}
			// Don't return error — we want all jobs to finish
			return nil
		})
	}

	g.Wait()
	return errs
}

// ExecuteStep runs a single step command in the given working directory.
// Variables are substituted in the command string.
// Output is written to the provided writers.
func ExecuteStep(ctx context.Context, stepRun string, workDir string, vars map[string]string, stdout, stderr *bytes.Buffer) error {
	// Replace variables in the run command
	// Sort keys by length (longest first) so $NAMESPACE is replaced before $NAME
	command := stepRun
	keys := make([]string, 0, len(vars))
	for key := range vars {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	for _, key := range keys {
		value := vars[key]
		command = strings.ReplaceAll(command, "${"+key+"}", value)
		command = strings.ReplaceAll(command, "$"+key, value)
	}

	// Detect shell based on OS
	shell, shellArg := getShell()

	cmd := exec.CommandContext(ctx, shell, shellArg, command)
	cmd.Dir = workDir

	if stdout != nil {
		cmd.Stdout = stdout
	} else {
		cmd.Stdout = os.Stdout
	}
	if stderr != nil {
		cmd.Stderr = stderr
	} else {
		cmd.Stderr = os.Stderr
	}

	return cmd.Run()
}

// ExecuteStepDirect runs a step with output directly to os.Stdout/os.Stderr (for verbose mode).
func ExecuteStepDirect(ctx context.Context, stepRun string, workDir string, vars map[string]string) error {
	command := stepRun
	keys := make([]string, 0, len(vars))
	for key := range vars {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	for _, key := range keys {
		value := vars[key]
		command = strings.ReplaceAll(command, "${"+key+"}", value)
		command = strings.ReplaceAll(command, "$"+key, value)
	}

	shell, shellArg := getShell()
	cmd := exec.CommandContext(ctx, shell, shellArg, command)
	cmd.Dir = workDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// getShell returns the appropriate shell and argument for the current OS
func getShell() (string, string) {
	if isWindows() {
		return "cmd", "/C"
	}
	return "sh", "-c"
}

// isWindows checks if the current OS is Windows
func isWindows() bool {
	return os.PathSeparator == '\\' && os.PathListSeparator == ';'
}

// CollectErrors filters non-nil errors from a slice
func CollectErrors(errs []error) []error {
	var result []error
	for _, err := range errs {
		if err != nil {
			result = append(result, err)
		}
	}
	return result
}

// FormatErrors formats multiple errors into a single error
func FormatErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	var msgs []string
	for _, err := range errs {
		msgs = append(msgs, err.Error())
	}
	return fmt.Errorf("%s", strings.Join(msgs, "\n"))
}
