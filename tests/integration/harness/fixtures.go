package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FixtureManager creates and manages test project fixtures.
type FixtureManager struct {
	baseDir string
}

// NewFixtureManager creates a new fixture manager.
func NewFixtureManager(baseDir string) *FixtureManager {
	return &FixtureManager{
		baseDir: baseDir,
	}
}

// CreateGoFixture creates a simple Go project fixture.
func (fm *FixtureManager) CreateGoFixture(name string) (string, error) {
	projectDir := filepath.Join(fm.baseDir, name)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return "", err
	}

	// Create main.go
	mainGo := fmt.Sprintf(`package main

import "fmt"

func main() {
	fmt.Println("Hello, World!")
}
`)
	if err := os.WriteFile(filepath.Join(projectDir, "main.go"), []byte(mainGo), 0644); err != nil {
		return "", err
	}

	// Create go.mod
	goMod := fmt.Sprintf(`module %s

go 1.21
`, name)
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte(goMod), 0644); err != nil {
		return "", err
	}

	return projectDir, nil
}

// CreatePythonFixture creates a simple Python project fixture.
func (fm *FixtureManager) CreatePythonFixture(name string) (string, error) {
	projectDir := filepath.Join(fm.baseDir, name)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return "", err
	}

	// Create main.py
	mainPy := `#!/usr/bin/env python3
"""Simple Python fixture."""

def greet(name: str) -> str:
    """Greet the user."""
    return f"Hello, {name}!"

if __name__ == "__main__":
    print(greet("World"))
`
	if err := os.WriteFile(filepath.Join(projectDir, "main.py"), []byte(mainPy), 0644); err != nil {
		return "", err
	}

	return projectDir, nil
}

// CreateJavaScriptFixture creates a simple JavaScript project fixture.
func (fm *FixtureManager) CreateJavaScriptFixture(name string) (string, error) {
	projectDir := filepath.Join(fm.baseDir, name)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return "", err
	}

	// Create main.js
	mainJs := `// Simple JavaScript fixture

function greet(name) {
    return "Hello, " + name + "!";
}

console.log(greet("World"));
`
	if err := os.WriteFile(filepath.Join(projectDir, "main.js"), []byte(mainJs), 0644); err != nil {
		return "", err
	}

	return projectDir, nil
}

// CreateRustFixture creates a simple Rust project fixture.
func (fm *FixtureManager) CreateRustFixture(name string) (string, error) {
	projectDir := filepath.Join(fm.baseDir, name)
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		return "", err
	}

	// Create main.rs
	mainRs := `fn main() {
    println!("Hello, World!");
}
`
	srcDir := filepath.Join(projectDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(srcDir, "main.rs"), []byte(mainRs), 0644); err != nil {
		return "", err
	}

	// Create Cargo.toml
	cargoToml := fmt.Sprintf(`[package]
name = "%s"
version = "0.1.0"
edition = "2021"

[dependencies]
`, strings.ReplaceAll(name, "-", "_"))
	if err := os.WriteFile(filepath.Join(projectDir, "Cargo.toml"), []byte(cargoToml), 0644); err != nil {
		return "", err
	}

	return projectDir, nil
}

// CreateComplexFixture creates a multi-file project fixture.
func (fm *FixtureManager) CreateComplexFixture(name string) (string, error) {
	projectDir, err := fm.CreateGoFixture(name)
	if err != nil {
		return "", err
	}

	// Create additional files
	utilsGo := `package main

import "strings"

func ToUpper(s string) string {
    return strings.ToUpper(s)
}

func ToLower(s string) string {
    return strings.ToLower(s)
}
`
	if err := os.WriteFile(filepath.Join(projectDir, "utils.go"), []byte(utilsGo), 0644); err != nil {
		return "", err
	}

	// Create test file
	utilsTest := `package main

import "testing"

func TestToUpper(t *testing.T) {
    result := ToUpper("hello")
    if result != "HELLO" {
        t.Errorf("Expected HELLO, got %s", result)
    }
}

func TestToLower(t *testing.T) {
    result := ToLower("HELLO")
    if result != "hello" {
        t.Errorf("Expected hello, got %s", result)
    }
}
`
	if err := os.WriteFile(filepath.Join(projectDir, "utils_test.go"), []byte(utilsTest), 0644); err != nil {
		return "", err
	}

	// Create README
	readme := fmt.Sprintf(`# %s

This is a test fixture for integration testing.

## Features
- Greeting functionality
- String utilities
- Test coverage
`, name)
	if err := os.WriteFile(filepath.Join(projectDir, "README.md"), []byte(readme), 0644); err != nil {
		return "", err
	}

	return projectDir, nil
}

// CreateBuggyFixture creates a project with intentional bugs for testing.
func (fm *FixtureManager) CreateBuggyFixture(name string) (string, error) {
	projectDir, err := fm.CreateGoFixture(name)
	if err != nil {
		return "", err
	}

	// Overwrite main.go with buggy code
	buggyGo := `package main

import "fmt"

func divide(a, b int) int {
    // Intentional bug: no division by zero check
    return a / b
}

func main() {
    result := divide(10, 0) // This will panic
    fmt.Println(result)
}
`
	if err := os.WriteFile(filepath.Join(projectDir, "main.go"), []byte(buggyGo), 0644); err != nil {
		return "", err
	}

	return projectDir, nil
}

// CreateLargeFixture creates a larger project with multiple files and directories.
func (fm *FixtureManager) CreateLargeFixture(name string) (string, error) {
	projectDir, err := fm.CreateComplexFixture(name)
	if err != nil {
		return "", err
	}

	// Create additional directories
	dirs := []string{"pkg", "cmd", "internal", "docs"}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(projectDir, dir), 0755); err != nil {
			return "", err
		}
	}

	// Create package file
	pkgFile := `package pkg

import "fmt"

type Printer struct {
    prefix string
}

func NewPrinter(prefix string) *Printer {
    return &Printer{prefix: prefix}
}

func (p *Printer) Print(msg string) {
    fmt.Printf("[%s] %s\n", p.prefix, msg)
}
`
	if err := os.WriteFile(filepath.Join(projectDir, "pkg", "printer.go"), []byte(pkgFile), 0644); err != nil {
		return "", err
	}

	// Create command file
	cmdFile := `package main

import (
    "flag"
    "fmt"
)

var verbose bool

func init() {
    flag.BoolVar(&verbose, "verbose", false, "Enable verbose output")
}

func main() {
    flag.Parse()
    if verbose {
        fmt.Println("Verbose mode enabled")
    }
}
`
	if err := os.WriteFile(filepath.Join(projectDir, "cmd", "root.go"), []byte(cmdFile), 0644); err != nil {
		return "", err
	}

	return projectDir, nil
}

// CleanUp removes all created fixtures.
func (fm *FixtureManager) CleanUp() error {
	return os.RemoveAll(fm.baseDir)
}

// GetFixturePath returns the path to a fixture directory.
func (fm *FixtureManager) GetFixturePath(name string) string {
	return filepath.Join(fm.baseDir, name)
}

// CreateAllFixtures creates all fixture types for comprehensive testing.
func (fm *FixtureManager) CreateAllFixtures() (map[string]string, error) {
	fixtures := make(map[string]string)

	// Create various fixture types
	types := []struct {
		name string
		fn   func(string) (string, error)
	}{
		{"go-project", fm.CreateGoFixture},
		{"python-project", fm.CreatePythonFixture},
		{"js-project", fm.CreateJavaScriptFixture},
		{"rust-project", fm.CreateRustFixture},
		{"complex-project", fm.CreateComplexFixture},
		{"buggy-project", fm.CreateBuggyFixture},
		{"large-project", fm.CreateLargeFixture},
	}

	for _, fixtureType := range types {
		path, err := fixtureType.fn(fixtureType.name)
		if err != nil {
			return nil, fmt.Errorf("failed to create fixture %s: %w", fixtureType.name, err)
		}
		fixtures[fixtureType.name] = path
	}

	return fixtures, nil
}
