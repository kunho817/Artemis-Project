package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileTool writes content to a file (creates parent directories if needed).
// Uses atomic writes (temp→rename) and file locking for concurrent safety.
type WriteFileTool struct {
	baseDir  string
	fileLock *FileLockManager
}

func (t *WriteFileTool) Name() string { return "write_file" }
func (t *WriteFileTool) Description() string {
	return "Write content to a file (creates or overwrites). Uses atomic writes for crash safety."
}
func (t *WriteFileTool) Parameters() string {
	return "path (string, required) — relative file path; content (string, required) — file content"
}

func (t *WriteFileTool) Execute(ctx context.Context, params map[string]interface{}) (ToolResult, error) {
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return ToolResult{Error: "missing required parameter: path"}, nil
	}

	content, ok := params["content"].(string)
	if !ok {
		return ToolResult{Error: "missing required parameter: content"}, nil
	}

	fullPath := filepath.Join(t.baseDir, filepath.Clean(path))
	if !isInsideDir(t.baseDir, fullPath) {
		return ToolResult{Error: "path outside project directory"}, nil
	}

	// Acquire file lock to prevent concurrent writes
	if t.fileLock != nil {
		t.fileLock.Lock(fullPath)
		defer t.fileLock.Unlock(fullPath)
	}

	// Create parent directories
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return ToolResult{Error: fmt.Sprintf("failed to create directory: %s", err)}, nil
	}

	// Check if file is new or existing
	_, statErr := os.Stat(fullPath)
	isNew := os.IsNotExist(statErr)

	// Create backup of existing file before overwriting
	if !isNew {
		if err := createBackup(fullPath); err != nil {
			// Non-fatal: warn but continue
			_ = err
		}
	}

	// Atomic write: temp file → rename
	if err := atomicWriteFile(fullPath, []byte(content), 0644); err != nil {
		return ToolResult{Error: fmt.Sprintf("failed to write file: %s", err)}, nil
	}

	action := "updated"
	if isNew {
		action = "created"
	}

	return ToolResult{
		Content:      fmt.Sprintf("File %s: %s", action, path),
		FilesChanged: []string{path},
	}, nil
}

// createBackup saves a backup copy of the file before overwriting.
// Backup is stored as <filename>.artemis-backup in the same directory.
func createBackup(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	backupPath := path + ".artemis-backup"
	return os.WriteFile(backupPath, data, 0644)
}
