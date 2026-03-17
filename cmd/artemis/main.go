package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/artemis-project/artemis/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	args := os.Args[1:]

	// Route by subcommand or flag
	if len(args) > 0 {
		switch args[0] {
		case "chat":
			// One-shot: artemis chat "message"
			// Multi-agent: artemis chat --multi "message"
			runChat(args[1:])
			return
		case "--headless":
			// Interactive headless mode (stdin/stdout, no TUI)
			runHeadless(args[1:])
			return
		case "--help", "-h":
			printUsage()
			return
		case "--version", "-v":
			fmt.Println("Artemis v0.1.0")
			return
		}
		// If first arg doesn't look like a flag, treat as chat
		if !strings.HasPrefix(args[0], "-") {
			runChat(args)
			return
		}
	}

	// Default: TUI mode
	app := tui.NewApp()
	p := tea.NewProgram(
		app,
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`Artemis — TUI-based multi-agent coding system

Usage:
  artemis                          Start TUI (interactive)
  artemis --headless               Start headless mode (stdin/stdout)
  artemis chat "message"           One-shot chat (single response)
  artemis chat --multi "message"   One-shot with multi-agent pipeline

Options:
  --headless     Run without TUI (stdin/stdout conversation)
  --help, -h     Show this help
  --version, -v  Show version

Chat options:
  --multi        Use multi-agent orchestrator
  --agent NAME   Specify agent role (default: coder)
  --dir PATH     Working directory for tools (default: current)

Examples:
  artemis chat "Read the go.mod file"
  artemis chat --multi "Analyze this project and suggest improvements"
  artemis chat --agent analyzer "What does the main function do?"
  echo "Fix the bug" | artemis --headless`)
}
