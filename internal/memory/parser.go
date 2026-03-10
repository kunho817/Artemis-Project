package memory

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"unicode"
)

// SymbolParser extracts code symbols from source files.
type SymbolParser interface {
	Parse(ctx context.Context, filePath string) ([]Symbol, error)
	SupportedExts() []string
}

type CtagsParser struct {
	ctagsPath string // resolved ctags binary path
}

func NewCtagsParser(ctagsPath string) *CtagsParser {
	return &CtagsParser{ctagsPath: ctagsPath}
}

func (p *CtagsParser) Parse(ctx context.Context, filePath string) ([]Symbol, error) {
	cmd := exec.CommandContext(
		ctx,
		p.ctagsPath,
		"--output-format=json",
		"--fields=+neKS",
		"--kinds-all=*",
		"-f",
		"-",
		filePath,
	)

	output, err := cmd.Output()
	if err != nil {
		// Graceful handling for per-file parsing errors.
		return []Symbol{}, nil
	}

	lines := strings.Split(string(output), "\n")
	result := make([]Symbol, 0, len(lines))

	type ctagsLine struct {
		Type      string `json:"_type"`
		Name      string `json:"name"`
		Path      string `json:"path"`
		Language  string `json:"language"`
		Line      int    `json:"line"`
		Kind      string `json:"kind"`
		Scope     string `json:"scope"`
		Signature string `json:"signature"`
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var parsed ctagsLine
		if err := json.Unmarshal([]byte(line), &parsed); err != nil {
			continue
		}

		if parsed.Type != "tag" || parsed.Name == "" {
			continue
		}

		result = append(result, Symbol{
			Name:      parsed.Name,
			Kind:      mapCTagsKind(parsed.Kind),
			FilePath:  parsed.Path,
			Line:      parsed.Line,
			Signature: parsed.Signature,
			Scope:     parsed.Scope,
			Exported:  isExportedSymbol(parsed.Name, parsed.Language),
		})
	}

	return result, nil
}

func (p *CtagsParser) SupportedExts() []string {
	return []string{
		".go", ".py", ".js", ".ts", ".jsx", ".tsx", ".java", ".rs", ".rb",
		".c", ".cpp", ".h", ".hpp", ".cs", ".swift", ".kt", ".scala", ".php",
	}
}

func mapCTagsKind(kind string) SymbolKind {
	switch strings.ToLower(kind) {
	case "function":
		return KindFunction
	case "method":
		return KindMethod
	case "type":
		return KindType
	case "struct":
		return KindStruct
	case "interface":
		return KindInterface
	case "class":
		return KindClass
	case "variable":
		return KindVar
	case "constant":
		return KindConst
	case "member":
		return KindField
	case "property":
		return KindProperty
	case "package":
		return KindPackage
	case "module":
		return KindModule
	default:
		return KindUnknown
	}
}

func isExportedSymbol(name, language string) bool {
	if name == "" {
		return false
	}

	switch strings.ToLower(language) {
	case "go":
		r := []rune(name)
		if len(r) == 0 {
			return false
		}
		return unicode.IsUpper(r[0])
	case "python":
		return !strings.HasPrefix(name, "_")
	default:
		return true
	}
}
