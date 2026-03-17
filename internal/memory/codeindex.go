package memory

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

// CodeIndex builds and maintains a semantic index of code files.
// It chunks source files by function/type boundaries and stores
// embeddings in the VectorStore for similarity search.
type CodeIndex struct {
	vectorStore *VectorStore
	rootPath    string
	exclude     []string // glob patterns to exclude
	indexed     int64    // atomic counter of indexed chunks

	mu           sync.Mutex
	indexedFiles map[string]string // filepath → content hash (for incremental)
}

// NewCodeIndex creates a new code indexer.
func NewCodeIndex(vs *VectorStore, rootPath string, exclude []string) *CodeIndex {
	return &CodeIndex{
		vectorStore:  vs,
		rootPath:     rootPath,
		exclude:      exclude,
		indexedFiles: make(map[string]string),
	}
}

// IndexDirectory walks a directory and indexes all supported source files.
func (ci *CodeIndex) IndexDirectory(ctx context.Context, dirPath string) (int, error) {
	total := 0

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Skip directories
		if info.IsDir() {
			name := info.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" || name == ".artemis" {
				return filepath.SkipDir
			}
			for _, pat := range ci.exclude {
				if ok, _ := filepath.Match(pat, name); ok {
					return filepath.SkipDir
				}
			}
			return nil
		}

		// Only index supported file types
		if !isSupportedCodeFile(path) {
			return nil
		}

		// Skip large files (>100KB)
		if info.Size() > 100*1024 {
			return nil
		}

		n, err := ci.IndexFile(ctx, path)
		if err == nil {
			total += n
		}
		return nil
	})

	return total, err
}

// IndexFile chunks a single file and adds chunks to the vector store.
// Returns the number of chunks created.
func (ci *CodeIndex) IndexFile(ctx context.Context, filePath string) (int, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return 0, err
	}

	content := string(data)
	relPath, _ := filepath.Rel(ci.rootPath, filePath)
	if relPath == "" {
		relPath = filePath
	}

	chunks := chunkCode(relPath, content)
	if len(chunks) == 0 {
		return 0, nil
	}

	indexed := 0
	for _, chunk := range chunks {
		if ctx.Err() != nil {
			break
		}

		err := ci.vectorStore.AddCodeChunk(ctx, CodeChunk{
			ID:       chunk.id,
			FilePath: relPath,
			Content:  chunk.content,
		})
		if err != nil {
			continue // skip failed chunks
		}
		indexed++
		atomic.AddInt64(&ci.indexed, 1)
	}

	ci.mu.Lock()
	ci.indexedFiles[relPath] = fmt.Sprintf("%d", len(data))
	ci.mu.Unlock()

	return indexed, nil
}

// Search finds code chunks semantically similar to a query.
func (ci *CodeIndex) Search(ctx context.Context, query string, limit int) ([]CodeChunk, error) {
	if ci.vectorStore == nil {
		return nil, nil
	}
	return ci.vectorStore.QueryCodeChunks(ctx, query, limit)
}

// IndexedChunkCount returns the total number of indexed chunks.
func (ci *CodeIndex) IndexedChunkCount() int {
	return int(atomic.LoadInt64(&ci.indexed))
}

// IndexedFileCount returns the number of files that have been indexed.
func (ci *CodeIndex) IndexedFileCount() int {
	ci.mu.Lock()
	defer ci.mu.Unlock()
	return len(ci.indexedFiles)
}

// --- Code Chunking ---

type codeChunk struct {
	id      string
	content string
}

// chunkCode splits source code into meaningful chunks.
// Strategy: split on function/type boundaries, with context overlap.
func chunkCode(filePath, content string) []codeChunk {
	lines := strings.Split(content, "\n")
	if len(lines) <= 20 {
		// Small file: single chunk
		return []codeChunk{{
			id:      fmt.Sprintf("%s:1-%d", filePath, len(lines)),
			content: fmt.Sprintf("// File: %s\n%s", filePath, content),
		}}
	}

	// Split on function/type boundaries
	var chunks []codeChunk
	ext := strings.ToLower(filepath.Ext(filePath))

	boundaries := findBoundaries(lines, ext)
	if len(boundaries) == 0 {
		// No boundaries found — chunk by fixed size
		return chunkBySize(filePath, lines, 60)
	}

	// Create chunks between boundaries
	for i, start := range boundaries {
		end := len(lines)
		if i+1 < len(boundaries) {
			end = boundaries[i+1]
		}

		// Cap chunk size at 80 lines
		if end-start > 80 {
			end = start + 80
		}

		chunkLines := lines[start:end]
		chunkContent := strings.Join(chunkLines, "\n")
		if strings.TrimSpace(chunkContent) == "" {
			continue
		}

		chunks = append(chunks, codeChunk{
			id:      fmt.Sprintf("%s:%d-%d", filePath, start+1, end),
			content: fmt.Sprintf("// File: %s (lines %d-%d)\n%s", filePath, start+1, end, chunkContent),
		})
	}

	return chunks
}

// findBoundaries returns line indices where functions/types/classes start.
func findBoundaries(lines []string, ext string) []int {
	var boundaries []int

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		switch ext {
		case ".go":
			if strings.HasPrefix(trimmed, "func ") || strings.HasPrefix(trimmed, "type ") {
				boundaries = append(boundaries, i)
			}
		case ".ts", ".tsx", ".js", ".jsx":
			if strings.HasPrefix(trimmed, "function ") ||
				strings.HasPrefix(trimmed, "export function ") ||
				strings.HasPrefix(trimmed, "export default function ") ||
				strings.HasPrefix(trimmed, "export class ") ||
				strings.HasPrefix(trimmed, "class ") ||
				strings.HasPrefix(trimmed, "export interface ") ||
				strings.HasPrefix(trimmed, "interface ") ||
				strings.Contains(trimmed, "=> {") {
				boundaries = append(boundaries, i)
			}
		case ".py":
			if strings.HasPrefix(trimmed, "def ") || strings.HasPrefix(trimmed, "class ") ||
				strings.HasPrefix(trimmed, "async def ") {
				boundaries = append(boundaries, i)
			}
		case ".rs":
			if strings.HasPrefix(trimmed, "fn ") || strings.HasPrefix(trimmed, "pub fn ") ||
				strings.HasPrefix(trimmed, "impl ") || strings.HasPrefix(trimmed, "struct ") {
				boundaries = append(boundaries, i)
			}
		case ".java", ".kt":
			if strings.HasPrefix(trimmed, "public ") || strings.HasPrefix(trimmed, "private ") ||
				strings.HasPrefix(trimmed, "class ") || strings.HasPrefix(trimmed, "fun ") {
				boundaries = append(boundaries, i)
			}
		default:
			// Generic: look for common patterns
			if strings.HasPrefix(trimmed, "func ") || strings.HasPrefix(trimmed, "function ") ||
				strings.HasPrefix(trimmed, "def ") || strings.HasPrefix(trimmed, "class ") {
				boundaries = append(boundaries, i)
			}
		}
	}

	// Always include file start if first boundary isn't at line 0
	if len(boundaries) > 0 && boundaries[0] > 5 {
		boundaries = append([]int{0}, boundaries...)
	}

	return boundaries
}

// chunkBySize splits into fixed-size line chunks with overlap.
func chunkBySize(filePath string, lines []string, chunkSize int) []codeChunk {
	var chunks []codeChunk
	overlap := 5

	for start := 0; start < len(lines); start += chunkSize - overlap {
		end := start + chunkSize
		if end > len(lines) {
			end = len(lines)
		}

		chunkContent := strings.Join(lines[start:end], "\n")
		if strings.TrimSpace(chunkContent) == "" {
			continue
		}

		chunks = append(chunks, codeChunk{
			id:      fmt.Sprintf("%s:%d-%d", filePath, start+1, end),
			content: fmt.Sprintf("// File: %s (lines %d-%d)\n%s", filePath, start+1, end, chunkContent),
		})

		if end >= len(lines) {
			break
		}
	}
	return chunks
}

// isSupportedCodeFile checks if a file should be indexed.
func isSupportedCodeFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	supported := map[string]bool{
		".go": true, ".py": true, ".ts": true, ".tsx": true,
		".js": true, ".jsx": true, ".rs": true, ".java": true,
		".kt": true, ".rb": true, ".c": true, ".cpp": true,
		".h": true, ".hpp": true, ".cs": true, ".swift": true,
		".vue": true, ".svelte": true,
	}
	return supported[ext]
}

// FormatChunksForPrompt formats search results for prompt injection.
func FormatChunksForPrompt(chunks []CodeChunk, maxTokens int) string {
	if len(chunks) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("Relevant code from the codebase:\n\n")

	approxTokens := 0
	for _, chunk := range chunks {
		chunkTokens := len(chunk.Content) / 4 // rough estimate
		if approxTokens+chunkTokens > maxTokens && approxTokens > 0 {
			break
		}

		sb.WriteString(fmt.Sprintf("```\n%s\n```\n\n", chunk.Content))
		approxTokens += chunkTokens
	}

	return sb.String()
}

// scanLines is a helper that counts lines in content.
func scanLines(content string) int {
	scanner := bufio.NewScanner(strings.NewReader(content))
	count := 0
	for scanner.Scan() {
		count++
	}
	return count
}
