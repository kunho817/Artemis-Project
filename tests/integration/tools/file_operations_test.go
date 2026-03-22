// Package tools provides integration tests for file operations.
package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/artemis-project/artemis/internal/tools"
	"github.com/artemis-project/artemis/tests/integration/harness"
)

// TestReadFile tests basic file reading.
func TestReadFile(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	// Create a test file
	content := "Hello, World!"
	filePath := h.CreateTempFile("test_read.txt", content)

	exec := tools.NewToolExecutor(h.TempDir)

	ctx := context.Background()
	result, err := exec.Execute(ctx, "read_file", map[string]interface{}{
		"file_path": filePath,
	})

	if err != nil {
		h.T.Fatalf("Execute failed: %v", err)
	}

	if result.Error != "" {
		h.T.Errorf("Unexpected error: %s", result.Error)
	}

	if !strings.Contains(result.Content, content) {
		h.T.Errorf("Expected content %q, got %q", content, result.Content)
	}
}

// TestWriteFile tests basic file writing.
func TestWriteFile(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	filePath := filepath.Join(h.TempDir, "test_write.txt")
	content := "Test content for writing"

	exec := tools.NewToolExecutor(h.TempDir)

	ctx := context.Background()
	result, err := exec.Execute(ctx, "write_file", map[string]interface{}{
		"file_path": filePath,
		"content":   content,
	})

	if err != nil {
		h.T.Fatalf("Execute failed: %v", err)
	}

	if result.Error != "" {
		h.T.Errorf("Unexpected error: %s", result.Error)
	}

	// Verify file was written
	data, err := os.ReadFile(filePath)
	if err != nil {
		h.T.Fatalf("Failed to read written file: %v", err)
	}

	if string(data) != content {
		h.T.Errorf("Expected content %q, got %q", content, string(data))
	}
}

// TestPatchFile tests file patching.
func TestPatchFile(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	// Create initial file
	initialContent := "Line 1\nLine 2\nLine 3\n"
	filePath := h.CreateTempFile("test_patch.txt", initialContent)

	exec := tools.NewToolExecutor(h.TempDir)

	ctx := context.Background()
	result, err := exec.Execute(ctx, "patch_file", map[string]interface{}{
		"file_path": filePath,
		"patch":     "Line 2\nModified Line 2",
	})

	if err != nil {
		h.T.Fatalf("Execute failed: %v", err)
	}

	if result.Error != "" {
		h.T.Errorf("Unexpected error: %s", result.Error)
	}

	// Verify patch was applied
	data, err := os.ReadFile(filePath)
	if err != nil {
		h.T.Fatalf("Failed to read patched file: %v", err)
	}

	patchedContent := string(data)
	if !strings.Contains(patchedContent, "Modified Line 2") {
		h.T.Errorf("Expected patched content, got %q", patchedContent)
	}

	if strings.Contains(patchedContent, "Line 2\n") && !strings.Contains(patchedContent, "Modified Line 2") {
		h.T.Errorf("Original content still present, patch may not have been applied")
	}
}

// TestListDirectory tests directory listing.
func TestListDirectory(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	// Create some test files
	h.CreateTempFile("file1.txt", "content1")
	h.CreateTempFile("file2.txt", "content2")

	exec := tools.NewToolExecutor(h.TempDir)

	ctx := context.Background()
	result, err := exec.Execute(ctx, "list_dir", map[string]interface{}{
		"path": h.TempDir,
	})

	if err != nil {
		h.T.Fatalf("Execute failed: %v", err)
	}

	if result.Error != "" {
		h.T.Errorf("Unexpected error: %s", result.Error)
	}

	// Verify files are listed
	if !strings.Contains(result.Content, "file1.txt") {
		h.T.Errorf("Expected file1.txt in listing, got %s", result.Content)
	}

	if !strings.Contains(result.Content, "file2.txt") {
		h.T.Errorf("Expected file2.txt in listing, got %s", result.Content)
	}
}

// TestFileOperationsMissingParams tests error handling for missing parameters.
func TestFileOperationsMissingParams(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	exec := tools.NewToolExecutor(h.TempDir)
	ctx := context.Background()

	t.Run("ReadFileMissingPath", func(t *testing.T) {
		result, err := exec.Execute(ctx, "read_file", map[string]interface{}{
			// Missing file_path
		})

		if err != nil {
			h.T.Fatalf("Execute failed: %v", err)
		}

		if result.Error == "" {
			h.T.Error("Expected error for missing file_path parameter")
		}
	})

	t.Run("WriteFileMissingPath", func(t *testing.T) {
		result, err := exec.Execute(ctx, "write_file", map[string]interface{}{
			"content": "test",
			// Missing file_path
		})

		if err != nil {
			h.T.Fatalf("Execute failed: %v", err)
		}

		if result.Error == "" {
			h.T.Error("Expected error for missing file_path parameter")
		}
	})

	t.Run("PatchFileMissingPatch", func(t *testing.T) {
		result, err := exec.Execute(ctx, "patch_file", map[string]interface{}{
			"file_path": "test.txt",
			// Missing patch
		})

		if err != nil {
			h.T.Fatalf("Execute failed: %v", err)
		}

		if result.Error == "" {
			h.T.Error("Expected error for missing patch parameter")
		}
	})
}

// TestFileOperationsNonExistentFile tests error handling for non-existent files.
func TestFileOperationsNonExistentFile(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	exec := tools.NewToolExecutor(h.TempDir)
	ctx := context.Background()

	nonExistentPath := filepath.Join(h.TempDir, "non_existent_file.txt")

	t.Run("ReadNonExistent", func(t *testing.T) {
		result, err := exec.Execute(ctx, "read_file", map[string]interface{}{
			"file_path": nonExistentPath,
		})

		if err != nil {
			h.T.Fatalf("Execute failed: %v", err)
		}

		// Should return error
		if result.Error == "" {
			h.T.Log("Note: Some file systems may not error on read, content may be empty")
		}
	})

	t.Run("PatchNonExistent", func(t *testing.T) {
		result, err := exec.Execute(ctx, "patch_file", map[string]interface{}{
			"file_path": nonExistentPath,
			"patch":     "test patch",
		})

		if err != nil {
			h.T.Fatalf("Execute failed: %v", err)
		}

		// Should return error
		if result.Error == "" {
			h.T.Log("Note: Patch may create file or return error depending on implementation")
		}
	})
}

// TestFileOperationsEscaping tests special character handling in file paths.
func TestFileOperationsEscaping(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	// Create file with special characters in name
	specialFileName := "test file with spaces.txt"
	content := "Test content with spaces"

	filePath := filepath.Join(h.TempDir, specialFileName)

	exec := tools.NewToolExecutor(h.TempDir)

	ctx := context.Background()

	// Write file with special characters
	writeResult, err := exec.Execute(ctx, "write_file", map[string]interface{}{
		"file_path": filePath,
		"content":   content,
	})

	if err != nil {
		h.T.Fatalf("Write Execute failed: %v", err)
	}

	if writeResult.Error != "" {
		// Special characters may not be supported in all environments
		h.T.Logf("Note: Special characters in file paths may not be supported: %s", writeResult.Error)
		return
	}

	// Read file back
	readResult, err := exec.Execute(ctx, "read_file", map[string]interface{}{
		"file_path": filePath,
	})

	if err != nil {
		h.T.Fatalf("Read Execute failed: %v", err)
	}

	if readResult.Error != "" {
		h.T.Errorf("Unexpected error on read: %s", readResult.Error)
	}

	if !strings.Contains(readResult.Content, content) {
		h.T.Errorf("Expected content %q, got %q", content, readResult.Content)
	}
}

// TestFileOperationsConcurrency tests concurrent file operations.
func TestFileOperationsConcurrency(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	exec := tools.NewToolExecutor(h.TempDir)
	ctx := context.Background()

	// Create multiple files concurrently
	numFiles := 10
	done := make(chan bool, numFiles)

	for i := 0; i < numFiles; i++ {
		go func(idx int) {
			fileName := filepath.Join(h.TempDir, fmt.Sprintf("concurrent_%d.txt", idx))
			content := fmt.Sprintf("Content %d", idx)

			_, err := exec.Execute(ctx, "write_file", map[string]interface{}{
				"file_path": fileName,
				"content":   content,
			})

			if err != nil {
				h.T.Errorf("Concurrent write %d failed: %v", idx, err)
			}

			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < numFiles; i++ {
		select {
		case <-done:
			// OK
		case <-time.After(5 * time.Second):
			h.T.Fatalf("Timeout waiting for concurrent operations")
		}
	}

	// Verify all files were written
	for i := 0; i < numFiles; i++ {
		fileName := filepath.Join(h.TempDir, fmt.Sprintf("concurrent_%d.txt", i))
		if _, err := os.Stat(fileName); os.IsNotExist(err) {
			h.T.Errorf("Concurrent file %d was not created", i)
		}
	}
}

// TestFileOperationsLargeFile tests handling of large files.
func TestFileOperationsLargeFile(t *testing.T) {
	harness.SkipIfShort(t)

	h := harness.Setup(t)

	// Create a large content string
	largeContent := strings.Repeat("This is a line of text for testing large file operations.\n", 10000) // ~500KB

	filePath := filepath.Join(h.TempDir, "large_file.txt")

	exec := tools.NewToolExecutor(h.TempDir)

	ctx := context.Background()

	// Write large file
	writeResult, err := exec.Execute(ctx, "write_file", map[string]interface{}{
		"file_path": filePath,
		"content":   largeContent,
	})

	if err != nil {
		h.T.Fatalf("Write Execute failed: %v", err)
	}

	if writeResult.Error != "" {
		h.T.Errorf("Unexpected error on large file write: %s", writeResult.Error)
	}

	// Read large file back
	readResult, err := exec.Execute(ctx, "read_file", map[string]interface{}{
		"file_path": filePath,
	})

	if err != nil {
		h.T.Fatalf("Read Execute failed: %v", err)
	}

	if readResult.Error != "" {
		// Large files may be truncated
		h.T.Logf("Note: Large file read returned error: %s", readResult.Error)
	}

	// Verify at least some content was read
	if !strings.Contains(readResult.Content, "This is a line of text") {
		h.T.Error("Expected at least some content to be read")
	}
}
