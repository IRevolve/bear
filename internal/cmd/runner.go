package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
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

	errs := make([]error, count)
	var wg sync.WaitGroup
	var next atomic.Int64
	for worker := 0; worker < min(concurrency, count); worker++ {
		wg.Go(func() {
			for {
				i := int(next.Add(1) - 1)
				if i >= count {
					return
				}
				if err := ctx.Err(); err != nil {
					errs[i] = err
					continue
				}
				errs[i] = f(ctx, i)
			}
		})
	}
	wg.Wait()
	return errs
}

const (
	maxVarSize    = 1 << 20
	maxVarsSize   = 8 << 20
	maxVarDepth   = 128
	stepWaitDelay = 2 * time.Second
)

// resolveVars expands each source value once. Substituted values, including
// inherited secrets containing dollar signs, are always literal.
func resolveVars(vars map[string]string) (map[string]string, error) {
	resolved := make(map[string]string, len(vars))
	keys := make([]string, 0, len(vars))
	for key := range vars {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if key == "" || strings.ContainsAny(key, "=\x00") || strings.ContainsRune(vars[key], 0) {
			return nil, fmt.Errorf("invalid environment variable %q", key)
		}
		if isGitRepositoryOverride(key) {
			return nil, fmt.Errorf("environment variable %q is reserved for source integrity", key)
		}
	}
	visiting := make(map[string]bool, len(vars))
	var stack []string
	total := 0
	var resolve func(string) (string, error)
	resolve = func(key string) (string, error) {
		if value, ok := resolved[key]; ok {
			return value, nil
		}
		if visiting[key] {
			return "", fmt.Errorf("variable dependency cycle: %s", strings.Join(append(stack, key), " -> "))
		}
		if len(stack) >= maxVarDepth {
			return "", fmt.Errorf("variable %q exceeds dependency depth limit (%d)", key, maxVarDepth)
		}
		if len(vars[key]) > maxVarSize {
			return "", fmt.Errorf("variable %q exceeds expansion size limit (%d bytes)", key, maxVarSize)
		}
		visiting[key] = true
		stack = append(stack, key)
		var expandErr error
		inserted := 0
		value := os.Expand(vars[key], func(ref string) string {
			if expandErr != nil {
				return ""
			}
			var replacement string
			_, configured := vars[ref]
			inherited, exists := os.LookupEnv(ref)
			if !configured || (ref == key && exists) {
				replacement = inherited
			} else {
				replacement, expandErr = resolve(ref)
			}
			// Bound allocations inside os.Expand, not merely its final result.
			if len(replacement) > maxVarSize-inserted {
				expandErr = fmt.Errorf("variable %q exceeds expansion size limit (%d bytes)", key, maxVarSize)
			}
			if expandErr != nil {
				return ""
			}
			inserted += len(replacement)
			return replacement
		})
		if expandErr != nil {
			return "", expandErr
		}
		if len(value) > maxVarSize || len(value) > maxVarsSize-total {
			return "", fmt.Errorf("variable %q exceeds expansion size limit", key)
		}
		total += len(value)
		resolved[key] = value
		stack = stack[:len(stack)-1]
		delete(visiting, key)
		return value, nil
	}
	for _, key := range keys {
		if _, err := resolve(key); err != nil {
			return nil, err
		}
	}
	return resolved, nil
}

func isGitRepositoryOverride(key string) bool {
	if isWindows() {
		key = strings.ToUpper(key)
	}
	switch key {
	case "GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE",
		"GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_PREFIX", "GIT_NAMESPACE":
		return true
	}
	return false
}

// buildEnv merges resolved vars with the process environment, excluding Git
// repository overrides so subprocesses use the intended checkout.
func buildEnv(vars map[string]string) ([]string, error) {
	resolved, err := resolveVars(vars)
	if err != nil {
		return nil, err
	}
	env := make([]string, 0, len(resolved))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if isGitRepositoryOverride(key) {
			continue
		}
		if _, overridden := resolved[key]; !overridden {
			env = append(env, entry)
		}
	}
	keys := make([]string, 0, len(resolved))
	for key := range resolved {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		env = append(env, key+"="+resolved[key])
	}
	return env, nil
}

// ExecuteStep runs a single step command in the given working directory.
// Variables are passed as environment variables to the shell.
// Output is written to the provided writers.
func ExecuteStep(ctx context.Context, stepRun string, workDir string, vars map[string]string, stdout, stderr io.Writer) error {
	env, err := buildEnv(vars)
	if err != nil {
		return fmt.Errorf("resolve step variables: %w", err)
	}
	// Detect shell based on OS
	shell, shellArg := getShell()

	cmd := exec.CommandContext(ctx, shell, shellArg, stepRun)
	cmd.Dir = workDir
	cmd.Env = env
	cmd.WaitDelay = stepWaitDelay
	configureProcessTree(cmd)

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

	err = cmd.Run()
	if err != nil {
		cleanupProcessTree(cmd)
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return err
}

// ExecuteStepDirect runs a step with output directly to os.Stdout/os.Stderr (for verbose mode).
func ExecuteStepDirect(ctx context.Context, stepRun string, workDir string, vars map[string]string) error {
	return ExecuteStep(ctx, stepRun, workDir, vars, os.Stdout, os.Stderr)
}

const tailBufferSize = 64 << 10

// TailBuffer is a concurrency-safe writer retaining only the last 64 KiB.
// Its zero value is ready to use.
type TailBuffer struct {
	mu   sync.Mutex
	data [tailBufferSize]byte
	end  int
	size int
}

func (b *TailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	if len(p) >= len(b.data) {
		p = p[len(p)-len(b.data):]
		copy(b.data[:], p)
		b.end, b.size = 0, len(b.data)
		return n, nil
	}
	first := copy(b.data[b.end:], p)
	copy(b.data[:], p[first:])
	b.end = (b.end + len(p)) % len(b.data)
	b.size = min(len(b.data), b.size+len(p))
	return n, nil
}

func (b *TailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	start := (b.end - b.size + len(b.data)) % len(b.data)
	result := make([]byte, b.size)
	first := copy(result, b.data[start:min(start+b.size, len(b.data))])
	copy(result[first:], b.data[:b.size-first])
	return string(result)
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
