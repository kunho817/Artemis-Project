package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"path"
	"runtime"
	"sort"
	"strings"
	"time"
)

// FindDependenciesTool finds direct/transitive dependencies for a Go package.
type FindDependenciesTool struct {
	baseDir string
}

func (t *FindDependenciesTool) Name() string { return "find_dependencies" }
func (t *FindDependenciesTool) Description() string {
	return "Find all packages that a given Go package imports (direct dependencies)"
}
func (t *FindDependenciesTool) Parameters() string {
	return "package (string, required) — Go package path (e.g. ./internal/tools or github.com/artemis-project/artemis/internal/tools)"
}

func (t *FindDependenciesTool) Execute(ctx context.Context, params map[string]interface{}) (ToolResult, error) {
	pkg, ok := params["package"].(string)
	pkg = normalizePackageInput(pkg)
	if !ok || pkg == "" {
		return ToolResult{Error: "missing required parameter: package"}, nil
	}

	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	pkgInfo, err := goListSingle(execCtx, t.baseDir, pkg)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("go list failed for %q: %v", pkg, err)}, nil
	}

	modulePath, _ := getModulePath(execCtx, t.baseDir)

	imports := append([]string(nil), pkgInfo.Imports...)
	sort.Strings(imports)

	stdlib, internal, external := classifyImports(imports, modulePath)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Package: %s\n", orFallback(pkgInfo.ImportPath, pkg)))
	sb.WriteString(fmt.Sprintf("Name: %s\n", orFallback(pkgInfo.Name, "(unknown)")))
	if pkgInfo.Dir != "" {
		sb.WriteString(fmt.Sprintf("Dir: %s\n", pkgInfo.Dir))
	}
	if len(pkgInfo.GoFiles) > 0 {
		files := append([]string(nil), pkgInfo.GoFiles...)
		sort.Strings(files)
		sb.WriteString(fmt.Sprintf("Files: %s (%d files)\n", strings.Join(files, ", "), len(files)))
	} else {
		sb.WriteString("Files: (none)\n")
	}

	sb.WriteString("\n")
	sb.WriteString(fmt.Sprintf("Direct Imports (%d):\n", len(imports)))
	if len(imports) == 0 {
		sb.WriteString("  (none)\n")
	} else {
		for _, imp := range imports {
			sb.WriteString("  " + imp + "\n")
		}
	}

	sb.WriteString("\n")
	writeImportSection(&sb, "Stdlib Imports", stdlib)
	writeImportSection(&sb, "Internal Imports (project-only)", internal)
	writeImportSection(&sb, "External Imports", external)

	if len(pkgInfo.Deps) > 0 {
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("Transitive Dependencies (%d)\n", len(pkgInfo.Deps)))
	}

	return ToolResult{Content: strings.TrimSpace(sb.String())}, nil
}

// FindDependentsTool finds project packages that directly import a target package.
type FindDependentsTool struct {
	baseDir string
}

func (t *FindDependentsTool) Name() string { return "find_dependents" }
func (t *FindDependentsTool) Description() string {
	return "Find all packages in the project that depend on (import) a given package"
}
func (t *FindDependentsTool) Parameters() string {
	return "package (string, required) — Go package path to find dependents of"
}

func (t *FindDependentsTool) Execute(ctx context.Context, params map[string]interface{}) (ToolResult, error) {
	targetParam, ok := params["package"].(string)
	targetParam = normalizePackageInput(targetParam)
	if !ok || targetParam == "" {
		return ToolResult{Error: "missing required parameter: package"}, nil
	}

	execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	modulePath, _ := getModulePath(execCtx, t.baseDir)
	targetImportPath, resolved := resolveTargetImportPath(execCtx, t.baseDir, targetParam, modulePath)
	aliases := buildTargetAliases(targetParam, targetImportPath, modulePath)

	packages, err := goListAll(execCtx, t.baseDir)
	if err != nil {
		return ToolResult{Error: fmt.Sprintf("go list failed for project packages: %v", err)}, nil
	}

	var dependents []string
	for _, pkg := range packages {
		if pkg.ImportPath == "" || len(pkg.Imports) == 0 {
			continue
		}
		for _, imp := range pkg.Imports {
			if _, hit := aliases[imp]; hit {
				dependents = append(dependents, pkg.ImportPath)
				break
			}
		}
	}

	sort.Strings(dependents)
	displayTarget := targetImportPath
	if displayTarget == "" {
		displayTarget = targetParam
	}

	if len(dependents) == 0 {
		var sb strings.Builder
		if !resolved {
			sb.WriteString(fmt.Sprintf("Note: could not resolve %q via go list; matched imports using normalized aliases only.\n", targetParam))
		}
		sb.WriteString(fmt.Sprintf("No packages in this project import %s", displayTarget))
		return ToolResult{Content: sb.String()}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Dependents of %s (%d packages):\n", displayTarget, len(dependents)))
	for _, dep := range dependents {
		sb.WriteString("  " + dep + "\n")
		sb.WriteString("    → direct import\n")
	}

	if !resolved {
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("Note: target package %q was not resolved by go list; results are best-effort alias matches.", targetParam))
	}

	return ToolResult{Content: strings.TrimSpace(sb.String())}, nil
}

type goListPackage struct {
	ImportPath string   `json:"ImportPath"`
	Name       string   `json:"Name"`
	Dir        string   `json:"Dir"`
	GoFiles    []string `json:"GoFiles"`
	Imports    []string `json:"Imports"`
	Deps       []string `json:"Deps"`
}

func goListSingle(ctx context.Context, baseDir, pkg string) (goListPackage, error) {
	out, err := runGoCommand(ctx, baseDir, "list", "-json", pkg)
	if err != nil {
		return goListPackage{}, err
	}

	var info goListPackage
	if err := json.Unmarshal(out, &info); err != nil {
		return goListPackage{}, fmt.Errorf("failed to parse go list output: %w", err)
	}
	return info, nil
}

func goListAll(ctx context.Context, baseDir string) ([]goListPackage, error) {
	out, err := runGoCommand(ctx, baseDir, "list", "-json", "./...")
	if err != nil {
		return nil, err
	}

	dec := json.NewDecoder(bytes.NewReader(out))
	var pkgs []goListPackage
	for {
		var p goListPackage
		if err := dec.Decode(&p); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("failed to parse go list stream: %w", err)
		}
		pkgs = append(pkgs, p)
	}

	return pkgs, nil
}

func getModulePath(ctx context.Context, baseDir string) (string, error) {
	out, err := runGoCommand(ctx, baseDir, "list", "-m", "-f", "{{.Path}}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func runGoCommand(ctx context.Context, baseDir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = baseDir
	if runtime.GOOS == "windows" {
		setHiddenProcessAttrs(cmd)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := err.Error()
		if stderr.Len() > 0 {
			errMsg = strings.TrimSpace(stderr.String())
		}
		return nil, fmt.Errorf("%s", errMsg)
	}

	return stdout.Bytes(), nil
}

func classifyImports(imports []string, modulePath string) (stdlib []string, internal []string, external []string) {
	for _, imp := range imports {
		switch {
		case modulePath != "" && (imp == modulePath || strings.HasPrefix(imp, modulePath+"/")):
			internal = append(internal, imp)
		case isStdlibImport(imp):
			stdlib = append(stdlib, imp)
		default:
			external = append(external, imp)
		}
	}
	return
}

func isStdlibImport(imp string) bool {
	if imp == "" {
		return false
	}
	seg := imp
	if i := strings.Index(seg, "/"); i >= 0 {
		seg = seg[:i]
	}
	return !strings.Contains(seg, ".")
}

func resolveTargetImportPath(ctx context.Context, baseDir, input, modulePath string) (string, bool) {
	if input == "" {
		return "", false
	}

	if p, err := goListSingle(ctx, baseDir, input); err == nil && p.ImportPath != "" {
		return p.ImportPath, true
	}

	clean := strings.TrimPrefix(input, "./")
	clean = strings.TrimPrefix(clean, "/")
	if modulePath != "" && clean != "" {
		candidate := path.Join(modulePath, clean)
		if p, err := goListSingle(ctx, baseDir, candidate); err == nil && p.ImportPath != "" {
			return p.ImportPath, true
		}
		return candidate, false
	}

	return input, false
}

func buildTargetAliases(input, resolvedImportPath, modulePath string) map[string]struct{} {
	aliases := make(map[string]struct{})
	addAlias := func(v string) {
		v = strings.TrimSpace(normalizePackageInput(v))
		if v != "" {
			aliases[v] = struct{}{}
		}
	}

	addAlias(input)
	addAlias(resolvedImportPath)

	clean := strings.TrimPrefix(normalizePackageInput(input), "./")
	clean = strings.TrimPrefix(clean, "/")
	if modulePath != "" && clean != "" {
		addAlias(path.Join(modulePath, clean))
	}

	if resolvedImportPath != "" && modulePath != "" && strings.HasPrefix(resolvedImportPath, modulePath+"/") {
		short := strings.TrimPrefix(resolvedImportPath, modulePath+"/")
		addAlias(short)
		addAlias("./" + short)
	}

	return aliases
}

func normalizePackageInput(pkg string) string {
	pkg = strings.TrimSpace(pkg)
	pkg = strings.ReplaceAll(pkg, "\\", "/")
	return pkg
}

func writeImportSection(sb *strings.Builder, title string, imports []string) {
	sb.WriteString(fmt.Sprintf("%s (%d):\n", title, len(imports)))
	if len(imports) == 0 {
		sb.WriteString("  (none)\n")
		return
	}
	for _, imp := range imports {
		sb.WriteString("  " + imp + "\n")
	}
}

func orFallback(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
