package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/artemis-project/artemis/internal/bus"
	"github.com/artemis-project/artemis/internal/state"
)

// TaskStatus represents the lifecycle state of a background task.
type TaskStatus string

const (
	TaskPending   TaskStatus = "pending"
	TaskRunning   TaskStatus = "running"
	TaskCompleted TaskStatus = "completed"
	TaskFailed    TaskStatus = "failed"
	TaskCancelled TaskStatus = "cancelled"
)

// BackgroundTask tracks the state and result of a single background task.
type BackgroundTask struct {
	ID          string
	Description string
	AgentRole   string
	Status      TaskStatus
	Result      string
	Error       error
	StartedAt   time.Time
	CompletedAt time.Time
	cancel      context.CancelFunc // per-task cancellation
}

// Duration returns how long the task has been running (or ran).
func (t *BackgroundTask) Duration() time.Duration {
	if t.CompletedAt.IsZero() {
		return time.Since(t.StartedAt)
	}
	return t.CompletedAt.Sub(t.StartedAt)
}

// BackgroundTaskManager manages goroutine-based background tasks that run
// parallel to the main pipeline. Tasks emit events to the shared EventBus
// for inline TUI status display.
type BackgroundTaskManager struct {
	mu       sync.RWMutex
	tasks    map[string]*BackgroundTask
	eventBus *bus.EventBus
	wg       sync.WaitGroup
}

// NewBackgroundTaskManager creates a new manager with the given event bus.
func NewBackgroundTaskManager(eb *bus.EventBus) *BackgroundTaskManager {
	return &BackgroundTaskManager{
		tasks:    make(map[string]*BackgroundTask),
		eventBus: eb,
	}
}

// Spawn launches a background task as a goroutine. The agent is created via
// buildAgent, configured with the given task description, and runs against
// an isolated SessionState containing the user's request.
//
// The agent's streaming events (chunks, output) flow through the shared
// EventBus and appear in the TUI alongside main pipeline events.
func (m *BackgroundTaskManager) Spawn(
	parentCtx context.Context,
	def BackgroundTaskDef,
	buildAgent AgentBuilder,
	userRequest string,
) {
	// Create per-task cancellable context
	ctx, cancel := context.WithCancel(parentCtx)

	task := &BackgroundTask{
		ID:          def.ID,
		Description: def.Task,
		AgentRole:   def.Agent,
		Status:      TaskPending,
		cancel:      cancel,
	}

	m.mu.Lock()
	m.tasks[def.ID] = task
	m.mu.Unlock()

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		m.runTask(ctx, task, def, buildAgent, userRequest)
	}()
}

// runTask executes a single background task: creates the agent, runs it,
// tracks state transitions, and emits lifecycle events.
func (m *BackgroundTaskManager) runTask(
	ctx context.Context,
	task *BackgroundTask,
	def BackgroundTaskDef,
	buildAgent AgentBuilder,
	userRequest string,
) {
	// Build the agent via AgentTask
	agentTask := AgentTask{
		Agent:    def.Agent,
		Task:     def.Task,
		Critical: false, // background tasks are never pipeline-critical
	}
	ag := buildAgent(agentTask)
	if ag == nil {
		m.setFailed(task, fmt.Errorf("no provider available for %s", def.Agent))
		return
	}

	// Mark running
	m.mu.Lock()
	task.Status = TaskRunning
	task.StartedAt = time.Now()
	m.mu.Unlock()
	m.emitStart(task)

	// Create isolated SessionState for this background task
	// Phase 5: Background tasks get their own run ID, linked to parent
	bgRunID := fmt.Sprintf("bgrun_%s_%d", def.ID, time.Now().UnixNano())
	ss := state.NewSessionStateWithID(bgRunID, "", "")
	ss.SetPhase("background")
	ss.AddArtifact(state.Artifact{
		Type:    state.ArtifactUserRequest,
		Source:  "user",
		Content: userRequest,
	})

	// Run the agent — streaming events emit to shared EventBus automatically
	err := ag.Run(ctx, ss)
	if err != nil {
		if ctx.Err() != nil {
			m.setCancelled(task)
		} else {
			m.setFailed(task, err)
		}
		return
	}

	// Extract result from SessionState artifacts
	var result string
	artifacts := ss.GetBySource(ag.Name())
	if len(artifacts) > 0 {
		result = artifacts[len(artifacts)-1].Content
	}

	m.setCompleted(task, result)
}

// Cancel cancels a specific background task by ID.
func (m *BackgroundTaskManager) Cancel(id string) {
	m.mu.RLock()
	task, ok := m.tasks[id]
	m.mu.RUnlock()
	if ok && task.cancel != nil && task.Status == TaskRunning {
		task.cancel()
	}
}

// CancelAll cancels all running background tasks.
func (m *BackgroundTaskManager) CancelAll() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, task := range m.tasks {
		if task.Status == TaskRunning && task.cancel != nil {
			task.cancel()
		}
	}
}

// WaitAll blocks until all spawned background tasks have completed.
func (m *BackgroundTaskManager) WaitAll() {
	m.wg.Wait()
}

// GetTask returns a snapshot of a background task by ID.
func (m *BackgroundTaskManager) GetTask(id string) *BackgroundTask {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if t, ok := m.tasks[id]; ok {
		// Return a copy to avoid race
		cp := *t
		return &cp
	}
	return nil
}

// GetResult returns the result and error for a completed task.
func (m *BackgroundTaskManager) GetResult(id string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if t, ok := m.tasks[id]; ok {
		return t.Result, t.Error
	}
	return "", fmt.Errorf("unknown task %q", id)
}

// AllTasks returns snapshots of all background tasks.
func (m *BackgroundTaskManager) AllTasks() []*BackgroundTask {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]*BackgroundTask, 0, len(m.tasks))
	for _, t := range m.tasks {
		cp := *t
		result = append(result, &cp)
	}
	return result
}

// TaskCount returns the number of tracked background tasks.
func (m *BackgroundTaskManager) TaskCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.tasks)
}

// --- State transitions + event emission ---

func (m *BackgroundTaskManager) setCompleted(task *BackgroundTask, result string) {
	m.mu.Lock()
	task.Status = TaskCompleted
	task.Result = result
	task.CompletedAt = time.Now()
	m.mu.Unlock()
	m.emitComplete(task)
}

func (m *BackgroundTaskManager) setFailed(task *BackgroundTask, err error) {
	m.mu.Lock()
	task.Status = TaskFailed
	task.Error = err
	task.CompletedAt = time.Now()
	m.mu.Unlock()
	m.emitFail(task, err)
}

func (m *BackgroundTaskManager) setCancelled(task *BackgroundTask) {
	m.mu.Lock()
	task.Status = TaskCancelled
	task.CompletedAt = time.Now()
	m.mu.Unlock()
	// No event for cancellation — context already handled
}

// --- Event emission ---

func (m *BackgroundTaskManager) emitStart(task *BackgroundTask) {
	if m.eventBus != nil {
		e := bus.NewEvent(bus.EventBackgroundTaskStart, task.AgentRole, "background", task.Description)
		e.Data = task.ID
		m.eventBus.Emit(e)
	}
}

func (m *BackgroundTaskManager) emitComplete(task *BackgroundTask) {
	if m.eventBus != nil {
		msg := fmt.Sprintf("done (%.1fs)", task.Duration().Seconds())
		e := bus.NewEvent(bus.EventBackgroundTaskComplete, task.AgentRole, "background", msg)
		e.Data = task.ID
		m.eventBus.Emit(e)
	}
}

func (m *BackgroundTaskManager) emitFail(task *BackgroundTask, err error) {
	if m.eventBus != nil {
		e := bus.NewEvent(bus.EventBackgroundTaskFail, task.AgentRole, "background", err.Error())
		e.Data = task.ID
		m.eventBus.Emit(e)
	}
}

// BackgroundTaskDef defines a background task in an ExecutionPlan.
// Background tasks run in parallel with the main pipeline steps.
type BackgroundTaskDef struct {
	ID    string `json:"id"`
	Agent string `json:"agent"` // Agent role (e.g., "scout", "consultant")
	Task  string `json:"task"`  // Task description
}
