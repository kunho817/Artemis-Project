// Package vision provides computer vision capabilities for UI element detection.
package vision

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Detector performs object detection using YOLOv9.
type Detector struct {
	modelPath  string
	threshold  float64
	modelSize  string // "n", "s", "m", "c", or "e"
	device     string // "cpu" or "cuda"
	useONNX    bool
	usePython  bool
	pythonPath string

	mu          sync.Mutex
	lastInference time.Time
	cache       map[string][]DetectedElement
	cacheTTL    time.Duration
}

// DetectedElement represents a detected UI element.
type DetectedElement struct {
	Class      string  `json:"class"`
	Label      string  `json:"label,omitempty"`
	Confidence float64 `json:"confidence"`
	BoundingBox Box     `json:"bounding_box"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
}

// Box represents a bounding box.
type Box struct {
	X      int     `json:"x"`
	Y      int     `json:"y"`
	Width  int     `json:"width"`
	Height int     `json:"height"`
}

// Center returns the center point of the box.
func (b *Box) Center() (int, int) {
	return b.X + b.Width/2, b.Y + b.Height/2
}

// Area returns the area of the box.
func (b *Box) Area() int {
	return b.Width * b.Height
}

// IoU calculates Intersection over Union with another box.
func (b *Box) IoU(other Box) float64 {
	x1 := math.Max(float64(b.X), float64(other.X))
	y1 := math.Max(float64(b.Y), float64(other.Y))
	x2 := math.Min(float64(b.X+b.Width), float64(other.X+other.Width))
	y2 := math.Min(float64(b.Y+b.Height), float64(other.Y+other.Height))

	intersection := math.Max(0, x2-x1) * math.Max(0, y2-y1)
	union := float64(b.Area()) + float64(other.Area()) - intersection

	if union == 0 {
		return 0
	}

	return intersection / union
}

// Contains checks if a point is inside the box.
func (b *Box) Contains(x, y int) bool {
	return x >= b.X && x <= b.X+b.Width && y >= b.Y && y <= b.Y+b.Height
}

// DetectionResult represents the output of detection.
type DetectionResult struct {
	Elements      []DetectedElement `json:"elements"`
	InferenceTime float64            `json:"inference_time_ms"`
	ImageSize     image.Point        `json:"image_size"`
	Timestamp     time.Time          `json:"timestamp"`
}

// NewDetector creates a new YOLOv9 detector.
func NewDetector(modelPath string) (*Detector, error) {
	d := &Detector{
		modelPath:   modelPath,
		threshold:   0.5,
		modelSize:   "n", // nano model (fastest)
		device:      "cpu",
		useONNX:     false,
		usePython:   true, // Default to Python subprocess
		cache:       make(map[string][]DetectedElement),
		cacheTTL:    5 * time.Minute,
	}

	// Check if Python is available
	if _, err := exec.LookPath("python3"); err == nil {
		d.pythonPath = "python3"
	} else if _, err := exec.LookPath("python"); err == nil {
		d.pythonPath = "python"
	} else {
		return nil, fmt.Errorf("python not found in PATH")
	}

	return d, nil
}

// SetThreshold sets the confidence threshold.
func (d *Detector) SetThreshold(threshold float64) *Detector {
	d.threshold = math.Max(0, math.Min(1, threshold))
	return d
}

// SetModelSize sets the YOLOv9 model size.
func (d *Detector) SetModelSize(size string) *Detector {
	validSizes := map[string]bool{"n": true, "s": true, "m": true, "c": true, "e": true}
	if validSizes[size] {
		d.modelSize = size
	}
	return d
}

// SetDevice sets the device (cpu or cuda).
func (d *Detector) SetDevice(device string) *Detector {
	device = strings.ToLower(device)
	if device == "cuda" || device == "cpu" {
		d.device = device
	}
	return d
}

// Detect performs object detection on an image.
func (d *Detector) Detect(img image.Image) (*DetectionResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	start := time.Now()

	// Check cache
	cacheKey := d.generateCacheKey(img)
	if cached, _ := d.cache[cacheKey]; time.Since(d.lastInference) < d.cacheTTL && len(cached) > 0 {
		return &DetectionResult{
			Elements:      cached,
			InferenceTime: float64(time.Since(start).Microseconds()) / 1000.0,
			ImageSize:     img.Bounds().Max,
			Timestamp:     time.Now(),
		}, nil
	}

	var elements []DetectedElement
	var err error

	if d.usePython {
		elements, err = d.detectWithPython(img)
	} else {
		elements, err = d.detectWithONNX(img)
	}

	if err != nil {
		return nil, fmt.Errorf("detection failed: %w", err)
	}

	// Filter by threshold
	var filtered []DetectedElement
	for _, elem := range elements {
		if elem.Confidence >= d.threshold {
			filtered = append(filtered, elem)
		}
	}

	// Update cache
	d.cache[cacheKey] = filtered
	d.lastInference = time.Now()

	return &DetectionResult{
		Elements:      filtered,
		InferenceTime: float64(time.Since(start).Microseconds()) / 1000.0,
		ImageSize:     img.Bounds().Max,
		Timestamp:     time.Now(),
	}, nil
}

// detectWithPython uses Python subprocess for YOLOv9 inference.
func (d *Detector) detectWithPython(img image.Image) ([]DetectedElement, error) {
	// Save image to temporary file
	tmpDir := os.TempDir()
	tmpPath := filepath.Join(tmpDir, fmt.Sprintf("yolo_input_%d.png", time.Now().UnixNano()))

	if err := d.saveImage(img, tmpPath); err != nil {
		return nil, fmt.Errorf("failed to save image: %w", err)
	}
	defer os.Remove(tmpPath)

	// Prepare Python script
	script := fmt.Sprintf(`
import sys
import json
from pathlib import Path

try:
    from ultralytics import YOLO
    model = YOLO('%s')
    results = model('%s', conf=%f, device='%s', verbose=False)

    detections = []
    for r in results:
        for box in r.boxes:
            x1, y1, x2, y2 = box.xyxy[0].tolist()
            detections.append({
                "class": r.names[int(box.cls)],
                "confidence": float(box.conf),
                "bounding_box": {
                    "x": int(x1),
                    "y": int(y1),
                    "width": int(x2 - x1),
                    "height": int(y2 - y1)
                }
            })

    print(json.dumps(detections))
except ImportError:
    print(json.dumps({"error": "ultralytics not installed"}))
    sys.exit(1)
except Exception as e:
    print(json.dumps({"error": str(e)}))
    sys.exit(1)
`, d.modelPath, tmpPath, d.threshold, d.device)

	// Execute Python script
	scriptPath := filepath.Join(tmpDir, fmt.Sprintf("yolo_script_%d.py", time.Now().UnixNano()))
	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		return nil, fmt.Errorf("failed to write script: %w", err)
	}
	defer os.Remove(scriptPath)

	cmd := exec.Command(d.pythonPath, scriptPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("python execution failed: %w, output: %s", err, string(output))
	}

	// Parse JSON output
	var rawElements []struct {
		Class      string  `json:"class"`
		Confidence float64 `json:"confidence"`
		BoundingBox Box    `json:"bounding_box"`
	}

	if err := json.Unmarshal(output, &rawElements); err != nil {
		return nil, fmt.Errorf("failed to parse output: %w, output: %s", err, string(output))
	}

	// Convert to DetectedElement
	var elements []DetectedElement
	for _, re := range rawElements {
		elements = append(elements, DetectedElement{
			Class:      re.Class,
			Confidence: re.Confidence,
			BoundingBox: re.BoundingBox,
		})
	}

	return elements, nil
}

// detectWithONNX uses ONNX Runtime for inference (placeholder).
func (d *Detector) detectWithONNX(img image.Image) ([]DetectedElement, error) {
	// TODO: Implement ONNX Runtime integration
	return nil, fmt.Errorf("ONNX inference not yet implemented")
}

// DetectToFile detects elements and saves annotated image.
func (d *Detector) DetectToFile(img image.Image, outputPath string) error {
	result, err := d.Detect(img)
	if err != nil {
		return err
	}

	// Create annotated image
	annotated := d.createAnnotatedImage(img, result.Elements)

	// Save annotated image
	return d.saveImage(annotated, outputPath)
}

// createAnnotatedImage creates an image with bounding boxes.
func (d *Detector) createAnnotatedImage(img image.Image, elements []DetectedElement) image.Image {
	// Create a new RGBA image
	bounds := img.Bounds()
	rgba := image.NewRGBA(bounds)
	draw.Draw(rgba, bounds, img, bounds.Min, draw.Src)

	// Draw bounding boxes
	for _, elem := range elements {
		box := elem.BoundingBox

		// Draw rectangle
		color := color.NRGBA{R: 0, G: 255, B: 0, A: 255}
		d.drawRect(rgba, box, color)

		// TODO: Draw label
	}

	return rgba
}

// drawRect draws a rectangle on an image.
func (d *Detector) drawRect(img *image.RGBA, box Box, c color.Color) {
	// Top edge
	for x := box.X; x < box.X+box.Width; x++ {
		img.Set(x, box.Y, c)
	}
	// Bottom edge
	for x := box.X; x < box.X+box.Width; x++ {
		img.Set(x, box.Y+box.Height, c)
	}
	// Left edge
	for y := box.Y; y < box.Y+box.Height; y++ {
		img.Set(box.X, y, c)
	}
	// Right edge
	for y := box.Y; y < box.Y+box.Height; y++ {
		img.Set(box.X+box.Width, y, c)
	}
}

// saveImage saves an image to a file.
func (d *Detector) saveImage(img image.Image, path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return png.Encode(f, img)
}

// generateCacheKey generates a cache key for an image.
func (d *Detector) generateCacheKey(img image.Image) string {
	bounds := img.Bounds()
	return fmt.Sprintf("%dx%d", bounds.Dx(), bounds.Dy())
}

// GetSupportedClasses returns the list of classes the detector can recognize.
func (d *Detector) GetSupportedClasses() []string {
	// UI element classes
	return []string{
		"button",
		"text_field",
		"label",
		"icon",
		"menu",
		"checkbox",
		"radio_button",
		"slider",
		"dropdown",
		"toggle",
		"tab",
		"navbar",
		"sidebar",
		"toolbar",
		"status_bar",
		"progress_bar",
		"spinner",
		"dialog",
		"window",
		"panel",
		"card",
		"list",
		"table",
		"tree",
		"chart",
		"image",
		"video",
	}
}

// GetModelInfo returns information about the detector.
func (d *Detector) GetModelInfo() map[string]interface{} {
	return map[string]interface{}{
		"model_path":  d.modelPath,
		"model_size":  d.modelSize,
		"device":      d.device,
		"threshold":   d.threshold,
		"use_onnx":    d.useONNX,
		"use_python":  d.usePython,
		"python_path": d.pythonPath,
		"os":          runtime.GOOS,
		"arch":        runtime.GOARCH,
	}
}

// ClearCache clears the detection cache.
func (d *Detector) ClearCache() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cache = make(map[string][]DetectedElement)
}

// GetCacheStats returns cache statistics.
func (d *Detector) GetCacheStats() map[string]interface{} {
	d.mu.Lock()
	defer d.mu.Unlock()

	return map[string]interface{}{
		"cache_size":   len(d.cache),
		"cache_ttl":    d.cacheTTL.String(),
		"last_inference": d.lastInference,
	}
}

// DetectBatch detects elements in multiple images.
func (d *Detector) DetectBatch(images []image.Image) ([]*DetectionResult, error) {
	results := make([]*DetectionResult, len(images))

	for i, img := range images {
		result, err := d.Detect(img)
		if err != nil {
			return nil, fmt.Errorf("detection failed for image %d: %w", i, err)
		}
		results[i] = result
	}

	return results, nil
}

// FilterByClass filters elements by class name.
func (d *Detector) FilterByClass(elements []DetectedElement, classes ...string) []DetectedElement {
	classSet := make(map[string]bool)
	for _, c := range classes {
		classSet[c] = true
	}

	var filtered []DetectedElement
	for _, elem := range elements {
		if classSet[elem.Class] {
			filtered = append(filtered, elem)
		}
	}

	return filtered
}

// MergeOverlapping merges overlapping bounding boxes.
func (d *Detector) MergeOverlapping(elements []DetectedElement, iouThreshold float64) []DetectedElement {
	if len(elements) == 0 {
		return elements
	}

	// Sort by confidence (descending)
	sorted := make([]DetectedElement, len(elements))
	copy(sorted, elements)

	// Simple bubble sort
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i].Confidence < sorted[j].Confidence {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	var merged []DetectedElement
	used := make(map[int]bool)

	for i := range sorted {
		if used[i] {
			continue
		}

		merged = append(merged, sorted[i])
		used[i] = true

		for j := i + 1; j < len(sorted); j++ {
			if used[j] {
				continue
			}

			if sorted[i].BoundingBox.IoU(sorted[j].BoundingBox) > iouThreshold {
				used[j] = true
			}
		}
	}

	return merged
}

// ParseOutput parses YOLOv9 output format.
func (d *Detector) ParseOutput(output string) ([]DetectedElement, error) {
	var elements []DetectedElement

	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) < 6 {
			continue
		}

		classIdx, _ := strconv.Atoi(parts[0])
		x, _ := strconv.ParseFloat(parts[1], 64)
		y, _ := strconv.ParseFloat(parts[2], 64)
		w, _ := strconv.ParseFloat(parts[3], 64)
		h, _ := strconv.ParseFloat(parts[4], 64)
		conf, _ := strconv.ParseFloat(parts[5], 64)

		classes := d.GetSupportedClasses()
		if classIdx >= 0 && classIdx < len(classes) {
			elements = append(elements, DetectedElement{
				Class:      classes[classIdx],
				Confidence: conf,
				BoundingBox: Box{
					X:      int(x),
					Y:      int(y),
					Width:  int(w),
					Height: int(h),
				},
			})
		}
	}

	return elements, nil
}
