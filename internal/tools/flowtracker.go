package tools

import (
	"context"
	"sort"
	"sync"
	"time"
)

// FlowTracker tracks tool usage patterns to enable smart context injection.
// Inspired by Windsurf's "Flows" — maintains awareness of what files
// the developer is actively working on.
type FlowTracker struct {
	mu          sync.RWMutex
	fileAccess  map[string]*fileActivity // path → activity
	recentEdits []editRecord             // ordered by time, newest first
	maxRecent   int
}

type fileActivity struct {
	ReadCount  int
	WriteCount int
	LastAccess time.Time
}

type editRecord struct {
	FilePath string
	ToolName string // write_file, patch_file, etc.
	Time     time.Time
}

// NewFlowTracker creates a new flow tracker.
func NewFlowTracker() *FlowTracker {
	return &FlowTracker{
		fileAccess: make(map[string]*fileActivity),
		maxRecent:  50,
	}
}

// RecordAccess records a file access from a tool call.
func (ft *FlowTracker) RecordAccess(toolName string, filePath string) {
	if filePath == "" {
		return
	}

	ft.mu.Lock()
	defer ft.mu.Unlock()

	activity, ok := ft.fileAccess[filePath]
	if !ok {
		activity = &fileActivity{}
		ft.fileAccess[filePath] = activity
	}

	activity.LastAccess = time.Now()

	switch toolName {
	case "write_file", "patch_file":
		activity.WriteCount++
		ft.recentEdits = append([]editRecord{{
			FilePath: filePath,
			ToolName: toolName,
			Time:     time.Now(),
		}}, ft.recentEdits...)
		if len(ft.recentEdits) > ft.maxRecent {
			ft.recentEdits = ft.recentEdits[:ft.maxRecent]
		}
	case "read_file":
		activity.ReadCount++
	}
}

// RecentlyEditedFiles returns file paths edited most recently (deduped).
func (ft *FlowTracker) RecentlyEditedFiles(limit int) []string {
	ft.mu.RLock()
	defer ft.mu.RUnlock()

	seen := map[string]bool{}
	var result []string
	for _, r := range ft.recentEdits {
		if seen[r.FilePath] {
			continue
		}
		seen[r.FilePath] = true
		result = append(result, r.FilePath)
		if len(result) >= limit {
			break
		}
	}
	return result
}

// MostAccessedFiles returns files ranked by total access count.
func (ft *FlowTracker) MostAccessedFiles(limit int) []string {
	ft.mu.RLock()
	defer ft.mu.RUnlock()

	type entry struct {
		path  string
		score int
	}
	var entries []entry
	for path, a := range ft.fileAccess {
		entries = append(entries, entry{path, a.ReadCount + a.WriteCount*3}) // writes weighted 3x
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].score > entries[j].score
	})

	var result []string
	for i, e := range entries {
		if i >= limit {
			break
		}
		result = append(result, e.path)
	}
	return result
}

// LastEditedFile returns the most recently edited file path, or "".
func (ft *FlowTracker) LastEditedFile() string {
	ft.mu.RLock()
	defer ft.mu.RUnlock()
	if len(ft.recentEdits) == 0 {
		return ""
	}
	return ft.recentEdits[0].FilePath
}

// FormatFlowContext returns a summary of recent activity for prompt injection.
func (ft *FlowTracker) FormatFlowContext() string {
	recent := ft.RecentlyEditedFiles(5)
	if len(recent) == 0 {
		return ""
	}

	result := "Recently edited files:\n"
	for _, f := range recent {
		result += "- " + f + "\n"
	}
	return result
}

// FlowAwareHook returns a PostHook that records file access patterns.
func FlowAwareHook(ft *FlowTracker) HookFunc {
	return func(ctx context.Context, toolName string, params map[string]interface{}, result *ToolResult) (bool, string) {
		if result == nil {
			return true, ""
		}

		// Record file access from params
		if path, ok := params["path"].(string); ok {
			ft.RecordAccess(toolName, path)
		}
		if path, ok := params["file"].(string); ok {
			ft.RecordAccess(toolName, path)
		}

		// Record all changed files
		for _, f := range result.FilesChanged {
			ft.RecordAccess(toolName, f)
		}

		return true, ""
	}
}
