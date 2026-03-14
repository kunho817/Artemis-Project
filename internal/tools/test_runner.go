package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"
)

// RunTestsTool executes `go test -json` and returns structured results.
type RunTestsTool struct {
	baseDir string
}

func (t *RunTestsTool) Name() string { return "run_tests" }

func (t *RunTestsTool) Description() string {
	return "Run tests and return structured results with pass/fail counts and failure details"
}

func (t *RunTestsTool) Parameters() string {
	return "path (string, optional, default ./...) — Go package path to test; filter (string, optional) — test name regex for -run; verbose (bool, optional, default false) — include all output; timeout (string, optional, default 120s) — go test timeout duration"
}

type test2jsonEvent struct {
	Action  string  `json:"Action"`
	Package string  `json:"Package"`
	Test    string  `json:"Test"`
	Output  string  `json:"Output"`
	Elapsed float64 `json:"Elapsed"`
}

type failedTest struct {
	Package string
	Name    string
	Elapsed float64
	Output  []string
}

func (t *RunTestsTool) Execute(ctx context.Context, params map[string]interface{}) (ToolResult, error) {
	path := "./..."
	if v, ok := params["path"].(string); ok && strings.TrimSpace(v) != "" {
		path = strings.TrimSpace(v)
	}

	filter := ""
	if v, ok := params["filter"].(string); ok {
		filter = strings.TrimSpace(v)
	}

	verbose := false
	if v, ok := params["verbose"].(bool); ok {
		verbose = v
	}

	timeoutStr := "120s"
	if v, ok := params["timeout"].(string); ok && strings.TrimSpace(v) != "" {
		timeoutStr = strings.TrimSpace(v)
	}

	timeoutDur, err := time.ParseDuration(timeoutStr)
	if err != nil || timeoutDur <= 0 {
		return ToolResult{Error: fmt.Sprintf("invalid timeout %q: expected Go duration like 30s, 2m", timeoutStr)}, nil
	}

	args := []string{"test", "-json", "-count=1"}
	if filter != "" {
		args = append(args, "-run", filter)
	}
	args = append(args, "-timeout", timeoutStr)
	if verbose {
		args = append(args, "-v")
	}
	args = append(args, path)

	execCtx, cancel := context.WithTimeout(ctx, timeoutDur)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "go", args...)
	cmd.Dir = t.baseDir
	if runtime.GOOS == "windows" {
		setHiddenProcessAttrs(cmd)
	}

	rawOut, runErr := cmd.CombinedOutput()

	if runErr != nil {
		if errors.Is(runErr, exec.ErrNotFound) {
			return ToolResult{Error: "go toolchain not found in PATH", Content: "Unable to run tests: `go` command is not available."}, nil
		}
		var execErr *exec.Error
		if errors.As(runErr, &execErr) && errors.Is(execErr.Err, exec.ErrNotFound) {
			return ToolResult{Error: "go toolchain not found in PATH", Content: "Unable to run tests: `go` command is not available."}, nil
		}
	}

	out, truncated := capOutput(rawOut, 100*1024)
	if truncated {
		out = append(out, []byte("\n... (output truncated to 100KB)")...)
	}

	summary, parsed := parseGoTestJSONOutput(out)
	if !parsed {
		fallback := strings.TrimSpace(string(out))
		if fallback == "" {
			fallback = "(no output)"
		}
		res := ToolResult{Content: fallback}
		if runErr != nil {
			res.Error = runErr.Error()
		}
		return res, nil
	}

	res := ToolResult{Content: summary}
	if runErr != nil {
		res.Error = runErr.Error()
	}
	return res, nil
}

func capOutput(in []byte, max int) ([]byte, bool) {
	if len(in) <= max {
		return in, false
	}
	return in[:max], true
}

func parseGoTestJSONOutput(out []byte) (string, bool) {
	testOutputs := make(map[string][]string)
	packageOutputs := make(map[string][]string)

	passedTests := make([]string, 0)
	skippedTests := make([]string, 0)
	failedTests := make(map[string]*failedTest)
	failedPackages := make(map[string]bool)

	parsedLines := 0
	totalElapsed := 0.0

	s := bufio.NewScanner(bytes.NewReader(out))
	const maxLine = 1024 * 1024
	s.Buffer(make([]byte, 64*1024), maxLine)

	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			continue
		}

		var ev test2jsonEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		parsedLines++

		if ev.Action == "output" {
			if ev.Test != "" {
				key := ev.Package + "::" + ev.Test
				testOutputs[key] = append(testOutputs[key], strings.TrimRight(ev.Output, "\r\n"))
			} else {
				pkg := ev.Package
				if pkg == "" {
					pkg = "(unknown package)"
				}
				packageOutputs[pkg] = append(packageOutputs[pkg], strings.TrimRight(ev.Output, "\r\n"))
			}
			continue
		}

		if ev.Action == "pass" || ev.Action == "fail" {
			if ev.Test == "" && ev.Elapsed > 0 {
				totalElapsed += ev.Elapsed
			}
		}

		switch ev.Action {
		case "pass":
			if ev.Test != "" {
				passedTests = append(passedTests, ev.Test)
			}
		case "skip":
			if ev.Test != "" {
				skippedTests = append(skippedTests, ev.Test)
			}
		case "fail":
			if ev.Test != "" {
				key := ev.Package + "::" + ev.Test
				failedTests[key] = &failedTest{
					Package: ev.Package,
					Name:    ev.Test,
					Elapsed: ev.Elapsed,
					Output:  append([]string{}, testOutputs[key]...),
				}
			} else {
				pkg := ev.Package
				if pkg == "" {
					pkg = "(unknown package)"
				}
				failedPackages[pkg] = true
			}
		}
	}

	if parsedLines == 0 {
		return "", false
	}

	// Remove test names that also appear in failures.
	if len(failedTests) > 0 {
		failedNames := make(map[string]bool, len(failedTests))
		for _, ft := range failedTests {
			failedNames[ft.Name] = true
		}
		filtered := make([]string, 0, len(passedTests))
		for _, name := range passedTests {
			if !failedNames[name] {
				filtered = append(filtered, name)
			}
		}
		passedTests = filtered
	}

	passCount := len(passedTests)
	failCount := len(failedTests)
	skipCount := len(skippedTests)

	if failCount == 0 && len(failedPackages) == 0 {
		return fmt.Sprintf("All tests passed: %d tests in %.2fs", passCount, totalElapsed), true
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Test Results: %d passed, %d failed, %d skipped (%.2fs)\n\n", passCount, failCount, skipCount, totalElapsed))

	if failCount > 0 || len(failedPackages) > 0 {
		sb.WriteString("FAILED:\n")

		failedList := make([]*failedTest, 0, len(failedTests))
		for _, ft := range failedTests {
			failedList = append(failedList, ft)
		}
		sort.Slice(failedList, func(i, j int) bool {
			if failedList[i].Package == failedList[j].Package {
				return failedList[i].Name < failedList[j].Name
			}
			return failedList[i].Package < failedList[j].Package
		})

		for _, ft := range failedList {
			pkg := ft.Package
			if pkg == "" {
				pkg = "(unknown package)"
			}
			sb.WriteString(fmt.Sprintf("  %s (%s) — %.2fs\n", ft.Name, pkg, ft.Elapsed))
			for _, line := range ft.Output {
				trimmed := strings.TrimSpace(line)
				if trimmed == "" {
					continue
				}
				sb.WriteString("    " + trimmed + "\n")
			}
			sb.WriteString("\n")
		}

		if len(failedPackages) > 0 {
			pkgs := make([]string, 0, len(failedPackages))
			for pkg := range failedPackages {
				pkgs = append(pkgs, pkg)
			}
			sort.Strings(pkgs)
			for _, pkg := range pkgs {
				sb.WriteString(fmt.Sprintf("  package %s\n", pkg))
				for _, line := range packageOutputs[pkg] {
					trimmed := strings.TrimSpace(line)
					if trimmed == "" {
						continue
					}
					sb.WriteString("    " + trimmed + "\n")
				}
				sb.WriteString("\n")
			}
		}
	}

	sort.Strings(passedTests)
	sort.Strings(skippedTests)

	if len(passedTests) > 0 {
		sb.WriteString(fmt.Sprintf("PASSED: %s (%d tests)\n", strings.Join(passedTests, ", "), len(passedTests)))
	}
	if len(skippedTests) > 0 {
		sb.WriteString(fmt.Sprintf("SKIPPED: %s (%d tests)\n", strings.Join(skippedTests, ", "), len(skippedTests)))
	}

	return strings.TrimSpace(sb.String()), true
}
