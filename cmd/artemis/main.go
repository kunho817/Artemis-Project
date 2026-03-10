package main

import (
	"fmt"
	"os"

	"github.com/artemis-project/artemis/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
)

func main() {
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
