// Package prompts provides specialized prompt builders for visual testing.
package prompts

import (
	"fmt"
	"image"
	"strings"
)

// GridPromptBuilder builds grid-based analysis prompts.
// This divides the screen into a coordinate grid to improve spatial reasoning.
type GridPromptBuilder struct {
	rows int
	cols int
}

// NewGridPromptBuilder creates a new grid prompt builder.
func NewGridPromptBuilder(rows, cols int) *GridPromptBuilder {
	if rows < 2 || cols < 2 {
		rows, cols = 5, 5 // Default 5x5 grid
	}
	return &GridPromptBuilder{rows: rows, cols: cols}
}

// SetGridSize sets the grid dimensions.
func (gpb *GridPromptBuilder) SetGridSize(rows, cols int) *GridPromptBuilder {
	gpb.rows = rows
	gpb.cols = cols
	return gpb
}

// BuildPrompt creates a grid-based analysis prompt for an image.
func (gpb *GridPromptBuilder) BuildPrompt(question string) string {
	bounds := gpb.estimateImageSize()
	cellWidth := bounds.Dx() / gpb.cols
	cellHeight := bounds.Dy() / gpb.rows

	gridViz := gpb.generateGridVisualization()

	return fmt.Sprintf(`# Screen Analysis with Grid Coordinates

## Grid System
The screen is divided into a %dx%d grid for precise element location:
- Each cell size: %dx%d pixels
- Coordinate origin (0,0) = top-left corner
- Maximum coordinates: (%d, %d) = bottom-right corner

## Grid Layout
%s

## Analysis Task
%s

## Instructions
1. **Element Identification**: List all UI elements you can see
2. **Grid Location**: For each element, specify:
   - Grid cell(s) it occupies (e.g., "R2,C3" for row 2, column 3)
   - Relative position (e.g., "top-left", "center", "bottom-right")
3. **Spatial Relationships**: Describe how elements relate to each other
4. **Verification**: Confirm your analysis matches the actual visual layout

## Response Format
For each element, provide:
- **Element**: [button/text field/label/etc.]
- **Grid Position**: [row, col] or range
- **Size**: [approx. cells occupied]
- **Description**: [what it shows/does]

## Important Rules
- ALWAYS verify element positions against the grid
- Use specific grid coordinates, not vague terms like "near" or "around"
- If uncertain about exact position, give a range (e.g., "R2-3, C4-5")
- Report elements in reading order (top to bottom, left to right)
`, gpb.rows, gpb.cols, cellWidth, cellHeight, gpb.cols-1, gpb.rows-1, gridViz, question)
}

// generateGridVisualization creates a text-based grid visualization.
func (gpb *GridPromptBuilder) generateGridVisualization() string {
	var sb strings.Builder
	sb.WriteString("```\n")
	sb.WriteString("    ")
	for c := 0; c < gpb.cols; c++ {
		sb.WriteString(fmt.Sprintf("C%-2d", c))
	}
	sb.WriteString("\n")

	for r := 0; r < gpb.rows; r++ {
		sb.WriteString(fmt.Sprintf("R%-2d ", r))
		for c := 0; c < gpb.cols; c++ {
			sb.WriteString("[  ]")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("```\n")

	return sb.String()
}

// BuildPromptForImage creates a grid prompt with image dimensions.
func (gpb *GridPromptBuilder) BuildPromptForImage(img image.Image, question string) string {
	bounds := img.Bounds()
	cellWidth := bounds.Dx() / gpb.cols
	cellHeight := bounds.Dy() / gpb.rows

	return fmt.Sprintf(`# Image Analysis with Grid Coordinates

## Image Dimensions
- Width: %d pixels
- Height: %d pixels
- Grid size: %dx%d
- Cell size: %dx%d pixels

## Grid Coordinates
- (0,0) = top-left
- (%d,%d) = bottom-right

%s

## Grid-Based Analysis Instructions
1. Divide the image mentally into equal cells
2. For each UI element, identify which cell(s) it occupies
3. Report positions using (row,col) notation

Example response format:
- Element: "Submit button"
- Position: (4,2) to (4,3) [spans 2 columns in row 4]
- Description: Green button at bottom-center
`, bounds.Dx(), bounds.Dy(), gpb.rows, gpb.cols, cellWidth, cellHeight,
	gpb.cols-1, gpb.rows-1, gpb.generateGridVisualization())
}

// estimateImageSize estimates typical image dimensions.
func (gpb *GridPromptBuilder) estimateImageSize() *image.Rectangle {
	// Typical terminal screen size
	return &image.Rectangle{
		Min: image.Point{X: 0, Y: 0},
		Max: image.Point{X: 1200, Y: 800},
	}
}

// BuildElementLocationPrompt creates a prompt for finding specific elements.
func (gpb *GridPromptBuilder) BuildElementLocationPrompt(elementType, elementDesc string) string {
	return fmt.Sprintf(`# Find Element Location Using Grid

## Target Element
- Type: %s
- Description: %s

## Task
Using the %dx%d grid system:
1. Scan each grid cell systematically
2. Identify which cell(s) contain the element
3. Report the exact grid coordinates

## Response Format
- **Found**: [yes/no]
- **Grid Location**: [row,col or range]
- **Confidence**: [high/medium/low]
- **Visual Description**: [what the element looks like]

If the element spans multiple cells, report as: (r1-r2, c1-c2)
`, elementType, elementDesc, gpb.rows, gpb.cols)
}

// BuildComparisonPrompt creates a prompt for comparing two layouts.
func (gpb *GridPromptBuilder) BuildComparisonPrompt(expectedDesc, actualDesc string) string {
	return fmt.Sprintf(`# Layout Comparison Using Grid Coordinates

## Expected Layout
%s

## Actual Layout
%s

## Comparison Task
Using the %dx%d grid system:
1. Map each element from expected layout to grid coordinates
2. Map each element from actual layout to grid coordinates
3. Compare positions:
   - Which elements are in the same grid cell? (position match)
   - Which elements are in different cells? (position mismatch)
   - Are there elements present in one but not the other?

## Response Format
For each element:
- **Element**: [name]
- **Expected Position**: [grid coordinates]
- **Actual Position**: [grid coordinates]
- **Match**: [yes/no]
- **Difference**: [if mismatched, describe displacement]

## Final Verdict
- Overall Layout Match: [exact/similar/different]
- Displacement Summary: [brief description of differences]
`, expectedDesc, actualDesc, gpb.rows, gpb.cols)
}

// BuildCoordinateSystemPrompt creates a prompt for understanding coordinate systems.
func (gpb *GridPromptBuilder) BuildCoordinateSystemPrompt() string {
	return fmt.Sprintf(`# Grid Coordinate System Reference

## System Overview
The screen is divided into a %dx%d grid for precise element positioning.

## Coordinate Notation

### Single Cell
Format: (row, col)
Example: (2, 3) = row 2, column 3

### Cell Range
Format: (r1-r2, c1-c2)
Example: (2-4, 3-5) = rows 2-4, columns 3-5

### Center Point
Format: center(row, col) or center(r1-r2, c1-c2)
Example: center(2, 3) = center of cell (2, 3)

## Directions
- **Up**: Decreasing row number (e.g., (3,2) → (2,2))
- **Down**: Increasing row number (e.g., (3,2) → (4,2))
- **Left**: Decreasing column number (e.g., (3,4) → (3,3))
- **Right**: Increasing column number (e.g., (3,2) → (3,3))

## Grid Regions
- **Top-Left**: Rows 0-%d, Cols 0-%d
- **Top-Right**: Rows 0-%d, Cols %d-%d
- **Bottom-Left**: Rows %d-%d, Cols 0-%d
- **Bottom-Right**: Rows %d-%d, Cols %d-%d
- **Center**: Rows %d-%d, Cols %d-%d
`, gpb.rows-1, gpb.cols-1,
gpb.rows-1, gpb.cols/2, gpb.cols-1,
gpb.rows/2, gpb.rows-1, 0, gpb.cols-1,
gpb.rows/2, gpb.rows-1, gpb.cols/2, gpb.cols/2, gpb.cols-1,
gpb.rows/4, (gpb.rows*3)/4, gpb.cols/4, (gpb.cols*3)/4)
}

// BuildSpatialQueryPrompt creates a prompt for spatial queries.
func (gpb *GridPromptBuilder) BuildSpatialQueryPrompt(reference string, target string) string {
	return fmt.Sprintf(`# Spatial Relationship Query

## Reference Element
%s

## Target Question
%s

## Instructions
Using the %dx%d grid system:

1. **Locate Reference**: Find grid coordinates of reference element
2. **Locate Target**: Find grid coordinates of target element
3. **Calculate Relationship**:
   - Direction: [up/down/left/right/diagonal]
   - Distance: [number of cells]
   - Alignment: [left/center/right aligned, top/bottom/middle aligned]

## Response Format
- **Reference Position**: [grid coords]
- **Target Position**: [grid coords]
- **Direction**: [target is relative to reference]
- **Distance**: [cell count]
- **Alignment**: [horizontal/vertical]
- **Spatial Relation**: [e.g., "target is 2 cells below and 1 cell left of reference"]

## Example
If reference is at (3,3) and target is at (5,2):
- Direction: down-left
- Distance: 3 cells (2 down + 1 left)
- Alignment: not aligned
`, reference, target, gpb.rows, gpb.cols)
}

// String returns the string representation of the builder.
func (gpb *GridPromptBuilder) String() string {
	return fmt.Sprintf("GridPromptBuilder{%dx%d}", gpb.rows, gpb.cols)
}
