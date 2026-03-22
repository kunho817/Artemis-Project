// Package prompts provides spatial reasoning prompts for visual testing.
package prompts

import (
	"fmt"
	"strings"
)

// SpatialPromptBuilder builds prompts for spatial reasoning and validation.
type SpatialPromptBuilder struct {
	includeDirections   bool
	includeDistances    bool
	includeAlignment    bool
	includeGeometry    bool
	strictMode         bool // Require exact positioning vs allow approximate
}

// NewSpatialPromptBuilder creates a new spatial prompt builder.
func NewSpatialPromptBuilder() *SpatialPromptBuilder {
	return &SpatialPromptBuilder{
		includeDirections: true,
		includeDistances:   true,
		includeAlignment:   true,
		includeGeometry:   true,
		strictMode:        false, // Allow approximate descriptions
	}
}

// SetStrictMode enables strict spatial validation.
func (spb *SpatialPromptBuilder) SetStrictMode(strict bool) *SpatialPromptBuilder {
	spb.strictMode = strict
	return spb
}

// IncludeDirections controls whether to include direction analysis.
func (spb *SpatialPromptBuilder) IncludeDirections(include bool) *SpatialPromptBuilder {
	spb.includeDirections = include
	return spb
}

// BuildDirectionalityPrompt creates a prompt for verifying directional relationships.
func (spb *SpatialPromptBuilder) BuildDirectionalityPrompt(spec string) string {
	strictness := "approximately"
	if spb.strictMode {
		strictness = "exactly"
	}

	return fmt.Sprintf(`# Directionality Verification

## Layout Specification
%s

## Task
Verify the directional relationships between elements in the image.

## Instructions
1. Identify each element mentioned in the specification
2. For each relationship, verify the direction is correct
3. Use grid coordinates to be precise

## Direction Reference
- **Up**: Element B is above Element A (lower row number)
- **Down**: Element B is below Element A (higher row number)
- **Left**: Element B is to the left of Element A (lower column number)
- **Right**: Element B is to the right of Element A (higher column number)
- **Adjacent**: Directly touching (no gap in grid cells)

## Response Format
For each direction to verify:
- **Elements**: [Element A → Element B]
- **Expected Direction**: [from spec]
- **Actual Direction**: [up/down/left/right]
- **Offset**: [grid cells, %s]
- **Verdict**: [MATCH/MISMATCH]

## Accuracy Standard
Relationships must match as specified in the layout spec.
Vague descriptions like "near" or "around" are not acceptable.
`, spec, strictness)
}

// BuildDistancePrompt creates a prompt for measuring distances between elements.
func (spb *SpatialPromptBuilder) BuildDistancePrompt(spec string) string {
	return fmt.Sprintf(`# Distance Verification

## Layout Specification
%s

## Task
Measure distances between UI elements using grid coordinates.

## Instructions
1. Identify center points of elements (or nearest grid cell)
2. Calculate Manhattan distance: |row_diff| + |col_diff|
3. Report distances in grid cells

## Distance Categories
- **Adjacent**: 1-2 cells (touching or nearly touching)
- **Near**: 3-5 cells (within same region)
- **Medium**: 6-10 cells (different regions)
- **Far**: 11+ cells (opposite sides)

## Response Format
For each distance to verify:
- **Elements**: [Element A and Element B]
- **Expected Distance**: [from spec]
- **Actual Distance**: [grid cells]
- **Verdict**: [MATCH/MISMATCH]
- **Notes**: [if discrepancy, explain difference]

## Example
If Element A is at (2,3) and Element B is at (5,7):
- Row difference: |5-2| = 3 cells
- Column difference: |7-3| = 4 cells
- Manhattan distance: 3 + 4 = 7 cells

## Precision
- Report distances to the nearest grid cell
- For elements spanning multiple cells, use center point
- If specification gives range, use midpoint of range
`, spec)
}

// BuildAlignmentPrompt creates a prompt for verifying element alignment.
func (spb *SpatialPromptBuilder) BuildAlignmentPrompt(spec string) string {
	alignmentType := "approximate"
	if spb.strictMode {
		alignmentType = "exact"
	}

	return fmt.Sprintf(`# Alignment Verification

## Layout Specification
%s

## Task
Verify alignment relationships between UI elements.

## Alignment Types

### Horizontal Alignment
- **Left Aligned**: Same or similar column positions
- **Center Aligned**: Centers aligned vertically
- **Right Aligned**: Same or similar right edge positions
- **Justified**: Distributed across available space

### Vertical Alignment
- **Top Aligned**: Same or similar row positions
- **Middle Aligned**: Centers aligned horizontally
- **Bottom Aligned**: Same or similar bottom edge positions

### Grid-Based Alignment
For grid-based alignment:
- **Row Match**: Same row number
- **Column Match**: Same column number
- **Center Alignment**: Centers align on both axes

## Response Format
For each alignment to verify:
- **Elements**: [List of elements]
- **Alignment Type**: [horizontal/vertical/center]
- **Expected**: [from spec]
- **Actual**: [observed alignment]
- **Tolerance**: [%s allowed]

## Verification Method
1. Check grid coordinates of each element
2. Compare center points or edge positions
3. Determine if alignment matches specification
4. Report: PASS (within tolerance) or FAIL (outside tolerance)

## Tolerance Guidelines
- %s alignment means coordinates differ by ≤1 grid cell
- Exact alignment means coordinates match exactly (same row/col)
`, spec, alignmentType, alignmentType)
}

// BuildGeometryPrompt creates a prompt for verifying geometric properties.
func (spb *SpatialPromptBuilder) BuildGeometryPrompt(spec string) string {
	return fmt.Sprintf(`# Geometric Verification

## Layout Specification
%s

## Task
Verify geometric properties of UI elements.

## Geometric Properties to Check

### Element Size
- **Width**: Number of columns spanned
- **Height**: Number of rows spanned
- **Aspect Ratio**: width / height ratio

### Position
- **Bounding Box**: (min_row, max_row, min_col, max_col)
- **Center Point**: Average of min/max rows and cols

### Layout Properties
- **Spacing**: Gap between adjacent elements
- **Margins**: Distance from screen edges
- **Density**: Number of elements per area

## Response Format
For each geometric property:
- **Element**: [element name]
- **Property**: [size/position/etc]
- **Expected**: [from spec]
- **Actual**: [measured from image]
- **Verdict**: [MATCH/MISMATCH]

## Measurement Method
1. Identify which grid cells the element occupies
2. Count rows and columns
3. Calculate bounding box
4. Compare against specification

## Precision
- Report measurements in grid cells (not pixels)
- For partial cells, count if element covers >50%% of cell
- Round to nearest grid cell
`, spec)
}

// BuildFullSpatialPrompt creates a comprehensive spatial analysis prompt.
func (spb *SpatialPromptBuilder) BuildFullSpatialPrompt(question string) string {
	var sb strings.Builder

	sb.WriteString("# Comprehensive Spatial Analysis\n\n")
	sb.WriteString(fmt.Sprintf("## Analysis Task\n%s\n\n", question))

	sb.WriteString("## Spatial Analysis Steps\n\n")
	sb.WriteString("### 1. Element Identification\n")
	sb.WriteString("- List all UI elements with their types\n")
	sb.WriteString("- Note approximate sizes and positions\n\n")

	if spb.includeDirections {
		sb.WriteString("### 2. Directional Relationships\n")
		sb.WriteString("- For key element pairs, describe directions\n")
		sb.WriteString("- Use 8-point compass (N, NE, E, SE, S, SW, W, NW)\n")
		sb.WriteString("- Report directionality between elements\n\n")
	}

	if spb.includeDistances {
		sb.WriteString("### 3. Distance Measurement\n")
		sb.WriteString("- Calculate Manhattan distances between elements\n")
		sb.WriteString("- Report distances in grid cells\n")
		sb.WriteString("- Categorize: adjacent (<3), near (3-5), medium (6-10), far (>10)\n\n")
	}

	if spb.includeAlignment {
		sb.WriteString("### 4. Alignment Verification\n")
		sb.WriteString("- Check horizontal alignment (left/center/right)\n")
		sb.WriteString("- Check vertical alignment (top/middle/bottom)\n")
		sb.WriteString("- Note elements that align vs. each other\n\n")
	}

	if spb.includeGeometry {
		sb.WriteString("### 5. Geometric Properties\n")
		sb.WriteString("- Measure element sizes (width × height in grid cells)\n")
		sb.WriteString("- Calculate aspect ratios\n")
		sb.WriteString("- Identify bounding boxes\n\n")
	}

	sb.WriteString("## Response Format\n")
	sb.WriteString("Provide a structured analysis with:\n")
	sb.WriteString("- Elements: [list with positions]\n")
	sb.WriteString("- Relationships: [directional/spatial]\n")
	sb.WriteString("- Measurements: [distances/alignments]\n")
	sb.WriteString("- Summary: [key findings]\n")

	return sb.String()
}

// BuildLayoutComparisonPrompt creates a prompt for comparing two layouts.
func (spb *SpatialPromptBuilder) BuildLayoutComparisonPrompt(layout1, layout2 string) string {
	return fmt.Sprintf(`# Spatial Layout Comparison

## Layout 1
%s

## Layout 2
%s

## Comparison Task
Compare the spatial layouts and identify differences.

## Comparison Dimensions

### 1. Element Presence
- Which elements exist in Layout 1 but not Layout 2?
- Which elements exist in Layout 2 but not Layout 1?
- Which elements exist in both?

### 2. Position Differences
- For shared elements, how have positions changed?
- Report displacement in grid cells (direction + distance)
- Note any significant repositioning

### 3. Relationship Changes
- Have directional relationships changed?
- Have alignments been modified?
- Are spatial relationships preserved?

### 4. Overall Layout Structure
- Is the overall layout structure preserved?
- Has the density/spacing changed significantly?
- Are there major geometric differences?

## Response Format
- **Overall**: [identical/similar/different]
- **Element Presence**: [additions/removals]
- **Position Shifts**: [list of movements]
- **Relationship Changes**: [modified relationships]
- **Impact Assessment**: [how changes affect UX]

## Comparison Method
Use grid coordinates for precise measurements. Report both absolute positions and relative displacements.
`, layout1, layout2)
}

// BuildHallucinationDetectionPrompt creates a prompt to detect "배치 무시 환각".
func (spb *SpatialPromptBuilder) BuildHallucinationDetectionPrompt(description string) string {
	return fmt.Sprintf(`# Hallucination Detection for Layout Claims

## Claimed Layout
%s

## Task
Verify if the claimed layout actually matches the visual evidence.

## Hallucination Detection Steps

### Step 1: Claim Analysis
- Parse the layout claim into specific assertions
- Identify elements, positions, and relationships mentioned
- Extract testable predictions

### Step 2: Visual Verification
- Examine the image systematically
- For each assertion, check if visual evidence supports it
- Note discrepancies between claim and reality

### Step 3: Spatial Validation
- Use grid coordinates to verify positions
- Check directional relationships
- Measure distances and alignments
- Identify claims that are "present but elsewhere"

### Step 4: Hallucination Classification
- **True Positive**: Claim matches visual evidence exactly
- **Partial Hallucination**: Element exists but position/description is wrong
- **Complete Hallucination**: Element doesn't exist in the image
- **Placement Ignoring**: Element exists but claim ignores actual position

## Response Format
For each claim:
- **Claim**: [specific assertion from description]
- **Evidence**: [what you actually see]
- **Verdict**: [VERIFIED/HALLUCINATION]
- **Details**: [explanation of any discrepancy]

## Hallucination Types
- **Element Hallucination**: Claiming an element exists when it doesn't
- **Position Hallucination**: Describing wrong position for existing element
- **Relationship Hallucination**: Asserting relationships that don't exist
- **Direction Hallucination**: Getting directional relationships wrong

## Detection Criteria
A claim is a hallucination if:
1. Element doesn't exist but is claimed to exist
2. Element exists but is >2 grid cells away from claimed position
3. Directional relationship is opposite (e.g., "above" when actually "below")
4. Alignment claim differs by >1 grid cell

## Strictness Level
- **Strict Mode**: Every spatial claim must be exact
- **Lenient Mode**: Allow ±1 grid cell tolerance
- **Current Mode**: %s
`, description, map[bool]string{true: "Strict", false: "Lenient"}[spb.strictMode])
}

// BuildQuantitativePrompt creates a prompt for quantitative spatial analysis.
func (spb *SpatialPromptBuilder) BuildQuantitativePrompt(measurements []string) string {
	var sb strings.Builder

	sb.WriteString("# Quantitative Spatial Analysis\n\n")
	sb.WriteString("## Requested Measurements\n\n")
	for i, m := range measurements {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, m))
	}
	sb.WriteString("\n")

	sb.WriteString("## Measurement Instructions\n\n")
	sb.WriteString("For each requested measurement:\n")
	sb.WriteString("1. Identify relevant elements in the image\n")
	sb.WriteString("2. Determine positions using grid coordinates\n")
	sb.WriteString("3. Calculate the requested metric\n")
	sb.WriteString("4. Report the result with units\n\n")

	sb.WriteString("## Supported Measurements\n\n")
	sb.WriteString("- **Distance**: Manhattan or Euclidean distance in grid cells\n")
	sb.WriteString("- **Area**: Number of grid cells covered\n")
	sb.WriteString("- **Bounding Box**: (min_row, max_row, min_col, max_col)\n")
	sb.WriteString("- **Center Point**: Average position\n")
	sb.WriteString("- **Orientation**: Horizontal/Vertical/Diagonal\n")
	sb.WriteString("- **Density**: Elements per unit area\n")
	sb.WriteString("- **Spacing**: Gap between elements in grid cells\n")

	return sb.String()
}

// String returns the string representation of the builder.
func (spb *SpatialPromptBuilder) String() string {
	flags := []string{}
	if spb.includeDirections {
		flags = append(flags, "Directions")
	}
	if spb.includeDistances {
		flags = append(flags, "Distances")
	}
	if spb.includeAlignment {
		flags = append(flags, "Alignment")
	}
	if spb.includeGeometry {
		flags = append(flags, "Geometry")
	}

	mode := "Lenient"
	if spb.strictMode {
		mode = "Strict"
	}

	return fmt.Sprintf("SpatialPromptBuilder{%s, mode=%s}", strings.Join(flags, "+"), mode)
}

// PresetForLayoutValidation returns a pre-configured spatial prompt for layout validation.
func PresetForLayoutValidation() *SpatialPromptBuilder {
	spb := NewSpatialPromptBuilder()
	spb.SetStrictMode(false) // Allow approximate positions
	return spb
}

// PresetForHallucinationDetection returns a pre-configured prompt for detecting layout hallucinations.
func PresetForHallucinationDetection() *SpatialPromptBuilder {
	spb := NewSpatialPromptBuilder()
	spb.SetStrictMode(true) // Require exact matching
	return spb
}
