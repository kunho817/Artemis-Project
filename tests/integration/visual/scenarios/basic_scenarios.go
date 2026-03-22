// Package scenarios provides test scenarios for visual testing.
package scenarios

import (
	"image"
	"image/color"

	"github.com/artemis-project/artemis/tests/integration/visual/prompts"
)

// Scenario defines a visual test scenario.
type Scenario struct {
	Name        string
	Description string
	Image       image.Image
	Question    string
	Expected    []ExpectedElement
}

// ExpectedElement defines an expected UI element.
type ExpectedElement struct {
	Type        string // "button", "text_field", "label", etc.
	Label       string
	GridPosition string // e.g., "(2,3)" or "(2-3, 4-5)"
	Description string
}

// BasicScenarios returns simple test scenarios for visual testing.
func BasicScenarios() []Scenario {
	return []Scenario{
		{
			Name:        "Single Button",
			Description: "A single submit button centered on screen",
			Image:       createSingleButtonImage(),
			Question:    "Describe the UI elements you see and their positions.",
			Expected: []ExpectedElement{
				{
					Type:        "button",
					Label:       "Submit",
					GridPosition: "(2,2)",
					Description: "Green submit button at center",
				},
			},
		},
		{
			Name:        "Login Form",
			Description: "A standard login form with username, password fields",
			Image:       createLoginFormImage(),
			Question:    "Describe the login form layout and identify all input fields.",
			Expected: []ExpectedElement{
				{
					Type:        "label",
					Label:       "Username",
					GridPosition: "(1,1)",
					Description: "Username label",
				},
				{
					Type:        "text_field",
					Label:       "",
					GridPosition: "(2,1)",
					Description: "Username input field",
				},
				{
					Type:        "label",
					Label:       "Password",
					GridPosition: "(3,1)",
					Description: "Password label",
				},
				{
					Type:        "text_field",
					Label:       "",
					GridPosition: "(4,1)",
					Description: "Password input field",
				},
				{
					Type:        "button",
					Label:       "Login",
					GridPosition: "(5,1)",
					Description: "Login button",
				},
			},
		},
		{
			Name:        "Two Column Layout",
			Description: "Content split into two columns",
			Image:       createTwoColumnImage(),
			Question:    "Describe the layout structure and how elements are arranged.",
			Expected: []ExpectedElement{
				{
					Type:        "label",
					Label:       "Left Column",
					GridPosition: "(1,1)",
					Description: "Left column header",
				},
				{
					Type:        "label",
					Label:       "Right Column",
					GridPosition: "(1,3)",
					Description: "Right column header",
				},
			},
		},
	}
}

// GetGridBasedTestPrompts returns scenarios formatted for grid-based analysis.
func GetGridBasedTestPrompts() []struct {
	Name     string
	Image    image.Image
	Question string
	Prompt   string
} {
	builder := prompts.NewGridPromptBuilder(5, 5)
	scenarios := BasicScenarios()

	result := make([]struct {
		Name     string
		Image    image.Image
		Question string
		Prompt   string
	}, len(scenarios))

	for i, scenario := range scenarios {
		result[i].Name = scenario.Name
		result[i].Image = scenario.Image
		result[i].Question = scenario.Question
		result[i].Prompt = builder.BuildPromptForImage(scenario.Image, scenario.Question)
	}

	return result
}

// Helper functions to create test images

func createSingleButtonImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 400, 300))
	// Fill background with white
	for y := 0; y < 300; y++ {
		for x := 0; x < 400; x++ {
			img.Set(x, y, color.White)
		}
	}
	// Draw a green rectangle in the center (simulating a button)
	for y := 120; y < 180; y++ {
		for x := 150; x < 250; x++ {
			img.Set(x, y, color.RGBA{0, 200, 0, 255})
		}
	}
	return img
}

func createLoginFormImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 400, 300))
	// Fill background with light gray
	for y := 0; y < 300; y++ {
		for x := 0; x < 400; x++ {
			img.Set(x, y, color.RGBA{240, 240, 240, 255})
		}
	}
	// Draw username field
	for y := 50; y < 80; y++ {
		for x := 100; x < 300; x++ {
			img.Set(x, y, color.White)
		}
	}
	// Draw password field
	for y := 100; y < 130; y++ {
		for x := 100; x < 300; x++ {
			img.Set(x, y, color.White)
		}
	}
	// Draw login button
	for y := 150; y < 180; y++ {
		for x := 150; x < 250; x++ {
			img.Set(x, y, color.RGBA{0, 100, 200, 255})
		}
	}
	return img
}

func createTwoColumnImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 600, 400))
	// Fill background with white
	for y := 0; y < 400; y++ {
		for x := 0; x < 600; x++ {
			img.Set(x, y, color.White)
		}
	}
	// Draw divider line
	for y := 0; y < 400; y++ {
		img.Set(300, y, color.Gray{200})
	}
	return img
}
