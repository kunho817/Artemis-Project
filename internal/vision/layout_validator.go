// Package vision provides layout validation combining YOLOv9 and VLM.
package vision

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"math"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/artemis-project/artemis/tests/integration/visual"
)

// LayoutValidator validates UI layouts using YOLOv9 and VLM.
type LayoutValidator struct {
	detector       *Detector
	vlmProvider    visual.VisionProvider
	strictMode     bool
	spatialTolerance int // in pixels
	ensembleMode   bool // Use both YOLO and VLM

	mu sync.RWMutex
}

// ValidationResult represents the result of layout validation.
type ValidationResult struct {
	Timestamp          time.Time               `json:"timestamp"`
	ImageSize          image.Point             `json:"image_size"`
	YOLOElements       []DetectedElement       `json:"yolo_elements"`
	VLMDescription     string                  `json:"vlm_description"`
	VLMParsedElements  []UIElement             `json:"vlm_parsed_elements"`
	SpatialMatches     float64                 `json:"spatial_matches"`     // 0-1
	Hallucinations     []Hallucination          `json:"hallucinations"`
	MissingElements    []MissingElement        `json:"missing_elements"`
	OverallVerdict     string                  `json:"overall_verdict"`      // "PASS", "FAIL", "PARTIAL"
	Confidence         float64                 `json:"confidence"`
	Details            map[string]interface{}  `json:"details"`
}

// Hallucination represents a detected hallucination.
type Hallucination struct {
	Type        string  `json:"type"`         // "element", "position", "relationship", "direction"
	Description string  `json:"description"`
	Confidence  float64 `json:"confidence"`
	Source      string  `json:"source"`       // "vlm"
	Expected    string  `json:"expected,omitempty"`
	Actual      string  `json:"actual,omitempty"`
}

// MissingElement represents an element detected by YOLO but not mentioned by VLM.
type MissingElement struct {
	Element     DetectedElement `json:"element"`
	Reason      string          `json:"reason"`
	Confidence  float64         `json:"confidence"`
}

// NewLayoutValidator creates a new layout validator.
func NewLayoutValidator(detector *Detector, vlmProvider visual.VisionProvider) *LayoutValidator {
	return &LayoutValidator{
		detector:         detector,
		vlmProvider:      vlmProvider,
		strictMode:       false,
		spatialTolerance: 50, // 50 pixels tolerance
		ensembleMode:     true,
	}
}

// SetStrictMode enables strict spatial validation.
func (lv *LayoutValidator) SetStrictMode(strict bool) *LayoutValidator {
	lv.strictMode = strict
	if strict {
		lv.spatialTolerance = 10 // 10 pixels in strict mode
	} else {
		lv.spatialTolerance = 50 // 50 pixels in lenient mode
	}
	return lv
}

// ValidateLayout validates a layout using both YOLO and VLM.
func (lv *LayoutValidator) ValidateLayout(ctx context.Context, img interface{}, vlmPrompt string) (*ValidationResult, error) {
	start := time.Now()

	lv.mu.Lock()
	defer lv.mu.Unlock()

	// Step 1: YOLO detection
	var yoloElements []DetectedElement
	if lv.detector != nil {
		yoloResult, err := lv.detector.Detect(img.(image.Image))
		if err == nil {
			yoloElements = yoloResult.Elements
		}
	}

	// Step 2: VLM analysis
	var vlmDescription string
	if lv.vlmProvider != nil {
		response, err := lv.vlmProvider.AnalyzeImage(ctx, img, vlmPrompt)
		if err == nil {
			vlmDescription = response
		}
	}

	// Step 3: Parse VLM response
	vlmElements := lv.parseVLMResponse(vlmDescription)

	// Step 4: Calculate spatial matches
	spatialMatches := lv.calculateSpatialMatch(yoloElements, vlmElements)

	// Step 5: Detect hallucinations
	hallucinations := lv.detectHallucinations(yoloElements, vlmElements)

	// Step 6: Find missing elements
	missingElements := lv.findMissingElements(yoloElements, vlmElements)

	// Step 7: Determine overall verdict
	verdict, confidence := lv.determineVerdict(spatialMatches, hallucinations, missingElements)

	// Step 8: Calculate image size
	var imgSize image.Point
	if imgImg, ok := img.(image.Image); ok {
		imgSize = imgImg.Bounds().Max
	}

	result := &ValidationResult{
		Timestamp:          time.Now(),
		ImageSize:          imgSize,
		YOLOElements:       yoloElements,
		VLMDescription:     vlmDescription,
		VLMParsedElements:  vlmElements,
		SpatialMatches:     spatialMatches,
		Hallucinations:     hallucinations,
		MissingElements:    missingElements,
		OverallVerdict:     verdict,
		Confidence:         confidence,
		Details: map[string]interface{}{
			"inference_time_ms": float64(time.Since(start).Microseconds()) / 1000.0,
			"yolo_count":        len(yoloElements),
			"vlm_count":         len(vlmElements),
			"strict_mode":       lv.strictMode,
			"spatial_tolerance": lv.spatialTolerance,
		},
	}

	return result, nil
}

// parseVLMResponse parses VLM response to extract UI elements.
func (lv *LayoutValidator) parseVLMResponse(response string) []UIElement {
	var elements []UIElement

	// Pattern to match element descriptions like:
	// "Element: button at (100, 200)"
	// "Button at R2,C3"
	// "Element A: [button] Position: (row, col)"
	patterns := []struct {
		regex *regexp.Regexp
		extract func(matches []string) UIElement
	}{
		// Pattern 1: "Element: button at (100, 200)"
		{
			regex: regexp.MustCompile(`Element:\s*(\w+)\s+at\s+\((\d+),\s*(\d+)\)`),
			extract: func(matches []string) UIElement {
				if len(matches) < 4 {
					return UIElement{}
				}
				x, y := 0, 0
				fmt.Sscanf(matches[2], "%d", &x)
				fmt.Sscanf(matches[3], "%d", &y)
				return UIElement{
					ID:   fmt.Sprintf("vlm-%d", len(elements)),
					Type: ParseUIElementType(matches[1]),
					BoundingBox: Box{
						X:      x,
						Y:      y,
						Width:  50,  // Default size
						Height: 20,
					},
					Confidence: 0.8,
				}
			},
		},
		// Pattern 2: "button at R2,C3" (grid coordinates)
		{
			regex: regexp.MustCompile(`(\w+)\s+at\s+R(\d+),C(\d+)`),
			extract: func(matches []string) UIElement {
				if len(matches) < 4 {
					return UIElement{}
				}
				row, col := 0, 0
				fmt.Sscanf(matches[2], "%d", &row)
				fmt.Sscanf(matches[3], "%d", &col)
				// Convert grid to pixel coordinates (assume 100x100 cells)
				return UIElement{
					ID:   fmt.Sprintf("vlm-%d", len(elements)),
					Type: ParseUIElementType(matches[1]),
					BoundingBox: Box{
						X:      col * 100,
						Y:      row * 100,
						Width:  100,
						Height: 100,
					},
					Confidence: 0.7,
				}
			},
		},
	}

	lines := strings.Split(response, "\n")
	for _, line := range lines {
		for _, p := range patterns {
			if matches := p.regex.FindStringSubmatch(line); len(matches) > 0 {
				elem := p.extract(matches)
				if elem.Type != UnknownElement {
					elements = append(elements, elem)
				}
			}
		}
	}

	return elements
}

// calculateSpatialMatch calculates how well YOLO and VLM results match spatially.
func (lv *LayoutValidator) calculateSpatialMatch(yoloElements []DetectedElement, vlmElements []UIElement) float64 {
	if len(yoloElements) == 0 && len(vlmElements) == 0 {
		return 1.0 // Both empty = perfect match
	}

	if len(yoloElements) == 0 || len(vlmElements) == 0 {
		return 0.0 // One empty = no match
	}

	// Match YOLO elements to VLM elements
	matched := 0
	for _, yolo := range yoloElements {
		for _, vlm := range vlmElements {
			if yolo.Class == string(vlm.Type) {
				// Check spatial proximity
				yoloBox := yolo.BoundingBox
				vlmBox := vlm.BoundingBox

				dist := calculateDistance(&yoloBox, &vlmBox)
				if dist <= lv.spatialTolerance {
					matched++
					break
				}

				// Check overlap
				iou := yoloBox.IoU(vlmBox)
				if iou > 0.3 {
					matched++
					break
				}
			}
		}
	}

	return float64(matched) / math.Max(float64(len(yoloElements)), float64(len(vlmElements)))
}

// detectHallucinations detects hallucinations in VLM response.
func (lv *LayoutValidator) detectHallucinations(yoloElements []DetectedElement, vlmElements []UIElement) []Hallucination {
	var hallucinations []Hallucination

	// Create a set of YOLO classes
	yoloClasses := make(map[string]bool)
	for _, y := range yoloElements {
		yoloClasses[y.Class] = true
	}

	// Check for element hallucinations (VLM sees elements that YOLO doesn't)
	for _, v := range vlmElements {
		if !yoloClasses[string(v.Type)] {
			hallucinations = append(hallucinations, Hallucination{
				Type:        "element",
				Description: fmt.Sprintf("VLM detected %s element that YOLO didn't find", v.Type),
				Confidence:  0.7,
				Source:      "vlm",
				Expected:    "element not present or below threshold",
				Actual:      string(v.Type),
			})
		}
	}

	// Check for position hallucinations
	for _, v := range vlmElements {
		closestDist := -1
		var closestYolo *DetectedElement

		for i, y := range yoloElements {
			if y.Class == string(v.Type) {
				dist := calculateDistance(&y.BoundingBox, &v.BoundingBox)
				if closestDist == -1 || dist < closestDist {
					closestDist = dist
					closestYolo = &yoloElements[i]
				}
			}
		}

		if closestYolo != nil && closestDist > lv.spatialTolerance {
			hallucinations = append(hallucinations, Hallucination{
				Type:        "position",
				Description: fmt.Sprintf("VLM reported %s at wrong position (off by %d pixels)",
					v.Type, closestDist),
				Confidence:  minFloat64(0.9, float64(closestDist)/200),
				Source:      "vlm",
				Expected:    fmt.Sprintf("(%d, %d)", closestYolo.BoundingBox.X, closestYolo.BoundingBox.Y),
				Actual:      fmt.Sprintf("(%d, %d)", v.BoundingBox.X, v.BoundingBox.Y),
			})
		}
	}

	// Check for relationship hallucinations
	// This would require more complex parsing of VLM response for relationships
	// TODO: Implement relationship parsing and validation

	return hallucinations
}

// findMissingElements finds elements detected by YOLO but not mentioned by VLM.
func (lv *LayoutValidator) findMissingElements(yoloElements []DetectedElement, vlmElements []UIElement) []MissingElement {
	var missing []MissingElement

	// Create a set of VLM types
	vlmTypes := make(map[string]bool)
	for _, v := range vlmElements {
		vlmTypes[string(v.Type)] = true
	}

	// Find YOLO elements not in VLM
	for _, y := range yoloElements {
		if !vlmTypes[y.Class] {
			missing = append(missing, MissingElement{
				Element:    y,
				Reason:     "VLM did not mention this element",
				Confidence: y.Confidence,
			})
		}
	}

	return missing
}

// determineVerdict determines the overall validation verdict.
func (lv *LayoutValidator) determineVerdict(spatialMatches float64, hallucinations []Hallucination, missing []MissingElement) (string, float64) {
	// Calculate confidence based on multiple factors
	confidence := spatialMatches

	// Penalize hallucinations
	hallucinationPenalty := float64(len(hallucinations)) * 0.1
	confidence -= hallucinationPenalty

	// Penalize missing elements (less severe)
	missingPenalty := float64(len(missing)) * 0.05
	confidence -= missingPenalty

	// Clamp to 0-1
	confidence = math.Max(0, math.Min(1, confidence))

	// Determine verdict based on strict mode
	if lv.strictMode {
		// Strict mode: high threshold
		if confidence >= 0.9 && len(hallucinations) == 0 {
			return "PASS", confidence
		} else if confidence >= 0.6 {
			return "PARTIAL", confidence
		} else {
			return "FAIL", confidence
		}
	} else {
		// Lenient mode: lower threshold
		if confidence >= 0.7 && len(hallucinations) == 0 {
			return "PASS", confidence
		} else if confidence >= 0.4 {
			return "PARTIAL", confidence
		} else {
			return "FAIL", confidence
		}
	}
}

// ValidateLayoutRule validates a specific layout rule.
type LayoutRule struct {
	Name        string
	Description string
	Validate    func(yolo []DetectedElement, vlm []UIElement) (bool, string)
}

// ValidateWithRules validates layout using custom rules.
func (lv *LayoutValidator) ValidateWithRules(ctx context.Context, img interface{}, rules []LayoutRule) (map[string]bool, map[string]string, error) {
	results := make(map[string]bool)
	messages := make(map[string]string)

	// Get YOLO and VLM results
	var yoloElements []DetectedElement
	if lv.detector != nil {
		yoloResult, err := lv.detector.Detect(img.(image.Image))
		if err == nil {
			yoloElements = yoloResult.Elements
		}
	}

	vlmPrompt := "List all UI elements with their positions. Format: Element: [type] at (x, y)"
	var vlmDescription string
	if lv.vlmProvider != nil {
		response, err := lv.vlmProvider.AnalyzeImage(ctx, img, vlmPrompt)
		if err == nil {
			vlmDescription = response
		}
	}
	vlmElements := lv.parseVLMResponse(vlmDescription)

	// Validate each rule
	for _, rule := range rules {
		passed, message := rule.Validate(yoloElements, vlmElements)
		results[rule.Name] = passed
		messages[rule.Name] = message
	}

	return results, messages, nil
}

// GenerateReport generates a human-readable validation report.
func (vr *ValidationResult) GenerateReport() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("=== Layout Validation Report ===\n"))
	sb.WriteString(fmt.Sprintf("Timestamp: %s\n", vr.Timestamp.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("Overall Verdict: %s (Confidence: %.2f%%)\n\n", vr.OverallVerdict, vr.Confidence*100))

	sb.WriteString(fmt.Sprintf("Element Counts:\n"))
	sb.WriteString(fmt.Sprintf("  YOLO detected: %d\n", len(vr.YOLOElements)))
	sb.WriteString(fmt.Sprintf("  VLM mentioned: %d\n", len(vr.VLMParsedElements)))
	sb.WriteString(fmt.Sprintf("  Spatial match: %.2f%%\n\n", vr.SpatialMatches*100))

	if len(vr.Hallucinations) > 0 {
		sb.WriteString(fmt.Sprintf("Hallucinations Detected (%d):\n", len(vr.Hallucinations)))
		for _, h := range vr.Hallucinations {
			sb.WriteString(fmt.Sprintf("  - [%s] %s (%.1f%% confidence)\n", h.Type, h.Description, h.Confidence*100))
		}
		sb.WriteString("\n")
	}

	if len(vr.MissingElements) > 0 {
		sb.WriteString(fmt.Sprintf("Missing Elements (%d):\n", len(vr.MissingElements)))
		for _, m := range vr.MissingElements {
			sb.WriteString(fmt.Sprintf("  - %s: %s (%.1f%% confidence)\n",
				m.Element.Class, m.Reason, m.Confidence*100))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("VLM Description:\n")
	sb.WriteString(vr.VLMDescription)
	sb.WriteString("\n")

	return sb.String()
}

// SaveReport saves the validation report to a file.
func (vr *ValidationResult) SaveReport(path string) error {
	report := vr.GenerateReport()
	return os.WriteFile(path, []byte(report), 0644)
}

// SaveJSON saves the validation result as JSON.
func (vr *ValidationResult) SaveJSON(path string) error {
	data, err := json.MarshalIndent(vr, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// GetStatistics returns statistics about validation results.
func (lv *LayoutValidator) GetStatistics(results []*ValidationResult) map[string]interface{} {
	stats := map[string]interface{}{
		"total_validations": len(results),
		"pass_count":        0,
		"fail_count":        0,
		"partial_count":     0,
		"avg_confidence":    0.0,
		"avg_spatial_match": 0.0,
		"total_hallucinations": 0,
		"total_missing":       0,
	}

	totalConfidence := 0.0
	totalSpatialMatch := 0.0

	for _, r := range results {
		switch r.OverallVerdict {
		case "PASS":
			stats["pass_count"] = stats["pass_count"].(int) + 1
		case "FAIL":
			stats["fail_count"] = stats["fail_count"].(int) + 1
		case "PARTIAL":
			stats["partial_count"] = stats["partial_count"].(int) + 1
		}

		totalConfidence += r.Confidence
		totalSpatialMatch += r.SpatialMatches
		stats["total_hallucinations"] = stats["total_hallucinations"].(int) + len(r.Hallucinations)
		stats["total_missing"] = stats["total_missing"].(int) + len(r.MissingElements)
	}

	if len(results) > 0 {
		stats["avg_confidence"] = totalConfidence / float64(len(results))
		stats["avg_spatial_match"] = totalSpatialMatch / float64(len(results))
	}

	return stats
}

// BatchValidate validates multiple images in batch.
func (lv *LayoutValidator) BatchValidate(ctx context.Context, images []interface{}, prompts []string) ([]*ValidationResult, error) {
	if len(images) != len(prompts) {
		return nil, fmt.Errorf("images and prompts must have same length")
	}

	results := make([]*ValidationResult, len(images))

	var wg sync.WaitGroup
	errs := make([]error, len(images))

	for i := range images {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			result, err := lv.ValidateLayout(ctx, images[idx], prompts[idx])
			if err != nil {
				errs[idx] = err
				return
			}
			results[idx] = result
		}(i)
	}

	wg.Wait()

	// Check for errors
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}

	return results, nil
}

// CompareLayouts compares two validation results.
func (lv *LayoutValidator) CompareLayouts(r1, r2 *ValidationResult) map[string]interface{} {
	return map[string]interface{}{
		"spatial_match_diff":     r2.SpatialMatches - r1.SpatialMatches,
		"confidence_diff":        r2.Confidence - r1.Confidence,
		"hallucination_count_diff": len(r2.Hallucinations) - len(r1.Hallucinations),
		"missing_count_diff":     len(r2.MissingElements) - len(r1.MissingElements),
		"verdict_changed":        r1.OverallVerdict != r2.OverallVerdict,
		"yolo_count_diff":        len(r2.YOLOElements) - len(r1.YOLOElements),
		"vlm_count_diff":         len(r2.VLMParsedElements) - len(r1.VLMParsedElements),
	}
}

// SetSpatialTolerance sets the spatial tolerance for validation.
func (lv *LayoutValidator) SetSpatialTolerance(tolerance int) *LayoutValidator {
	lv.spatialTolerance = tolerance
	return lv
}

// EnableEnsembleMode enables ensemble validation (both YOLO and VLM).
func (lv *LayoutValidator) EnableEnsembleMode(enable bool) *LayoutValidator {
	lv.ensembleMode = enable
	return lv
}

// GetDetector returns the YOLO detector.
func (lv *LayoutValidator) GetDetector() *Detector {
	return lv.detector
}

// GetVLMProvider returns the VLM provider.
func (lv *LayoutValidator) GetVLMProvider() visual.VisionProvider {
	return lv.vlmProvider
}

// minFloat64 returns the minimum of two floats.
func minFloat64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
