package memory

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type RepoMapStore struct {
	db       *sql.DB
	parser   SymbolParser
	rootPath string
	exclude  []string
}

func NewRepoMapStore(db *sql.DB, parser SymbolParser, rootPath string, exclude []string) *RepoMapStore {
	return &RepoMapStore{
		db:       db,
		parser:   parser,
		rootPath: rootPath,
		exclude:  exclude,
	}
}

func (r *RepoMapStore) IndexFile(ctx context.Context, filePath string) error {
	_, err := r.indexFileWithStatus(ctx, filePath)
	return err
}

func (r *RepoMapStore) indexFileWithStatus(ctx context.Context, filePath string) (bool, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return false, fmt.Errorf("read file for indexing: %w", err)
	}

	hash := sha256.Sum256(content)
	fileHash := fmt.Sprintf("%x", hash[:])

	relPath, err := filepath.Rel(r.rootPath, filePath)
	if err != nil {
		relPath = filePath
	}
	relPath = filepath.ToSlash(relPath)

	var existingHash string
	row := r.db.QueryRowContext(ctx, `SELECT file_hash FROM repo_symbols WHERE file_path = ? LIMIT 1`, relPath)
	if err := row.Scan(&existingHash); err == nil {
		if existingHash == fileHash {
			return false, nil
		}
	} else if err != sql.ErrNoRows {
		return false, fmt.Errorf("check existing file hash: %w", err)
	}

	symbols, err := r.parser.Parse(ctx, filePath)
	if err != nil {
		return false, fmt.Errorf("parse symbols: %w", err)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin repo index tx: %w", err)
	}

	rollback := true
	defer func() {
		if rollback {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.ExecContext(ctx, `DELETE FROM repo_symbols WHERE file_path = ?`, relPath); err != nil {
		return false, fmt.Errorf("delete stale symbols: %w", err)
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO repo_symbols (file_path, name, kind, line, signature, scope, exported, file_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return false, fmt.Errorf("prepare symbol insert: %w", err)
	}
	defer stmt.Close()

	for _, sym := range symbols {
		if _, err := stmt.ExecContext(
			ctx,
			relPath,
			sym.Name,
			fmt.Sprint(sym.Kind),
			sym.Line,
			sym.Signature,
			sym.Scope,
			sym.Exported,
			fileHash,
		); err != nil {
			return false, fmt.Errorf("insert symbol %q: %w", sym.Name, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit repo index tx: %w", err)
	}
	rollback = false

	return true, nil
}

func (r *RepoMapStore) IndexDirectory(ctx context.Context, dirPath string) (indexed int, skipped int, err error) {
	supported := make(map[string]struct{}, len(r.parser.SupportedExts()))
	for _, ext := range r.parser.SupportedExts() {
		supported[strings.ToLower(ext)] = struct{}{}
	}

	walkErr := filepath.WalkDir(dirPath, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, relErr := filepath.Rel(r.rootPath, path)
		if relErr != nil {
			rel = path
		}
		rel = filepath.ToSlash(rel)

		if r.shouldExclude(rel, d.IsDir()) {
			skipped++
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if _, ok := supported[ext]; !ok {
			skipped++
			return nil
		}

		changed, idxErr := r.indexFileWithStatus(ctx, path)
		if idxErr != nil {
			return idxErr
		}
		if changed {
			indexed++
		} else {
			skipped++
		}

		return nil
	})

	if walkErr != nil {
		return indexed, skipped, fmt.Errorf("walk directory for repo-map index: %w", walkErr)
	}

	return indexed, skipped, nil
}

func (r *RepoMapStore) shouldExclude(relPath string, isDir bool) bool {
	relPath = strings.TrimPrefix(filepath.ToSlash(relPath), "./")
	for _, pattern := range r.exclude {
		p := filepath.ToSlash(strings.TrimSpace(pattern))
		if p == "" {
			continue
		}

		if strings.HasSuffix(p, "/") {
			segment := strings.TrimSuffix(p, "/")
			if segment != "" && strings.Contains(relPath, segment) {
				return true
			}
			continue
		}

		if ok, _ := filepath.Match(p, relPath); ok {
			return true
		}

		if isDir {
			if ok, _ := filepath.Match(p, relPath+"/"); ok {
				return true
			}
		}
	}
	return false
}

func (r *RepoMapStore) QuerySymbols(ctx context.Context, query string, limit int) ([]Symbol, error) {
	if limit <= 0 {
		limit = 100
	}

	baseSelect := `
		SELECT file_path, name, kind, line, signature, scope, exported
		FROM repo_symbols
	`

	var (
		rows *sql.Rows
		err  error
	)

	if strings.TrimSpace(query) == "" {
		rows, err = r.db.QueryContext(ctx,
			baseSelect+` ORDER BY file_path ASC, line ASC LIMIT ?`,
			limit,
		)
	} else {
		rows, err = r.db.QueryContext(ctx, `
			SELECT rs.file_path, rs.name, rs.kind, rs.line, rs.signature, rs.scope, rs.exported
			FROM repo_symbols rs
			JOIN repo_symbols_fts fts ON rs.id = fts.rowid
			WHERE repo_symbols_fts MATCH ?
			ORDER BY rs.file_path ASC, rs.line ASC
			LIMIT ?
		`, buildFTSQuery(query), limit)
	}
	if err != nil {
		return nil, fmt.Errorf("query repo symbols: %w", err)
	}
	defer rows.Close()

	return scanSymbols(rows)
}

func (r *RepoMapStore) GetFileSymbols(ctx context.Context, filePath string) ([]Symbol, error) {
	relPath, err := filepath.Rel(r.rootPath, filePath)
	if err != nil {
		relPath = filePath
	}
	relPath = filepath.ToSlash(relPath)

	rows, err := r.db.QueryContext(ctx, `
		SELECT file_path, name, kind, line, signature, scope, exported
		FROM repo_symbols
		WHERE file_path = ?
		ORDER BY line ASC
	`, relPath)
	if err != nil {
		return nil, fmt.Errorf("query symbols by file: %w", err)
	}
	defer rows.Close()

	return scanSymbols(rows)
}

func (r *RepoMapStore) FormatRepoMap(symbols []Symbol, maxTokens int) string {
	if len(symbols) == 0 || maxTokens <= 0 {
		return ""
	}

	grouped := make(map[string][]Symbol)
	for _, sym := range symbols {
		grouped[sym.FilePath] = append(grouped[sym.FilePath], sym)
	}

	files := make([]string, 0, len(grouped))
	for file := range grouped {
		files = append(files, file)
	}
	sort.Strings(files)

	var b strings.Builder
	for _, file := range files {
		chunk := formatFileSymbols(file, grouped[file])
		candidate := b.String() + chunk
		if len(candidate)/4 > maxTokens {
			break
		}
		b.WriteString(chunk)
	}

	return strings.TrimSpace(b.String())
}

func formatFileSymbols(file string, symbols []Symbol) string {
	sort.Slice(symbols, func(i, j int) bool {
		if symbols[i].Line == symbols[j].Line {
			return symbols[i].Name < symbols[j].Name
		}
		return symbols[i].Line < symbols[j].Line
	})

	var b strings.Builder
	b.WriteString(file)
	b.WriteString(":\n")

	for _, sym := range symbols {
		b.WriteString("│  ")

		kind := symbolKindLabel(sym.Kind)
		if sym.Scope != "" && sym.Kind == KindMethod {
			b.WriteString("func (")
			b.WriteString(sym.Scope)
			b.WriteString(") ")
			b.WriteString(sym.Name)
		} else {
			b.WriteString(kind)
			b.WriteString(" ")
			b.WriteString(sym.Name)
		}

		if strings.TrimSpace(sym.Signature) != "" {
			b.WriteString(sym.Signature)
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	return b.String()
}

func (r *RepoMapStore) Stats(ctx context.Context) (fileCount int, symbolCount int, err error) {
	err = r.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT file_path), COUNT(*) FROM repo_symbols`).
		Scan(&fileCount, &symbolCount)
	if err != nil {
		return 0, 0, fmt.Errorf("query repo-map stats: %w", err)
	}
	return fileCount, symbolCount, nil
}

func (r *RepoMapStore) Close() error {
	return nil
}

func scanSymbols(rows *sql.Rows) ([]Symbol, error) {
	out := make([]Symbol, 0)
	for rows.Next() {
		var s Symbol
		if err := rows.Scan(&s.FilePath, &s.Name, &s.Kind, &s.Line, &s.Signature, &s.Scope, &s.Exported); err != nil {
			return nil, fmt.Errorf("scan symbol row: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate symbol rows: %w", err)
	}
	return out, nil
}

func symbolKindLabel(kind SymbolKind) string {
	switch kind {
	case KindFunction:
		return "func"
	case KindMethod:
		return "method"
	case KindType:
		return "type"
	case KindStruct:
		return "struct"
	case KindInterface:
		return "interface"
	case KindClass:
		return "class"
	case KindModule:
		return "module"
	case KindConst:
		return "const"
	case KindVar:
		return "var"
	case KindField:
		return "field"
	case KindProperty:
		return "property"
	case KindPackage:
		return "package"
	default:
		return "symbol"
	}
}
