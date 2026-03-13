package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artemis-project/artemis/internal/orchestrator"
)

// RecoveryRequest carries a recovery prompt from the Engine goroutine to the TUI.
// The ReplyCh is a one-shot buffered channel for the user's decision.
type RecoveryRequest struct {
	Context orchestrator.RecoveryContext
	ReplyCh chan<- orchestrator.RecoveryAction // buffered(1)
}

// RecoveryRequestMsg wraps a RecoveryRequest as a bubbletea message.
type RecoveryRequestMsg struct {
	Request RecoveryRequest
}

// RecoveryBridge implements orchestrator.RecoveryPrompter using channels.
// It bridges the Engine goroutine (blocking) with the TUI event loop (async).
// The bridge is stored as a pointer in App so it survives bubbletea model copies.
type RecoveryBridge struct {
	requestCh chan RecoveryRequest
}

// NewRecoveryBridge creates a new bridge with a buffered request channel.
func NewRecoveryBridge() *RecoveryBridge {
	return &RecoveryBridge{
		requestCh: make(chan RecoveryRequest, 1),
	}
}

// Prompt implements orchestrator.RecoveryPrompter.
// Called from the Engine goroutine — blocks until the user decides or context cancels.
func (b *RecoveryBridge) Prompt(ctx context.Context, rc orchestrator.RecoveryContext) (orchestrator.RecoveryAction, error) {
	replyCh := make(chan orchestrator.RecoveryAction, 1)

	// Send request to TUI
	select {
	case b.requestCh <- RecoveryRequest{Context: rc, ReplyCh: replyCh}:
	case <-ctx.Done():
		return orchestrator.ActionAbort, ctx.Err()
	}

	// Block until user decides
	select {
	case action := <-replyCh:
		return action, nil
	case <-ctx.Done():
		return orchestrator.ActionAbort, ctx.Err()
	}
}

// waitForRecoveryRequest returns a tea.Cmd that listens for recovery requests
// from the Engine goroutine. Called alongside waitForEvent() via tea.Batch().
func waitForRecoveryRequest(bridge *RecoveryBridge) tea.Cmd {
	if bridge == nil {
		return nil
	}
	return func() tea.Msg {
		req, ok := <-bridge.requestCh
		if !ok {
			return nil // bridge closed
		}
		return RecoveryRequestMsg{Request: req}
	}
}
