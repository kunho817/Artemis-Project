// Package scenarios provides complex visual test scenarios.
package scenarios

import (
	"image"
	"image/color"

	"github.com/artemis-project/artemis/tests/integration/visual/prompts"
)

// ComplexScenarios returns complex test scenarios with multiple UI elements.
func ComplexScenarios() []Scenario {
	return []Scenario{
		{
			Name:        "Dashboard with Charts",
			Description: "A dashboard with multiple charts and metrics",
			Image:       createDashboardImage(),
			Question:    "Analyze the dashboard layout. Identify all charts, metrics, and their spatial relationships.",
			Expected: []ExpectedElement{
				{Type: "label", Label: "Revenue", GridPosition: "(0,0)", Description: "Revenue chart title"},
				{Type: "chart", Label: "", GridPosition: "(1-3,0-1)", Description: "Revenue bar chart"},
				{Type: "label", Label: "Users", GridPosition: "(0,2)", Description: "Users chart title"},
				{Type: "chart", Label: "", GridPosition: "(1-3,2-3)", Description: "Users line chart"},
				{Type: "metric", Label: "Total: $10K", GridPosition: "(4,0)", Description: "Total revenue metric"},
				{Type: "metric", Label: "Growth: +15%", GridPosition: "(4,2)", Description: "Growth metric"},
			},
		},
		{
			Name:        "Navigation Menu with Dropdown",
			Description: "A navigation bar with dropdown menus",
			Image:       createNavigationImage(),
			Question:    "Describe the navigation structure. Identify all menu items and their hierarchy.",
			Expected: []ExpectedElement{
				{Type: "menu_item", Label: "Home", GridPosition: "(0,0)", Description: "Home link"},
				{Type: "menu_item", Label: "Products", GridPosition: "(0,1)", Description: "Products menu with dropdown"},
				{Type: "dropdown", Label: "Electronics", GridPosition: "(1,1)", Description: "Electronics submenu"},
				{Type: "dropdown", Label: "Clothing", GridPosition: "(2,1)", Description: "Clothing submenu"},
				{Type: "menu_item", Label: "About", GridPosition: "(0,2)", Description: "About link"},
				{Type: "menu_item", Label: "Contact", GridPosition: "(0,3)", Description: "Contact link"},
			},
		},
		{
			Name:        "Data Table with Sort Headers",
			Description: "A sortable data table with multiple columns",
			Image:       createTableImage(),
			Question:    "Analyze the table structure. Identify headers, columns, and sorting indicators.",
			Expected: []ExpectedElement{
				{Type: "header", Label: "Name ▼", GridPosition: "(0,0)", Description: "Name column (descending)"},
				{Type: "header", Label: "Date", GridPosition: "(0,1)", Description: "Date column"},
				{Type: "header", Label: "Status", GridPosition: "(0,2)", Description: "Status column"},
				{Type: "header", Label: "Actions", GridPosition: "(0,3)", Description: "Actions column"},
				{Type: "row", Label: "Row 1", GridPosition: "(1,0-3)", Description: "First data row"},
				{Type: "row", Label: "Row 2", GridPosition: "(2,0-3)", Description: "Second data row"},
				{Type: "row", Label: "Row 3", GridPosition: "(3,0-3)", Description: "Third data row"},
			},
		},
		{
			Name:        "Multi-Tab Interface",
			Description: "An interface with multiple tabs and tab panels",
			Image:       createTabInterfaceImage(),
			Question:    "Describe the tab interface. Which tab is active and how are tabs arranged?",
			Expected: []ExpectedElement{
				{Type: "tab", Label: "Overview", GridPosition: "(0,0)", Description: "Overview tab (inactive)"},
				{Type: "tab", Label: "Details", GridPosition: "(0,1)", Description: "Details tab (active)"},
				{Type: "tab", Label: "Settings", GridPosition: "(0,2)", Description: "Settings tab (inactive)"},
				{Type: "panel", Label: "Details Panel", GridPosition: "(1-4,0-3)", Description: "Active tab content"},
			},
		},
	}
}

// GetSpatialComparisonScenarios returns scenarios for layout comparison testing.
func GetSpatialComparisonScenarios() []struct {
	Name         string
	ExpectedDesc string
	ActualDesc   string
	Prompt       string
} {
	builder := prompts.NewGridPromptBuilder(5, 5)

	return []struct {
		Name         string
		ExpectedDesc string
		ActualDesc   string
		Prompt       string
	}{
		{
			Name:         "Button Above Form",
			ExpectedDesc: "Submit button at (1,1), Login form at (3,1)",
			ActualDesc:   "Submit button at (1,1), Login form at (3,1)",
			Prompt:       builder.BuildComparisonPrompt("Submit button at (1,1), Login form at (3,1)", "Submit button at (1,1), Login form at (3,1)"),
		},
		{
			Name:         "Button Below Form (Mismatch)",
			ExpectedDesc: "Submit button at (1,1), Login form at (3,1)",
			ActualDesc:   "Login form at (1,1), Submit button at (3,1)",
			Prompt:       builder.BuildComparisonPrompt("Submit button at (1,1), Login form at (3,1)", "Login form at (1,1), Submit button at (3,1)"),
		},
		{
			Name:         "Side-by-Side Layout",
			ExpectedDesc: "Left panel at (1-4,0-1), Right panel at (1-4,3-4)",
			ActualDesc:   "Left panel at (1-4,0-1), Right panel at (1-4,3-4)",
			Prompt:       builder.BuildComparisonPrompt("Left panel at (1-4,0-1), Right panel at (1-4,3-4)", "Left panel at (1-4,0-1), Right panel at (1-4,3-4)"),
		},
	}
}

// Helper functions to create complex test images

func createDashboardImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 800, 600))

	// Background
	for y := 0; y < 600; y++ {
		for x := 0; x < 800; x++ {
			img.Set(x, y, color.RGBA{245, 247, 250, 255})
		}
	}

	// Chart 1 (top-left) - Revenue
	for y := 50; y < 250; y++ {
		for x := 20; x < 380; x++ {
			img.Set(x, y, color.White)
		}
	}

	// Chart 2 (top-right) - Users
	for y := 50; y < 250; y++ {
		for x := 420; x < 780; x++ {
			img.Set(x, y, color.White)
		}
	}

	// Metric boxes (bottom)
	for y := 300; y < 400; y++ {
		for x := 20; x < 200; x++ {
			img.Set(x, y, color.RGBA{76, 175, 80, 255}) // Green
		}
	}
	for y := 300; y < 400; y++ {
		for x := 220; x < 400; x++ {
			img.Set(x, y, color.RGBA{33, 150, 243, 255}) // Blue
		}
	}

	return img
}

func createNavigationImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 800, 100))

	// Navigation bar background
	for y := 0; y < 50; y++ {
		for x := 0; x < 800; x++ {
			img.Set(x, y, color.RGBA{33, 33, 33, 255})
		}
	}

	// Menu items
	menuPositions := []struct{ x1, x2 int }{
		{50, 150},   // Home
		{200, 300},  // Products
		{350, 450},  // About
		{500, 600},  // Contact
	}

	for _, pos := range menuPositions {
		for y := 10; y < 40; y++ {
			for x := pos.x1; x < pos.x2; x++ {
				img.Set(x, y, color.RGBA{255, 255, 255, 255})
			}
		}
	}

	// Dropdown under Products
	for y := 55; y < 85; y++ {
		for x := 200; x < 300; x++ {
			img.Set(x, y, color.White)
		}
	}

	return img
}

func createTableImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 600, 300))

	// Background
	for y := 0; y < 300; y++ {
		for x := 0; x < 600; x++ {
			img.Set(x, y, color.White)
		}
	}

	// Header row
	for y := 0; y < 40; y++ {
		for x := 0; x < 600; x++ {
			img.Set(x, y, color.RGBA{240, 240, 240, 255})
		}
	}

	// Grid lines
	for i := 1; i < 5; i++ {
		for y := 40; y < 300; y++ {
			img.Set(i*120, y, color.RGBA{230, 230, 230, 255})
		}
	}

	return img
}

func createTabInterfaceImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 600, 400))

	// Background
	for y := 0; y < 400; y++ {
		for x := 0; x < 600; x++ {
			img.Set(x, y, color.White)
		}
	}

	// Tab bar background
	for y := 0; y < 40; y++ {
		for x := 0; x < 600; x++ {
			img.Set(x, y, color.RGBA{245, 245, 245, 255})
		}
	}

	// Tabs
	tabPositions := []struct{ x1, x2 int }{
		{10, 110},   // Overview
		{120, 220},  // Details (active)
		{230, 330},  // Settings
	}

	for i, pos := range tabPositions {
		for y := 0; y < 40; y++ {
			for x := pos.x1; x < pos.x2; x++ {
				if i == 1 {
					// Active tab
					img.Set(x, y, color.RGBA{33, 150, 243, 255})
				} else {
					img.Set(x, y, color.RGBA{220, 220, 220, 255})
				}
			}
		}
	}

	// Active tab panel
	for y := 45; y < 350; y++ {
		for x := 10; x < 580; x++ {
			img.Set(x, y, color.RGBA{250, 250, 255, 255})
		}
	}

	return img
}
