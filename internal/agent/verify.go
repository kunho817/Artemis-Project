package agent

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// VerifyFunc is called after each autonomous iteration to check if the task is complete.
// Returns:
//   - passed: true if verification succeeded (loop stops)
//   - feedback: error description fed back to the agent for correction (empty if passed)
//   - err: hard error that aborts the loop entirely
type VerifyFunc func(ctx context.Context, agentOutput string) (passed bool, feedback string, err error)

// VerifyNone always passes — no verification, single iteration.
func VerifyNone() VerifyFunc {
	return func(ctx context.Context, output string) (bool, string, error) {
		return true, "", nil
	}
}

// VerifyBuild runs "go build ./..." and checks for compilation errors.
func VerifyBuild(workDir string) VerifyFunc {
	return func(ctx context.Context, output string) (bool, string, error) {
		cmdCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()

		cmd := exec.CommandContext(cmdCtx, "go", "build", "./...")
		cmd.Dir = workDir
		if runtime.GOOS == "windows" {
			setHiddenAttrs(cmd)
		}

		out, err := cmd.CombinedOutput()
		if err != nil {
			return false, fmt.Sprintf("Build failed:\n%s", strings.TrimSpace(string(out))), nil
		}
		return true, "", nil
	}
}

// VerifyTest runs "go test" on specified packages and checks for failures.
func VerifyTest(workDir string, packages string) VerifyFunc {
	if packages == "" {
		packages = "./..."
	}
	return func(ctx context.Context, output string) (bool, string, error) {
		cmdCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
		defer cancel()

		cmd := exec.CommandContext(cmdCtx, "go", "test", "-count=1", packages)
		cmd.Dir = workDir
		if runtime.GOOS == "windows" {
			setHiddenAttrs(cmd)
		}

		out, err := cmd.CombinedOutput()
		if err != nil {
			return false, fmt.Sprintf("Tests failed:\n%s", strings.TrimSpace(string(out))), nil
		}
		return true, "", nil
	}
}

// VerifyBuildAndTest combines build verification followed by test verification.
func VerifyBuildAndTest(workDir, packages string) VerifyFunc {
	buildFn := VerifyBuild(workDir)
	testFn := VerifyTest(workDir, packages)
	return func(ctx context.Context, output string) (bool, string, error) {
		// Build first
		passed, feedback, err := buildFn(ctx, output)
		if err != nil || !passed {
			return passed, feedback, err
		}
		// Then test
		return testFn(ctx, output)
	}
}

// VerifyCommand runs a custom shell command for verification.
// Exit code 0 = passed, non-zero = failed with stderr as feedback.
func VerifyCommand(workDir, command string) VerifyFunc {
	return func(ctx context.Context, output string) (bool, string, error) {
		cmdCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()

		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.CommandContext(cmdCtx, "cmd", "/c", command)
		} else {
			cmd = exec.CommandContext(cmdCtx, "sh", "-c", command)
		}
		cmd.Dir = workDir
		if runtime.GOOS == "windows" {
			setHiddenAttrs(cmd)
		}

		out, err := cmd.CombinedOutput()
		if err != nil {
			return false, fmt.Sprintf("Verification command failed:\n$ %s\n%s", command, strings.TrimSpace(string(out))), nil
		}
		return true, "", nil
	}
}

// ChainVerify runs multiple verification functions in sequence.
// Stops at the first failure.
func ChainVerify(fns ...VerifyFunc) VerifyFunc {
	return func(ctx context.Context, output string) (bool, string, error) {
		for _, fn := range fns {
			passed, feedback, err := fn(ctx, output)
			if err != nil || !passed {
				return passed, feedback, err
			}
		}
		return true, "", nil
	}
}

// ResolveVerifyFunc returns a VerifyFunc for a named verification strategy.
// Known names: "build", "test", "build+test", "none", or a custom command.
func ResolveVerifyFunc(name, workDir string) VerifyFunc {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "none":
		return VerifyNone()
	case "build":
		return VerifyBuild(workDir)
	case "test":
		return VerifyTest(workDir, "./...")
	case "build+test", "build_and_test":
		return VerifyBuildAndTest(workDir, "./...")
	default:
		// Treat as custom command
		return VerifyCommand(workDir, name)
	}
}

// setHiddenAttrs is a no-op stub for Windows process hiding.
// Same pattern as tools/git.go.
func setHiddenAttrs(cmd *exec.Cmd) {
	// Intentionally empty — see tools/git.go for explanation.
}
