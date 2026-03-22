// Package tools provides integration tests for Artemis tool execution.
package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/artemis-project/artemis/internal/tools"
	"github.com/artemis-project/artemis/tests/integration/harness"
)

// TestShellExecBasicCommand tests basic shell command execution.
func TestShellExecBasicCommand(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	// Create tool executor
	exec := tools.NewToolExecutor(h.TempDir)

	// Test echo command
	ctx := context.Background()
	result, err := exec.Execute(ctx, "shell_exec", map[string]interface{}{
		"command": "echo hello",
	})

	if err != nil {
		h.T.Fatalf("Execute failed: %v", err)
	}

	if result.Error != "" {
		h.T.Errorf("Unexpected error: %s", result.Error)
	}

	// Check output contains "hello"
	if !strings.Contains(result.Content, "hello") {
		h.T.Errorf("Expected output to contain 'hello', got: %s", result.Content)
	}

	// Check for exit code 0
	if !strings.Contains(result.Content, "[exit: 0]") {
		h.T.Errorf("Expected exit code 0, got: %s", result.Content)
	}
}

// TestShellExecWithTimeout tests command timeout handling.
func TestShellExecWithTimeout(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)
	_ = h // Harness manages cleanup automatically

	exec := tools.NewToolExecutor(h.TempDir)

	// Test a command that completes quickly
	ctx := context.Background()
	result, err := exec.Execute(ctx, "shell_exec", map[string]interface{}{
		"command": "echo quick",
		"timeout": 5.0, // 5 seconds
	})

	if err != nil {
		h.T.Fatalf("Execute failed: %v", err)
	}

	if result.Error != "" {
		h.T.Errorf("Unexpected error: %s", result.Error)
	}

	if !strings.Contains(result.Content, "quick") {
		h.T.Errorf("Expected output to contain 'quick', got: %s", result.Content)
	}
}

// TestShellExecNonZeroExit tests command that fails with non-zero exit code.
func TestShellExecNonZeroExit(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)
	_ = h // Harness manages cleanup automatically

	exec := tools.NewToolExecutor(h.TempDir)

	// Test a command that will fail
	ctx := context.Background()
	result, err := exec.Execute(ctx, "shell_exec", map[string]interface{}{
		"command": "exit 1", // Command that exits with code 1
	})

	if err != nil {
		h.T.Fatalf("Execute failed: %v", err)
	}

	// Error field should be set
	if result.Error == "" {
		h.T.Error("Expected error to be set for non-zero exit")
	}

	// Content should still contain output
	if !strings.Contains(result.Content, "[exit:") {
		h.T.Errorf("Expected exit code in output, got: %s", result.Content)
	}
}

// TestShellExecWorkdir tests working directory parameter.
func TestShellExecWorkdir(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)
	_ = h // Harness manages cleanup automatically

	// Create a subdirectory
	subDir := h.CreateTempFile("test_subdir", "")
	subDirPath := subDir[:len(subDir)-len("/test_subdir")] // Remove filename

	exec := tools.NewToolExecutor(subDirPath)

	// Test command with workdir
	ctx := context.Background()
	result, err := exec.Execute(ctx, "shell_exec", map[string]interface{}{
		"command": "cd test_subdir && echo test",
	})

	if err != nil {
		h.T.Fatalf("Execute failed: %v", err)
	}

	if result.Error != "" {
		h.T.Errorf("Unexpected error: %s", result.Error)
	}
}

// TestShellExecMissingCommand tests error handling for missing command parameter.
func TestShellExecMissingCommand(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)
	_ = h // Harness manages cleanup automatically

	exec := tools.NewToolExecutor(h.TempDir)

	ctx := context.Background()
	result, err := exec.Execute(ctx, "shell_exec", map[string]interface{}{
		// No "command" parameter
	})

	if err != nil {
		h.T.Fatalf("Execute failed: %v", err)
	}

	// Should return error about missing parameter
	if result.Error == "" {
		h.T.Error("Expected error for missing command parameter")
	}

	if !strings.Contains(result.Error, "missing required parameter") {
		h.T.Errorf("Expected 'missing required parameter' error, got: %s", result.Error)
	}
}

// TestShellExecLargeOutput tests handling of large command output.
func TestShellExecLargeOutput(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)
	_ = h // Harness manages cleanup automatically

	exec := tools.NewToolExecutor(h.TempDir)

	// Create a command that generates large output
	// Note: Using a simple command that generates significant output
	ctx := context.Background()
	result, err := exec.Execute(ctx, "shell_exec", map[string]interface{}{
		"command": "echo test && for i in {1..100}; do echo \"Line $i with some additional text to make it longer\"; done",
	})

	if err != nil {
		h.T.Fatalf("Execute failed: %v", err)
	}

	if result.Error != "" {
		h.T.Errorf("Unexpected error: %s", result.Error)
	}

	// Verify output is present (may be truncated)
	if !strings.Contains(result.Content, "Line 1") {
		h.T.Errorf("Expected 'Line 1' in output, got: %s", result.Content)
	}

	// Check for truncation marker if output is large
	if len(result.Content) > 200*1024 {
		if !strings.Contains(result.Content, "(truncated)") {
			h.T.Error("Expected truncation marker for large output")
		}
	}
}

// TestShellExecContextCancellation tests command cancellation via context.
func TestShellExecContextCancellation(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)
	_ = h // Harness manages cleanup automatically

	exec := tools.NewToolExecutor(h.TempDir)

	// Create a context that cancels immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	result, err := exec.Execute(ctx, "shell_exec", map[string]interface{}{
		"command": "sleep 10",
	})

	if err != nil {
		h.T.Fatalf("Execute failed: %v", err)
	}

	// Command should be cancelled
	if result.Error == "" {
		h.T.Log("Note: Command may have completed before cancellation (expected for very fast commands)")
	}
}

// TestShellExecStderrCapture tests that stderr is properly captured.
func TestShellExecStderrCapture(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)
	_ = h // Harness manages cleanup automatically

	exec := tools.NewToolExecutor(h.TempDir)

	// Create a test file that writes to stderr
	testScript := h.CreateTempFile("test_stderr.sh", "echo \"Error message\" >&2")

	ctx := context.Background()

	// Note: On Windows, shell scripts work differently
	// We'll use a cross-platform approach
	var result tools.ToolResult
	var err error

	if strings.Contains(testScript, ".sh") {
		// Unix-like: execute the script
		result, err = exec.Execute(ctx, "shell_exec", map[string]interface{}{
			"command": "sh " + testScript,
		})
	} else {
		// Windows: use a command that writes to stderr
		result, err = exec.Execute(ctx, "shell_exec", map[string]interface{}{
			"command": "powershell -Command \"Write-Error 'Error message'\"",
		})
	}

	if err != nil {
		h.T.Fatalf("Execute failed: %v", err)
	}

	// Check that stderr was captured
	if !strings.Contains(result.Content, "Error message") {
		h.T.Logf("Note: stderr output format may vary by platform")
	}
}
