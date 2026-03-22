// Package vision provides UI element definitions and spatial relationships.
package vision

import (
	"fmt"
)

// UIElementType represents the type of a UI element.
type UIElementType string

const (
	Button          UIElementType = "button"
	TextField       UIElementType = "text_field"
	Label           UIElementType = "label"
	Icon            UIElementType = "icon"
	Menu            UIElementType = "menu"
	Checkbox        UIElementType = "checkbox"
	RadioButton     UIElementType = "radio_button"
	Slider          UIElementType = "slider"
	Dropdown        UIElementType = "dropdown"
	Toggle          UIElementType = "toggle"
	Tab             UIElementType = "tab"
	NavigationBar   UIElementType = "navbar"
	SideBar         UIElementType = "sidebar"
	ToolBar         UIElementType = "toolbar"
	StatusBar       UIElementType = "status_bar"
	ProgressBar     UIElementType = "progress_bar"
	Spinner         UIElementType = "spinner"
	Dialog          UIElementType = "dialog"
	Window          UIElementType = "window"
	Panel           UIElementType = "panel"
	Card            UIElementType = "card"
	List            UIElementType = "list"
	Table           UIElementType = "table"
	Tree            UIElementType = "tree"
	Chart           UIElementType = "chart"
	ImageElement    UIElementType = "image"
	Video           UIElementType = "video"
	Link            UIElementType = "link"
	Header          UIElementType = "header"
	Footer          UIElementType = "footer"
	Container       UIElementType = "container"
	Separator       UIElementType = "separator"
	UnknownElement  UIElementType = "unknown"
)

// UIElement represents a detected UI element with rich metadata.
type UIElement struct {
	ID          string                 `json:"id"`
	Type        UIElementType          `json:"type"`
	Label       string                 `json:"label,omitempty"`
	Text        string                 `json:"text,omitempty"`
	BoundingBox Box                    `json:"bounding_box"`
	Confidence  float64                `json:"confidence"`
	Properties  map[string]interface{} `json:"properties,omitempty"`
	Parent      string                 `json:"parent,omitempty"`
	Children    []string               `json:"children,omitempty"`
}

// SpatialRelation describes the spatial relationship between two elements.
type SpatialRelation struct {
	Direction  string  `json:"direction"`  // "above", "below", "left", "right", "center", "overlap"
	Distance   int     `json:"distance"`   // in pixels or grid cells
	Alignment  string  `json:"alignment"`  // "left", "center", "right", "top", "middle", "bottom"
	IoU        float64 `json:"iou"`        // Intersection over Union
	Contains   bool    `json:"contains"`   // Whether A contains B
	Contained  bool    `json:"contained"`  // Whether A is contained by B
}

// RelativeTo calculates the spatial relationship between this element and another.
func (e *UIElement) RelativeTo(other *UIElement) SpatialRelation {
	box1 := e.BoundingBox
	box2 := other.BoundingBox

	relation := SpatialRelation{
		Direction: calculateDirection(&box1, &box2),
		Distance:  calculateDistance(&box1, &box2),
		Alignment: calculateAlignment(&box1, &box2),
		IoU:       box1.IoU(box2),
		Contains:  box1.Contains(box2.X, box2.Y) && box1.Contains(box2.X+box2.Width, box2.Y+box2.Height),
		Contained: box2.Contains(box1.X, box1.Y) && box2.Contains(box1.X+box1.Width, box1.Y+box1.Height),
	}

	return relation
}

// calculateDirection determines the primary direction from box1 to box2.
func calculateDirection(box1, box2 *Box) string {
	cx1, cy1 := box1.Center()
	cx2, cy2 := box2.Center()

	dx := cx2 - cx1
	dy := cy2 - cy1

	// Check for overlap
	iou := box1.IoU(*box2)
	if iou > 0.1 {
		return "overlap"
	}

	// Check for center alignment
	if abs(dx) < 10 && abs(dy) < 10 {
		return "center"
	}

	// Determine primary direction
	horizontal := "center"
	if abs(dx) > abs(dy) {
		if dx > 0 {
			horizontal = "right"
		} else {
			horizontal = "left"
		}
	}

	vertical := "center"
	if abs(dy) > abs(dx) {
		if dy > 0 {
			vertical = "below"
		} else {
			vertical = "above"
		}
	}

	// Combine directions if significant
	if abs(dx) > 20 && abs(dy) > 20 {
		return fmt.Sprintf("%s-%s", vertical, horizontal)
	}

	if abs(dx) > abs(dy) {
		return horizontal
	}
	return vertical
}

// calculateDistance calculates the Manhattan distance between boxes.
func calculateDistance(box1, box2 *Box) int {
	cx1, cy1 := box1.Center()
	cx2, cy2 := box2.Center()

	return abs(cx2-cx1) + abs(cy2-cy1)
}

// calculateAlignment determines the alignment between boxes.
func calculateAlignment(box1, box2 *Box) string {
	_, cy1 := box1.Center()
	_, cy2 := box2.Center()

	cx1, _ := box1.Center()
	cx2, _ := box2.Center()

	verticalTolerance := 10
	horizontalTolerance := 10

	var hAlign, vAlign string

	// Horizontal alignment
	if abs(cy1-cy2) < verticalTolerance {
		hAlign = "middle"
	} else if cy1 < cy2 {
		hAlign = "top"
	} else {
		hAlign = "bottom"
	}

	// Vertical alignment
	if abs(cx1-cx2) < horizontalTolerance {
		vAlign = "center"
	} else if cx1 < cx2 {
		vAlign = "left"
	} else {
		vAlign = "right"
	}

	if vAlign == "center" && hAlign == "middle" {
		return "center-aligned"
	}

	return fmt.Sprintf("%s-%s", vAlign, hAlign)
}

// abs returns the absolute value of an integer.
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// ElementGroup represents a group of related UI elements.
type ElementGroup struct {
	ID         string      `json:"id"`
	Type       string      `json:"type"`
	Elements   []UIElement `json:"elements"`
	BoundingBox Box        `json:"bounding_box"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

// LayoutPattern describes common UI layout patterns.
type LayoutPattern string

const (
	// VerticalStack: Elements stacked vertically
	VerticalStack LayoutPattern = "vertical_stack"
	// HorizontalStack: Elements arranged horizontally
	HorizontalStack LayoutPattern = "horizontal_stack"
	// GridLayout: Elements in a 2D grid
	GridLayout LayoutPattern = "grid_layout"
	// FormLayout: Label + input pairs
	FormLayout LayoutPattern = "form_layout"
	// NavigationLayout: Navigation + content
	NavigationLayout LayoutPattern = "navigation_layout"
	// CardLayout: Cards in a grid or list
	CardLayout LayoutPattern = "card_layout"
	// TabLayout: Tabbed interface
	TabLayout LayoutPattern = "tab_layout"
	// DialogLayout: Modal dialog
	DialogLayout LayoutPattern = "dialog_layout"
	// ListLayout: Vertical list of items
	ListLayout LayoutPattern = "list_layout"
	// UnknownLayout: Unrecognized pattern
	UnknownLayout LayoutPattern = "unknown"
)

// DetectLayoutPattern analyzes elements to detect the layout pattern.
func DetectLayoutPattern(elements []UIElement) LayoutPattern {
	if len(elements) == 0 {
		return UnknownLayout
	}

	// Calculate bounding box for all elements
	bounds := calculateGroupBounds(elements)

	// Check for common patterns

	// Check for dialog (small, centered box)
	if isDialogPattern(elements, bounds) {
		return DialogLayout
	}

	// Check for navigation layout
	if isNavigationPattern(elements) {
		return NavigationLayout
	}

	// Check for tab layout
	if isTabPattern(elements) {
		return TabLayout
	}

	// Check for form layout
	if isFormPattern(elements) {
		return FormLayout
	}

	// Check for grid layout
	if isGridPattern(elements, bounds) {
		return GridLayout
	}

	// Check for card layout
	if isCardPattern(elements) {
		return CardLayout
	}

	// Check for vertical stack
	if isVerticalStackPattern(elements) {
		return VerticalStack
	}

	// Check for horizontal stack
	if isHorizontalStackPattern(elements) {
		return HorizontalStack
	}

	// Check for list layout
	if isListPattern(elements) {
		return ListLayout
	}

	return UnknownLayout
}

// calculateGroupBounds calculates the bounding box for a group of elements.
func calculateGroupBounds(elements []UIElement) Box {
	if len(elements) == 0 {
		return Box{}
	}

	minX := elements[0].BoundingBox.X
	minY := elements[0].BoundingBox.Y
	maxX := elements[0].BoundingBox.X + elements[0].BoundingBox.Width
	maxY := elements[0].BoundingBox.Y + elements[0].BoundingBox.Height

	for _, elem := range elements[1:] {
		if elem.BoundingBox.X < minX {
			minX = elem.BoundingBox.X
		}
		if elem.BoundingBox.Y < minY {
			minY = elem.BoundingBox.Y
		}
		if elem.BoundingBox.X+elem.BoundingBox.Width > maxX {
			maxX = elem.BoundingBox.X + elem.BoundingBox.Width
		}
		if elem.BoundingBox.Y+elem.BoundingBox.Height > maxY {
			maxY = elem.BoundingBox.Y + elem.BoundingBox.Height
		}
	}

	return Box{
		X:      minX,
		Y:      minY,
		Width:  maxX - minX,
		Height: maxY - minY,
	}
}

// Pattern detection helpers

func isDialogPattern(elements []UIElement, bounds Box) bool {
	// Dialog: small box, contains button(s), centered
	area := bounds.Area()
	avgArea := 0
	for _, e := range elements {
		avgArea += e.BoundingBox.Area()
	}
	if len(elements) > 0 {
		avgArea /= len(elements)
	}

	// Check if contains dialog or window element
	for _, e := range elements {
		if e.Type == Dialog || e.Type == Window {
			return true
		}
	}

	// Small bounding box relative to total elements
	return area < avgArea*10 && len(elements) < 20
}

func isNavigationPattern(elements []UIElement) bool {
	// Check for navbar or sidebar
	for _, e := range elements {
		if e.Type == NavigationBar || e.Type == SideBar {
			return true
		}
	}
	return false
}

func isTabPattern(elements []UIElement) bool {
	// Check for tab elements
	tabCount := 0
	for _, e := range elements {
		if e.Type == Tab {
			tabCount++
		}
	}
	return tabCount >= 2
}

func isFormPattern(elements []UIElement) bool {
	// Form: label + text_field pairs
	labelCount := 0
	fieldCount := 0

	for _, e := range elements {
		if e.Type == Label {
			labelCount++
		} else if e.Type == TextField || e.Type == Dropdown {
			fieldCount++
		}
	}

	return labelCount >= 2 && fieldCount >= 2 && abs(labelCount-fieldCount) <= 2
}

func isGridPattern(elements []UIElement, bounds Box) bool {
	if len(elements) < 4 {
		return false
	}

	// Check if elements are roughly aligned in rows and columns
	rows := make(map[int]int)
	cols := make(map[int]int)

	for _, e := range elements {
		centerY := e.BoundingBox.Y + e.BoundingBox.Height/2
		centerX := e.BoundingBox.X + e.BoundingBox.Width/2

		rows[centerY/20]++ // Group by approximate rows
		cols[centerX/20]++ // Group by approximate cols
	}

	return len(rows) >= 2 && len(cols) >= 2
}

func isCardPattern(elements []UIElement) bool {
	// Card: multiple card or panel elements
	cardCount := 0
	for _, e := range elements {
		if e.Type == Card || e.Type == Panel {
			cardCount++
		}
	}
	return cardCount >= 2
}

func isVerticalStackPattern(elements []UIElement) bool {
	if len(elements) < 2 {
		return false
	}

	// Check if elements are primarily stacked vertically
	horizontalOverlap := 0
	for i := 0; i < len(elements)-1; i++ {
		e1 := elements[i]
		e2 := elements[i+1]

		// Check if horizontally aligned
		cx1 := e1.BoundingBox.X + e1.BoundingBox.Width/2
		cx2 := e2.BoundingBox.X + e2.BoundingBox.Width/2

		if abs(cx1-cx2) < 50 {
			horizontalOverlap++
		}
	}

	return horizontalOverlap > len(elements)/2
}

func isHorizontalStackPattern(elements []UIElement) bool {
	if len(elements) < 2 {
		return false
	}

	// Check if elements are primarily arranged horizontally
	verticalOverlap := 0
	for i := 0; i < len(elements)-1; i++ {
		e1 := elements[i]
		e2 := elements[i+1]

		// Check if vertically aligned
		cy1 := e1.BoundingBox.Y + e1.BoundingBox.Height/2
		cy2 := e2.BoundingBox.Y + e2.BoundingBox.Height/2

		if abs(cy1-cy2) < 50 {
			verticalOverlap++
		}
	}

	return verticalOverlap > len(elements)/2
}

func isListPattern(elements []UIElement) bool {
	// List: similar elements stacked vertically
	if len(elements) < 3 {
		return false
	}

	// Check if all elements are similar type
	typeCounts := make(map[UIElementType]int)
	for _, e := range elements {
		typeCounts[e.Type]++
	}

	// If most elements are the same type, it's likely a list
	maxCount := 0
	for _, count := range typeCounts {
		if count > maxCount {
			maxCount = count
		}
	}

	return maxCount >= len(elements)*2/3 && isVerticalStackPattern(elements)
}

// GroupByPosition groups elements by their spatial proximity.
func GroupByPosition(elements []UIElement, threshold int) []ElementGroup {
	if len(elements) == 0 {
		return nil
	}

	groups := []ElementGroup{}
	used := make(map[int]bool)

	for i := range elements {
		if used[i] {
			continue
		}

		group := ElementGroup{
			ID:       fmt.Sprintf("group-%d", len(groups)),
			Elements: []UIElement{elements[i]},
		}

		used[i] = true

		// Find nearby elements
		for j := i + 1; j < len(elements); j++ {
			if used[j] {
				continue
			}

			dist := calculateDistance(&elements[i].BoundingBox, &elements[j].BoundingBox)
			if dist <= threshold {
				group.Elements = append(group.Elements, elements[j])
				used[j] = true
			}
		}

		group.BoundingBox = calculateGroupBounds(group.Elements)
		groups = append(groups, group)
	}

	return groups
}

// FindNearest finds the nearest element(s) to a reference element.
func FindNearest(ref *UIElement, elements []UIElement, n int) []UIElement {
	if len(elements) == 0 || n <= 0 {
		return nil
	}

	type distancePair struct {
		element  UIElement
		distance int
	}

	pairs := make([]distancePair, 0, len(elements))

	for _, e := range elements {
		if e.ID == ref.ID {
			continue
		}

		dist := calculateDistance(&ref.BoundingBox, &e.BoundingBox)
		pairs = append(pairs, distancePair{element: e, distance: dist})
	}

	// Simple selection sort (for small n)
	for i := 0; i < minInt(n, len(pairs)) && i < len(pairs); i++ {
		for j := i + 1; j < len(pairs); j++ {
			if pairs[j].distance < pairs[i].distance {
				pairs[i], pairs[j] = pairs[j], pairs[i]
			}
		}
	}

	result := make([]UIElement, 0, n)
	for i := 0; i < n && i < len(pairs); i++ {
		result = append(result, pairs[i].element)
	}

	return result
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// DetectHierarchy detects parent-child relationships based on containment.
func DetectHierarchy(elements []UIElement) {
	for i := range elements {
		for j := range elements {
			if i == j {
				continue
			}

			if elements[j].BoundingBox.Contains(elements[i].BoundingBox.X, elements[i].BoundingBox.Y) &&
				elements[j].BoundingBox.Contains(
					elements[i].BoundingBox.X+elements[i].BoundingBox.Width,
					elements[i].BoundingBox.Y+elements[i].BoundingBox.Height) {
				// j is parent of i
				elements[i].Parent = elements[j].ID
				elements[j].Children = append(elements[j].Children, elements[i].ID)
			}
		}
	}
}

// ConvertFromDetected converts DetectedElement to UIElement.
func ConvertFromDetected(detected DetectedElement, index int) UIElement {
	return UIElement{
		ID:         fmt.Sprintf("element-%d", index),
		Type:       ParseUIElementType(detected.Class),
		Label:      detected.Label,
		BoundingBox: detected.BoundingBox,
		Confidence:  detected.Confidence,
		Properties:  detected.Attributes,
	}
}

// ParseUIElementType parses a string into UIElementType.
func ParseUIElementType(s string) UIElementType {
	switch s {
	case "button":
		return Button
	case "text_field", "textfield", "input":
		return TextField
	case "label", "text":
		return Label
	case "icon":
		return Icon
	case "menu", "menubar":
		return Menu
	case "checkbox":
		return Checkbox
	case "radio_button", "radio":
		return RadioButton
	case "slider":
		return Slider
	case "dropdown", "select":
		return Dropdown
	case "toggle", "switch":
		return Toggle
	case "tab":
		return Tab
	case "navbar", "navigation":
		return NavigationBar
	case "sidebar":
		return SideBar
	case "toolbar":
		return ToolBar
	case "statusbar", "status_bar":
		return StatusBar
	case "progressbar", "progress_bar":
		return ProgressBar
	case "spinner", "loading":
		return Spinner
	case "dialog", "modal":
		return Dialog
	case "window":
		return Window
	case "panel":
		return Panel
	case "card":
		return Card
	case "list":
		return List
	case "table":
		return Table
	case "tree":
		return Tree
	case "chart":
		return Chart
	case "image":
		return ImageElement
	case "video":
		return Video
	case "link", "anchor":
		return Link
	case "header":
		return Header
	case "footer":
		return Footer
	case "container", "div":
		return Container
	case "separator", "hr":
		return Separator
	default:
		return UnknownElement
	}
}

// GetElementTypeFromConfidence returns the most likely element type based on confidence.
func GetElementTypeFromConfidence(detected DetectedElement) UIElementType {
	if detected.Confidence > 0.8 {
		return ParseUIElementType(detected.Class)
	}
	return UnknownElement
}

// CalculateDensity calculates the density of elements in a region.
func CalculateDensity(elements []UIElement, bounds Box) float64 {
	if bounds.Area() == 0 {
		return 0
	}

	count := 0
	for _, e := range elements {
		// Check if element center is within bounds
		cx := e.BoundingBox.X + e.BoundingBox.Width/2
		cy := e.BoundingBox.Y + e.BoundingBox.Height/2

		if cx >= bounds.X && cx <= bounds.X+bounds.Width &&
			cy >= bounds.Y && cy <= bounds.Y+bounds.Height {
			count++
		}
	}

	return float64(count) / float64(bounds.Area()) * 10000 // per 10000 pixels
}

// FindOverlapping finds elements that overlap with the given element.
func FindOverlapping(element *UIElement, elements []UIElement, threshold float64) []UIElement {
	var overlapping []UIElement

	for _, e := range elements {
		if e.ID == element.ID {
			continue
		}

		iou := element.BoundingBox.IoU(e.BoundingBox)
		if iou > threshold {
			overlapping = append(overlapping, e)
		}
	}

	return overlapping
}

// ClusterByType groups elements by their type.
func ClusterByType(elements []UIElement) map[UIElementType][]UIElement {
	clusters := make(map[UIElementType][]UIElement)

	for _, e := range elements {
		clusters[e.Type] = append(clusters[e.Type], e)
	}

	return clusters
}

// SortByPosition sorts elements by their position (top-to-bottom, left-to-right).
func SortByPosition(elements []UIElement) []UIElement {
	sorted := make([]UIElement, len(elements))
	copy(sorted, elements)

	// Simple bubble sort by row, then column
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			cy1 := sorted[i].BoundingBox.Y + sorted[i].BoundingBox.Height/2
			cy2 := sorted[j].BoundingBox.Y + sorted[j].BoundingBox.Height/2
			cx1 := sorted[i].BoundingBox.X + sorted[i].BoundingBox.Width/2
			cx2 := sorted[j].BoundingBox.X + sorted[j].BoundingBox.Width/2

			if cy1 > cy2 || (cy1 == cy2 && cx1 > cx2) {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	return sorted
}

// ValidateSpatialConsistency checks if the detected layout is spatially consistent.
func ValidateSpatialConsistency(elements []UIElement) []string {
	issues := []string{}

	// Check for overlapping elements
	for i := range elements {
		overlaps := FindOverlapping(&elements[i], elements[i+1:], 0.5)
		if len(overlaps) > 0 {
			issues = append(issues, fmt.Sprintf("Element %s overlaps with %d other elements",
				elements[i].ID, len(overlaps)))
		}
	}

	// Check for elements outside reasonable bounds
	for _, e := range elements {
		if e.BoundingBox.X < 0 || e.BoundingBox.Y < 0 {
			issues = append(issues, fmt.Sprintf("Element %s has negative coordinates", e.ID))
		}
	}

	// Check for extremely small or large elements
	for _, e := range elements {
		area := e.BoundingBox.Area()
		if area < 100 {
			issues = append(issues, fmt.Sprintf("Element %s is too small (area: %d)", e.ID, area))
		}
		if area > 10000000 {
			issues = append(issues, fmt.Sprintf("Element %s is too large (area: %d)", e.ID, area))
		}
	}

	return issues
}

// GetLayoutMetrics calculates metrics about the layout.
func GetLayoutMetrics(elements []UIElement) map[string]interface{} {
	if len(elements) == 0 {
		return nil
	}

	totalArea := 0
	minX, minY := elements[0].BoundingBox.X, elements[0].BoundingBox.Y
	maxX, maxY := minX, minY

	for _, e := range elements {
		area := e.BoundingBox.Area()
		totalArea += area

		if e.BoundingBox.X < minX {
			minX = e.BoundingBox.X
		}
		if e.BoundingBox.Y < minY {
			minY = e.BoundingBox.Y
		}
		if e.BoundingBox.X+e.BoundingBox.Width > maxX {
			maxX = e.BoundingBox.X + e.BoundingBox.Width
		}
		if e.BoundingBox.Y+e.BoundingBox.Height > maxY {
			maxY = e.BoundingBox.Y + e.BoundingBox.Height
		}
	}

	canvasWidth := maxX - minX
	canvasHeight := maxY - minY

	avgWidth := 0
	avgHeight := 0
	for _, e := range elements {
		avgWidth += e.BoundingBox.Width
		avgHeight += e.BoundingBox.Height
	}
	avgWidth /= len(elements)
	avgHeight /= len(elements)

	return map[string]interface{}{
		"element_count":       len(elements),
		"total_area":          totalArea,
		"avg_area":            totalArea / len(elements),
		"avg_width":           avgWidth,
		"avg_height":          avgHeight,
		"canvas_width":        canvasWidth,
		"canvas_height":       canvasHeight,
		"canvas_area":         canvasWidth * canvasHeight,
		"coverage_ratio":      float64(totalArea) / float64(canvasWidth*canvasHeight),
		"density":             CalculateDensity(elements, Box{X: minX, Y: minY, Width: canvasWidth, Height: canvasHeight}),
		"layout_pattern":      DetectLayoutPattern(elements),
	}
}

// ConvertToGrid converts element positions to grid coordinates.
func ConvertToGrid(elements []UIElement, rows, cols int, imageWidth, imageHeight int) map[string][2]int {
	gridCoords := make(map[string][2]int)

	cellWidth := imageWidth / cols
	cellHeight := imageHeight / rows

	for _, e := range elements {
		cx := e.BoundingBox.X + e.BoundingBox.Width/2
		cy := e.BoundingBox.Y + e.BoundingBox.Height/2

		col := cx / cellWidth
		row := cy / cellHeight

		// Clamp to valid range
		if col >= cols {
			col = cols - 1
		}
		if row >= rows {
			row = rows - 1
		}

		gridCoords[e.ID] = [2]int{row, col}
	}

	return gridCoords
}
