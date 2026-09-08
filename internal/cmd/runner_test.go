package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestResolveVarsLiteralSecretsAndPATH(t *testing.T) {
	t.Setenv("BEAR_TEST_SECRET", "token$OTHER/${OTHER}/$$")
	t.Setenv("PATH", "/inherited/$literal")
	vars := map[string]string{
		"SECRET": "${BEAR_TEST_SECRET}",
		"CHAIN":  "prefix/${SECRET}",
		"OTHER":  "must-not-expand",
		"PATH":   "${PATH}:/custom",
	}
	want := map[string]string{
		"SECRET": "token$OTHER/${OTHER}/$$",
		"CHAIN":  "prefix/token$OTHER/${OTHER}/$$",
		"OTHER":  "must-not-expand",
		"PATH":   "/inherited/$literal:/custom",
	}
	for range 20 {
		got, err := resolveVars(vars)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("resolveVars = %v, %v; want %v", got, err, want)
		}
	}
}

func TestResolveVarsCyclesAndLimits(t *testing.T) {
	t.Setenv("BEAR_TEST_UNSET", "temporary")
	if err := os.Unsetenv("BEAR_TEST_UNSET"); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		vars    map[string]string
		message string
	}{
		{"self", map[string]string{"BEAR_TEST_UNSET": "$BEAR_TEST_UNSET"}, "cycle"},
		{"cycle", map[string]string{"B": "$A", "A": "$B"}, "A -> B -> A"},
		{"source limit", map[string]string{"A": strings.Repeat("x", maxVarSize+1)}, "size limit"},
		{"expansion limit", map[string]string{"A": strings.Repeat("x", maxVarSize/2+1), "B": "$A$A"}, "size limit"},
		{"invalid key", map[string]string{"A=B": "x"}, "invalid environment"},
		{"invalid value", map[string]string{"A": "x\x00"}, "invalid environment"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for range 5 {
				if _, err := resolveVars(tt.vars); err == nil || !strings.Contains(err.Error(), tt.message) {
					t.Fatalf("resolveVars error = %v; want %q", err, tt.message)
				}
			}
		})
	}
	vars := make(map[string]string)
	for i := 0; i < maxVarDepth+1; i++ {
		vars[fmt.Sprintf("V%03d", i)] = fmt.Sprintf("${V%03d}", i+1)
	}
	if _, err := resolveVars(vars); err == nil || !strings.Contains(err.Error(), "depth limit") {
		t.Fatalf("depth error = %v", err)
	}
	vars = make(map[string]string)
	for i := 0; i <= maxVarsSize/maxVarSize; i++ {
		vars[fmt.Sprintf("V%d", i)] = strings.Repeat("x", maxVarSize)
	}
	if _, err := resolveVars(vars); err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("aggregate limit error = %v", err)
	}
}

func TestBuildEnvOverridesAndValidates(t *testing.T) {
	t.Setenv("BEAR_TEST_ENV", "old")
	env, err := buildEnv(map[string]string{"BEAR_TEST_ENV": "new"})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range env {
		if strings.HasPrefix(entry, "BEAR_TEST_ENV=") {
			count++
			if entry != "BEAR_TEST_ENV=new" {
				t.Fatal(entry)
			}
		}
	}
	if count != 1 {
		t.Fatalf("override appears %d times", count)
	}
	if _, err := buildEnv(map[string]string{"A": "$B", "B": "$A"}); err == nil {
		t.Fatal("buildEnv accepted a cycle")
	}
	err = ExecuteStep(context.Background(), "exit 0", "", map[string]string{"A": "$B", "B": "$A"}, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "resolve step variables") {
		t.Fatalf("ExecuteStep validation error = %v", err)
	}
}

func TestRunParallelCancellationAndOrdinaryErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32
	errs := RunParallel(ctx, 1, 20, func(ctx context.Context, i int) error {
		calls.Add(1)
		cancel()
		return nil
	})
	if calls.Load() != 1 {
		t.Fatalf("ran %d functions after cancellation", calls.Load())
	}
	if errs[0] != nil {
		t.Fatalf("successful function became cancelled: %v", errs[0])
	}
	for i, err := range errs[1:] {
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("errs[%d] = %v", i+1, err)
		}
	}
	errs = RunParallel(ctx, 3, 10, func(context.Context, int) error {
		t.Error("called function with already cancelled context")
		return nil
	})
	if len(CollectErrors(errs)) != len(errs) {
		t.Fatal("cancelled queue reported success")
	}
	failure := errors.New("ordinary failure")
	calls.Store(0)
	errs = RunParallel(context.Background(), 3, 20, func(ctx context.Context, i int) error {
		calls.Add(1)
		if ctx.Err() != nil {
			t.Error("ordinary error cancelled siblings")
		}
		if i == 0 {
			return failure
		}
		return nil
	})
	if calls.Load() != 20 || !errors.Is(errs[0], failure) || len(CollectErrors(errs)) != 1 {
		t.Fatalf("calls = %d, errs = %v", calls.Load(), errs)
	}
}

func TestTailBufferBoundedAndConcurrent(t *testing.T) {
	var b TailBuffer
	var want string
	for _, size := range []int{3, 40000, 30000, 100000, 1, 65535} {
		p := strings.Repeat(fmt.Sprint(size%10), size)
		n, err := io.WriteString(&b, p)
		want += p
		if len(want) > tailBufferSize {
			want = want[len(want)-tailBufferSize:]
		}
		if n != size || err != nil || b.String() != want {
			t.Fatalf("incorrect tail after %d-byte write", size)
		}
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 100 {
				_, _ = io.WriteString(&b, strings.Repeat("x", 1024))
				if len(b.String()) > tailBufferSize {
					t.Error("capture exceeds limit")
				}
			}
		})
	}
	wg.Wait()
	if b.String() != strings.Repeat("x", tailBufferSize) {
		t.Fatal("incorrect concurrent tail")
	}
}

func TestRunParallelKeepsWorkersBusy(t *testing.T) {
	release := make(chan struct{})
	last := make(chan struct{})
	done := make(chan []error, 1)
	go func() {
		done <- RunParallel(context.Background(), 2, 4, func(_ context.Context, i int) error {
			if i == 0 {
				<-release
			}
			if i == 3 {
				close(last)
			}
			return nil
		})
	}()
	select {
	case <-last:
	case <-time.After(time.Second):
		t.Error("idle worker did not pick up queued work")
	}
	close(release)
	if errs := <-done; len(CollectErrors(errs)) != 0 {
		t.Fatal(errs)
	}
}

func TestBuildEnvGitRepositoryOverrides(t *testing.T) {
	keys := []string{
		"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE",
		"GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_PREFIX", "GIT_NAMESPACE",
	}
	for _, key := range keys {
		t.Setenv(key, "foreign-repository")
		t.Run(key, func(t *testing.T) {
			for _, value := range []string{"", "configured-repository"} {
				_, err := buildEnv(map[string]string{key: value})
				if err == nil || !strings.Contains(err.Error(), key) || !strings.Contains(err.Error(), "reserved for source integrity") {
					t.Fatalf("configured override error = %v", err)
				}
			}
			if isWindows() {
				if _, err := buildEnv(map[string]string{strings.ToLower(key): "foreign"}); err == nil {
					t.Fatal("case-insensitive override accepted on Windows")
				}
			}
		})
	}
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
	env, err := buildEnv(nil)
	if err != nil {
		t.Fatal(err)
	}
	preserved := false
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		for _, reserved := range keys {
			if strings.EqualFold(key, reserved) {
				t.Fatalf("inherited repository override retained: %s", entry)
			}
		}
		preserved = preserved || entry == "GIT_TERMINAL_PROMPT=0"
	}
	if !preserved {
		t.Fatal("unrelated Git environment setting removed")
	}
	err = ExecuteStep(context.Background(), "exit 0", "", map[string]string{"GIT_DIR": "foreign"}, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "reserved for source integrity") {
		t.Fatalf("ExecuteStep accepted repository override: %v", err)
	}
}

func TestExecuteStepIgnoresInheritedForeignGitDir(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	intended, foreign := t.TempDir(), t.TempDir()
	env, err := buildEnv(nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, dir := range []string{intended, foreign} {
		cmd := exec.CommandContext(ctx, "git", "init", dir)
		cmd.Env = env
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git init: %v\n%s", err, output)
		}
	}
	t.Setenv("GIT_DIR", filepath.Join(foreign, ".git"))
	t.Setenv("GIT_WORK_TREE", foreign)
	var output TailBuffer
	if err := ExecuteStep(ctx, "git rev-parse --show-toplevel", intended, nil, &output, &output); err != nil {
		t.Fatalf("git rev-parse: %v\n%s", err, output.String())
	}
	want, err := filepath.EvalSymlinks(intended)
	if err != nil {
		t.Fatal(err)
	}
	got, err := filepath.EvalSymlinks(strings.TrimSpace(output.String()))
	if err != nil || got != want {
		t.Fatalf("git selected %q, want %q: %v", got, want, err)
	}
}

// os/exec checks Done before querying Err. With an uncancelled command, an Err
// query here models cancellation at the caller's post-run context check.
type cancelOnErrContext struct {
	context.Context
	cancel context.CancelFunc
	checks atomic.Int32
}

func (ctx *cancelOnErrContext) Err() error {
	ctx.checks.Add(1)
	ctx.cancel()
	return ctx.Context.Err()
}

func TestExecuteStepPreservesSuccessOnLateCancellation(t *testing.T) {
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &cancelOnErrContext{Context: base, cancel: cancel}
	err := ExecuteStep(ctx, "exit 0", "", nil, io.Discard, io.Discard)
	cancel()
	if err != nil {
		t.Fatalf("successful command became cancelled: %v", err)
	}
	if ctx.checks.Load() != 0 {
		t.Fatalf("successful command rechecked cancellation %d times", ctx.checks.Load())
	}
}
