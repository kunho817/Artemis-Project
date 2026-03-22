// Package process provides enhanced process termination utilities.
// Implements SIGTERM → SIGKILL graceful shutdown with timeout support.
package process

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"time"
)

// ProcessKiller manages graceful and forceful process termination.
// Implements SIGTERM → SIGKILL pattern with configurable timeout.
type ProcessKiller struct {
	timeout     time.Duration // SIGTERM → SIGKILL wait duration
	forceKill   bool          // Whether to force kill after timeout
	shutdownCh  chan struct{} // Shutdown signal channel
	mu          sync.Mutex    // Protects concurrent kills
}

// NewProcessKiller creates a new ProcessKiller with specified timeout.
// If timeout is 0, DefaultKillTimeout (10 seconds) is used.
func NewProcessKiller(timeout time.Duration) *ProcessKiller {
	if timeout == 0 {
		timeout = DefaultKillTimeout
	}
	return &ProcessKiller{
		timeout:    timeout,
		forceKill:  true,
		shutdownCh: make(chan struct{}),
	}
}

// DefaultKillTimeout is the default SIGTERM → SIGKILL wait duration.
const DefaultKillTimeout = 10 * time.Second

// Kill terminates a process gracefully with SIGTERM, then SIGKILL if needed.
// Returns nil if process was terminated successfully, error otherwise.
func (pk *ProcessKiller) Kill(ctx context.Context, cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil // No process to kill
	}

	pk.mu.Lock()
	defer pk.mu.Unlock()

	select {
	case <-pk.shutdownCh:
		return fmt.Errorf("process killer is shut down")
	default:
	}

	// Try graceful shutdown first (SIGTERM)
	if err := pk.gracefulShutdown(cmd.Process); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}

	// Wait for process to exit with timeout
	done := make(chan error, 1)
	go func() {
		_, err := cmd.Process.Wait()
		done <- err
	}()

	select {
	case err := <-done:
		// Process exited gracefully
		return err
	case <-time.After(pk.timeout):
		// Timeout - force kill if enabled
		if pk.forceKill {
			return pk.forceKillProcess(cmd.Process)
		}
		return fmt.Errorf("process did not exit within timeout (force kill disabled)")
	case <-ctx.Done():
		// Context cancelled
		if pk.forceKill {
			_ = pk.forceKillProcess(cmd.Process)
		}
		return ctx.Err()
	}
}

// KillCommand is a convenience method that kills a process and handles cleanup.
// It closes stdin/stdout/stderr and waits for the process to exit.
func (pk *ProcessKiller) KillCommand(ctx context.Context, cmd *exec.Cmd) error {
	if cmd == nil {
		return nil
	}

	// Close pipes first to prevent blocking
	if cmd.Stdin != nil {
		if closer, ok := cmd.Stdin.(interface{ Close() }); ok {
			closer.Close()
		}
	}
	if cmd.Stdout != nil {
		if closer, ok := cmd.Stdout.(interface{ Close() }); ok {
			closer.Close()
		}
	}
	if cmd.Stderr != nil {
		if closer, ok := cmd.Stderr.(interface{ Close() }); ok {
			closer.Close()
		}
	}

	// Kill the process
	if err := pk.Kill(ctx, cmd); err != nil {
		return err
	}

	// Release resources
	cmd.Process = nil
	return nil
}

// gracefulShutdown sends SIGTERM (or SIGINT on Windows) to the process.
func (pk *ProcessKiller) gracefulShutdown(process *os.Process) error {
	if process == nil {
		return nil
	}

	if runtime.GOOS == "windows" {
		// Windows: Use taskkill /T to terminate process tree
		// This is more reliable than sending signals
		return pk.windowsKillTree(process.Pid)
	}

	// Unix: Send SIGTERM for graceful shutdown
	return process.Signal(syscall.SIGTERM)
}

// forceKillProcess sends SIGKILL (or taskkill /F on Windows) to the process.
func (pk *ProcessKiller) forceKillProcess(process *os.Process) error {
	if process == nil {
		return nil
	}

	if runtime.GOOS == "windows" {
		// Windows: Force kill with taskkill /F
		return pk.windowsForceKill(process.Pid)
	}

	// Unix: Send SIGKILL for immediate termination
	return process.Signal(syscall.SIGKILL)
}

// windowsKillTree terminates a process tree on Windows using taskkill.
// This ensures child processes are also terminated.
func (pk *ProcessKiller) windowsKillTree(pid int) error {
	// taskkill /T: terminate process tree
	// taskkill /PID: specify process ID
	cmd := exec.Command("taskkill", "/T", "/PID", fmt.Sprintf("%d", pid))
	_ = cmd.Run() // Ignore error, process may already be dead
	return nil
}

// windowsForceKill forcefully terminates a process on Windows.
func (pk *ProcessKiller) windowsForceKill(pid int) error {
	// taskkill /F: force terminate
	// taskkill /T: terminate process tree
	cmd := exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", pid))
	_ = cmd.Run() // Ignore error, process may already be dead
	return nil
}

// Shutdown stops the ProcessKiller and prevents further kills.
func (pk *ProcessKiller) Shutdown() {
	pk.mu.Lock()
	defer pk.mu.Unlock()
	select {
	case <-pk.shutdownCh:
		// Already shut down
	default:
		close(pk.shutdownCh)
	}
}

// SetForceKill enables or disables force kill after timeout.
func (pk *ProcessKiller) SetForceKill(force bool) {
	pk.mu.Lock()
	defer pk.mu.Unlock()
	pk.forceKill = force
}

// SetTimeout updates the kill timeout.
func (pk *ProcessKiller) SetTimeout(timeout time.Duration) {
	pk.mu.Lock()
	defer pk.mu.Unlock()
	if timeout > 0 {
		pk.timeout = timeout
	}
}

// GetTimeout returns the current kill timeout.
func (pk *ProcessKiller) GetTimeout() time.Duration {
	pk.mu.Lock()
	defer pk.mu.Unlock()
	return pk.timeout
}

// KillWithTimeout kills a process with a custom timeout.
// This is a convenience function that creates a temporary ProcessKiller.
func KillWithTimeout(ctx context.Context, cmd *exec.Cmd, timeout time.Duration) error {
	pk := NewProcessKiller(timeout)
	return pk.KillCommand(ctx, cmd)
}

// KillImmediate forcefully kills a process without waiting.
// Equivalent to KillWithTimeout with 0 timeout.
func KillImmediate(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	if runtime.GOOS == "windows" {
		pk := NewProcessKiller(0)
		return pk.forceKillProcess(cmd.Process)
	}

	return cmd.Process.Kill()
}
