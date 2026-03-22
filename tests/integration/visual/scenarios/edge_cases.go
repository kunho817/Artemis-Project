// Package scenarios provides edge case scenarios for visual testing.
package scenarios

import (
	"image"
	"image/color"

	"github.com/artemis-project/artemis/tests/integration/visual/prompts"
)

// EdgeCaseScenarios returns edge case scenarios that test boundary conditions.
func EdgeCaseScenarios() []Scenario {
	return []Scenario{
		{
			Name:        "Overlapping Elements",
			Description: "UI elements that visually overlap",
			Image:       createOverlappingElementsImage(),
			Question:    "Identify all UI elements. Note any overlaps or z-index issues.",
			Expected: []ExpectedElement{
				{Type: "button", Label: "Primary", GridPosition: "(2,2)", Description: "Primary button (front)"},
				{Type: "button", Label: "Secondary", GridPosition: "(2,2)", Description: "Secondary button (behind)"},
			},
		},
		{
			Name:        "Very Small Elements",
			Description: "Tiny UI elements that might be missed",
			Image:       createSmallElementsImage(),
			Question:    "Find and describe all UI elements, including very small ones.",
			Expected: []ExpectedElement{
				{Type: "icon", Label: "Settings", GridPosition: "(0,4)", Description: "Small settings icon"},
				{Type: "icon", Label: "Help", GridPosition: "(0,4)", Description: "Tiny help icon next to settings"},
			},
		},
		{
			Name:        "Empty Screen",
			Description: "A blank screen with no UI elements",
			Image:       createEmptyScreenImage(),
			Question:    "Describe what you see on this screen.",
			Expected:    []ExpectedElement{},
		},
		{
			Name:        "Nearly Identical Colors",
			Description: "Elements with similar colors that blend together",
			Image:       createSimilarColorsImage(),
			Question:    "Identify all distinct UI elements despite similar colors.",
			Expected: []ExpectedElement{
				{Type: "panel", Label: "Panel A", GridPosition: "(1,1)", Description: "Light gray panel"},
				{Type: "panel", Label: "Panel B", GridPosition: "(1,2)", Description: "Slightly darker gray panel"},
				{Type: "panel", Label: "Panel C", GridPosition: "(2,1)", Description: "Medium gray panel"},
			},
		},
		{
			Name:        "Dense Information Display",
			Description: "Screen packed with text and data",
			Image:       createDenseInfoImage(),
			Question:    "Analyze this information-dense screen. Identify all sections and data points.",
			Expected: []ExpectedElement{
				{Type: "header", Label: "Table 1", GridPosition: "(0,0)", Description: "First table header"},
				{Type: "data", Label: "", GridPosition: "(1-3,0)", Description: "Dense data rows"},
				{Type: "header", Label: "Table 2", GridPosition: "(0,1)", Description: "Second table header"},
				{Type: "data", Label: "", GridPosition: "(1-3,1)", Description: "More dense data"},
			},
		},
		{
			Name:        "Off-Center Elements",
			Description: "Elements positioned at unusual coordinates",
			Image:       createOffCenterImage(),
			Question:    "Describe the exact positions of all elements using grid coordinates.",
			Expected: []ExpectedElement{
				{Type: "button", Label: "Button", GridPosition: "(1,0)", Description: "Button at far left"},
				{Type: "button", Label: "Button", GridPosition: "(3,4)", Description: "Button at bottom right"},
			},
		},
	}
}

// GetSpatialQueryScenarios returns scenarios for spatial relationship testing.
func GetSpatialQueryScenarios() []struct {
	Name      string
	Reference string
	Target    string
	Prompt    string
} {
	builder := prompts.NewGridPromptBuilder(5, 5)

	return []struct {
		Name      string
		Reference string
		Target    string
		Prompt    string
	}{
		{
			Name:      "Button Above Form",
			Reference: "Submit button at (1,2)",
			Target:    "Where is the login form relative to the submit button?",
			Prompt:    builder.BuildSpatialQueryPrompt("Submit button at (1,2)", "Where is the login form?"),
		},
		{
			Name:      "Logo to Navigation",
			Reference: "Logo at (0,0)",
			Target:    "Where is the navigation menu relative to the logo?",
			Prompt:    builder.BuildSpatialQueryPrompt("Logo at (0,0)", "Where is the navigation menu?"),
		},
		{
			Name:      "Sidebar to Content",
			Reference: "Sidebar spanning rows (0-4), column 0",
			Target:    "Where is the main content area relative to the sidebar?",
			Prompt:    builder.BuildSpatialQueryPrompt("Sidebar at (0-4,0)", "Where is the main content?"),
		},
	}
}

// GetElementLocationScenarios returns scenarios for element finding tests.
func GetElementLocationScenarios() []struct {
	Name        string
	ElementType string
	ElementDesc string
	Prompt      string
} {
	builder := prompts.NewGridPromptBuilder(5, 5)

	return []struct {
		Name        string
		ElementType string
		ElementDesc string
		Prompt      string
	}{
		{
			Name:        "Find Submit Button",
			ElementType: "button",
			ElementDesc: "Green submit button with 'Submit' label",
			Prompt:      builder.BuildElementLocationPrompt("button", "Green submit button with 'Submit' label"),
		},
		{
			Name:        "Find Search Bar",
			ElementType: "text_field",
			ElementDesc: "Search input field with placeholder text",
			Prompt:      builder.BuildElementLocationPrompt("text_field", "Search input field"),
		},
		{
			Name:        "Find Close Icon",
			ElementType: "icon",
			ElementDesc: "X-shaped close icon in top-right corner",
			Prompt:      builder.BuildElementLocationPrompt("icon", "X-shaped close icon"),
		},
	}
}

// Helper functions to create edge case images

func createOverlappingElementsImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 400, 300))

	// Background
	for y := 0; y < 300; y++ {
		for x := 0; x < 400; x++ {
			img.Set(x, y, color.White)
		}
	}

	// Button 1 (blue, behind)
	for y := 100; y < 150; y++ {
		for x := 150; x < 250; x++ {
			img.Set(x, y, color.RGBA{100, 100, 255, 255})
		}
	}

	// Button 2 (green, front - slightly offset)
	for y := 105; y < 145; y++ {
		for x := 155; x < 245; x++ {
			img.Set(x, y, color.RGBA{50, 200, 50, 255})
		}
	}

	return img
}

func createSmallElementsImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 400, 300))

	// Background
	for y := 0; y < 300; y++ {
		for x := 0; x < 400; x++ {
			img.Set(x, y, color.White)
		}
	}

	// Tiny icon at (380, 10) - 5x5 pixels
	for y := 10; y < 15; y++ {
		for x := 380; x < 385; x++ {
			img.Set(x, y, color.RGBA{100, 100, 100, 255})
		}
	}

	// Even smaller icon at (375, 12) - 3x3 pixels
	for y := 12; y < 15; y++ {
		for x := 375; x < 378; x++ {
			img.Set(x, y, color.RGBA{150, 150, 150, 255})
		}
	}

	return img
}

func createEmptyScreenImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 400, 300))
	// Pure white background
	for y := 0; y < 300; y++ {
		for x := 0; x < 400; x++ {
			img.Set(x, y, color.White)
		}
	}
	return img
}

func createSimilarColorsImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 400, 300))

	// Panel A: RGB(240, 240, 240)
	for y := 50; y < 150; y++ {
		for x := 20; x < 140; x++ {
			img.Set(x, y, color.RGBA{240, 240, 240, 255})
		}
	}

	// Panel B: RGB(235, 235, 235)
	for y := 50; y < 150; y++ {
		for x := 160; x < 280; x++ {
			img.Set(x, y, color.RGBA{235, 235, 235, 255})
		}
	}

	// Panel C: RGB(238, 238, 238)
	for y := 170; y < 270; y++ {
		for x := 20; x < 140; x++ {
			img.Set(x, y, color.RGBA{238, 238, 238, 255})
		}
	}

	return img
}

func createDenseInfoImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 600, 400))

	// Background
	for y := 0; y < 400; y++ {
		for x := 0; x < 600; x++ {
			img.Set(x, y, color.White)
		}
	}

	// Dense grid of small cells
	cellSize := 20
	for row := 0; row < 10; row++ {
		for col := 0; col < 15; col++ {
			y := row * cellSize + 30
			x := col * (cellSize + 5) + 20

			// Alternate colors for visual density
			c := byte(200 + (row+col)%2*30)
			for cy := y; cy < y+cellSize && cy < 400; cy++ {
				for cx := x; cx < x+cellSize && cx < 600; cx++ {
					img.Set(cx, cy, color.RGBA{c, c, c, 255})
				}
			}
		}
	}

	return img
}

func createOffCenterImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 400, 300))

	// Background
	for y := 0; y < 300; y++ {
		for x := 0; x < 400; x++ {
			img.Set(x, y, color.White)
		}
	}

	// Button at far left
	for y := 50; y < 80; y++ {
		for x := 10; x < 60; x++ {
			img.Set(x, y, color.RGBA{100, 150, 255, 255})
		}
	}

	// Button at far right
	for y := 220; y < 250; y++ {
		for x := 300; x < 350; x++ {
			img.Set(x, y, color.RGBA{255, 100, 100, 255})
		}
	}

	// Button near top edge
	for y := 10; y < 30; y++ {
		for x := 150; x < 200; x++ {
			img.Set(x, y, color.RGBA{100, 255, 100, 255})
		}
	}

	// Button near bottom edge
	for y := 270; y < 290; y++ {
		for x := 180; x < 230; x++ {
			img.Set(x, y, color.RGBA{255, 255, 100, 255})
		}
	}

	return img
}
