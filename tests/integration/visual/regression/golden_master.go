// Package regression provides visual regression testing capabilities.
package regression

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/artemis-project/artemis/internal/vision"
)

// GoldenMaster manages golden master screenshots for visual regression testing.
type GoldenMaster struct {
	basePath       string
	version        string
	platform       string
	updateMode     bool
	allowUpdate    bool
	diffThreshold  float64
	ignoreRegions  []vision.Box

	mu             sync.RWMutex
	capturedImages map[string][]byte
}

// NewGoldenMaster creates a new golden master manager.
func NewGoldenMaster(basePath string) *GoldenMaster {
	return &GoldenMaster{
		basePath:       basePath,
		version:        "current",
		platform:       "any",
		updateMode:     false,
		allowUpdate:    false,
		diffThreshold:  0.05, // 5% difference threshold
		ignoreRegions:  nil,
		capturedImages: make(map[string][]byte),
	}
}

// SetVersion sets the version for golden master files.
func (gm *GoldenMaster) SetVersion(version string) *GoldenMaster {
	gm.version = version
	return gm
}

// SetPlatform sets the platform for golden master files.
func (gm *GoldenMaster) SetPlatform(platform string) *GoldenMaster {
	gm.platform = platform
	return gm
}

// SetUpdateMode enables/disables update mode.
func (gm *GoldenMaster) SetUpdateMode(update bool) *GoldenMaster {
	gm.updateMode = update
	return gm
}

// SetAllowUpdate allows updates via environment variable.
func (gm *GoldenMaster) SetAllowUpdate(allow bool) *GoldenMaster {
	gm.allowUpdate = allow
	return gm
}

// SetDiffThreshold sets the difference threshold for failures.
func (gm *GoldenMaster) SetDiffThreshold(threshold float64) *GoldenMaster {
	gm.diffThreshold = threshold
	return gm
}

// SetIgnoreRegions sets regions to ignore during comparison.
func (gm *GoldenMaster) SetIgnoreRegions(regions []vision.Box) *GoldenMaster {
	gm.ignoreRegions = regions
	return gm
}

// GetPath returns the path for a golden master file.
func (gm *GoldenMaster) GetPath(name string) string {
	var parts []string

	if gm.platform != "any" {
		parts = append(parts, gm.platform)
	}

	if gm.version != "current" {
		parts = append(parts, gm.version)
	}

	dir := filepath.Join(append([]string{gm.basePath}, parts...)...)
	return filepath.Join(dir, name+".png")
}

// Capture captures a screenshot and saves it as golden master.
func (gm *GoldenMaster) Capture(name string, img image.Image) error {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	path := gm.GetPath(name)

	// Create directory if needed
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Save image
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer f.Close()

	if err := png.Encode(f, img); err != nil {
		return fmt.Errorf("failed to encode image: %w", err)
	}

	// Store in memory
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	gm.capturedImages[name] = buf.Bytes()

	return nil
}

// Load loads a golden master image.
func (gm *GoldenMaster) Load(name string) (image.Image, error) {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	path := gm.GetPath(name)

	// Check memory cache first
	if data, ok := gm.capturedImages[name]; ok {
		img, err := png.Decode(bytes.NewReader(data))
		if err == nil {
			return img, nil
		}
	}

	// Load from disk
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open golden master: %w", err)
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	return img, nil
}

// Exists checks if a golden master exists.
func (gm *GoldenMaster) Exists(name string) bool {
	path := gm.GetPath(name)
	_, err := os.Stat(path)
	return err == nil
}

// Compare compares an image with the golden master.
func (gm *GoldenMaster) Compare(name string, current image.Image) (*ComparisonResult, error) {
	gm.mu.Lock()
	defer gm.mu.Unlock()

	// Check if update mode is enabled
	if gm.updateMode || (gm.allowUpdate && os.Getenv("UPDATE_GOLDEN") == "true") {
		if err := gm.Capture(name, current); err != nil {
			return nil, fmt.Errorf("failed to update golden master: %w", err)
		}
		return &ComparisonResult{
			Name:       name,
			Passed:     true,
			DiffScore:  0.0,
			SSIM:       1.0,
			Message:    "Golden master updated",
			Updated:    true,
		}, nil
	}

	// Load golden master
	golden, err := gm.Load(name)
	if err != nil {
		return nil, fmt.Errorf("failed to load golden master: %w", err)
	}

	// Calculate SSIM
	ssimCalc := NewSSIMCalculator()
	ssim := ssimCalc.Calculate(golden, current)

	// Generate diff if needed
	var diffPath string
	if ssim < (1.0 - gm.diffThreshold) {
		diffPath = gm.getDiffPath(name)
		if err := gm.generateDiff(golden, current, diffPath); err != nil {
			return nil, fmt.Errorf("failed to generate diff: %w", err)
		}
	}

	// Determine result
	passed := ssim >= (1.0 - gm.diffThreshold)
	result := &ComparisonResult{
		Name:           name,
		GoldenPath:     gm.GetPath(name),
		CurrentPath:    "",
		DiffPath:       diffPath,
		Passed:         passed,
		DiffScore:      1.0 - ssim,
		SSIM:           ssim,
		Message:        gm.generateMessage(ssim, passed),
		Threshold:      gm.diffThreshold,
	}

	return result, nil
}

// getDiffPath returns the path for a diff image.
func (gm *GoldenMaster) getDiffPath(name string) string {
	return filepath.Join(gm.basePath, "diffs", gm.platform, gm.version, name+".png")
}

// generateDiff generates a diff image between two images.
func (gm *GoldenMaster) generateDiff(golden, current image.Image, diffPath string) error {
	// Create diff image
	bounds := golden.Bounds()
	diff := image.NewRGBA(bounds)

	// Mark differences in red
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c1 := golden.At(x, y)
			c2 := current.At(x, y)

			// Check if pixel is in ignore region
			ignored := false
			for _, region := range gm.ignoreRegions {
				if region.Contains(int(x), int(y)) {
					ignored = true
					break
				}
			}

			if ignored {
				// Use original color
				diff.Set(x, y, c1)
			} else if !colorsEqual(c1, c2, 10) {
				// Different pixel - mark as red
				diff.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
			} else {
				// Same pixel - use grayscale
				gray := colorGray(c1)
				diff.Set(x, y, gray)
			}
		}
	}

	// Save diff image
	if err := os.MkdirAll(filepath.Dir(diffPath), 0755); err != nil {
		return err
	}

	f, err := os.Create(diffPath)
	if err != nil {
		return err
	}
	defer f.Close()

	return png.Encode(f, diff)
}

// generateMessage generates a human-readable message.
func (gm *GoldenMaster) generateMessage(ssim float64, passed bool) string {
	if passed {
		return fmt.Sprintf("Image matches golden master (SSIM: %.4f)", ssim)
	}
	return fmt.Sprintf("Image differs from golden master (SSIM: %.4f, threshold: %.4f)",
		ssim, 1.0-gm.diffThreshold)
}

// colorsEqual checks if two colors are equal within tolerance.
func colorsEqual(c1, c2 color.Color, tolerance int) bool {
	r1, g1, b1, a1 := c1.RGBA()
	r2, g2, b2, a2 := c2.RGBA()

	return abs(int(r1)-int(r2)) <= tolerance &&
		abs(int(g1)-int(g2)) <= tolerance &&
		abs(int(b1)-int(b2)) <= tolerance &&
		abs(int(a1)-int(a2)) <= tolerance
}

// colorGray converts a color to grayscale.
func colorGray(c color.Color) color.Color {
	r, g, b, a := c.RGBA()
	gray := (uint32(r)*299 + uint32(g)*587 + uint32(b)*114) / 1000
	return color.RGBA{
		R: uint8(gray >> 8),
		G: uint8(gray >> 8),
		B: uint8(gray >> 8),
		A: uint8(a >> 8),
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// ComparisonResult represents the result of comparing two images.
type ComparisonResult struct {
	Name        string    `json:"name"`
	GoldenPath  string    `json:"golden_path"`
	CurrentPath string    `json:"current_path"`
	DiffPath    string    `json:"diff_path"`
	Passed      bool      `json:"passed"`
	DiffScore   float64   `json:"diff_score"`   // 0-1, where 0 is identical
	SSIM        float64   `json:"ssim"`         // Structural Similarity Index
	Message     string    `json:"message"`
	Threshold   float64   `json:"threshold"`
	Updated     bool      `json:"updated"`
	Timestamp   time.Time `json:"timestamp"`
}

// String returns a string representation of the result.
func (cr *ComparisonResult) String() string {
	return cr.Message
}

// GoldenMasterTest represents a golden master test case.
type GoldenMasterTest struct {
	Name          string
	CaptureFunc   func() (image.Image, error)
	SetupFunc     func() error
	TeardownFunc  func() error
	Timeout       time.Duration
	Skip          bool
	SkipReason    string
}

// Run executes a golden master test.
func (gm *GoldenMaster) Run(test *GoldenMasterTest) (*ComparisonResult, error) {
	if test.Skip {
		return &ComparisonResult{
			Name:    test.Name,
			Passed:  true,
			Message: fmt.Sprintf("SKIPPED: %s", test.SkipReason),
		}, nil
	}

	// Setup
	if test.SetupFunc != nil {
		if err := test.SetupFunc(); err != nil {
			return nil, fmt.Errorf("setup failed: %w", err)
		}
	}

	// Teardown
	if test.TeardownFunc != nil {
		defer func() {
			_ = test.TeardownFunc()
		}()
	}

	// Capture image
	var img image.Image
	var err error

	if test.Timeout > 0 {
		done := make(chan struct{})
		go func() {
			img, err = test.CaptureFunc()
			close(done)
		}()

		select {
		case <-done:
			// Capture completed
		case <-time.After(test.Timeout):
			return nil, fmt.Errorf("capture timeout after %v", test.Timeout)
		}
	} else {
		img, err = test.CaptureFunc()
	}

	if err != nil {
		return nil, fmt.Errorf("capture failed: %w", err)
	}

	// Compare with golden master
	result, err := gm.Compare(test.Name, img)
	if err != nil {
		return nil, fmt.Errorf("comparison failed: %w", err)
	}

	result.Timestamp = time.Now()
	return result, nil
}

// RunBatch runs multiple golden master tests.
func (gm *GoldenMaster) RunBatch(tests []*GoldenMasterTest) ([]*ComparisonResult, error) {
	results := make([]*ComparisonResult, len(tests))

	var wg sync.WaitGroup
	errs := make([]error, len(tests))

	for i := range tests {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			result, err := gm.Run(tests[idx])
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

// List returns a list of all golden master files.
func (gm *GoldenMaster) List() ([]string, error) {
	var files []string

	err := filepath.Walk(gm.basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if !strings.HasSuffix(path, ".png") {
			return nil
		}

		// Skip diff files
		if strings.Contains(path, "diffs") {
			return nil
		}

		relPath, err := filepath.Rel(gm.basePath, path)
		if err != nil {
			return err
		}

		// Remove .png extension
		name := strings.TrimSuffix(relPath, ".png")
		files = append(files, name)

		return nil
	})

	return files, err
}

// Remove removes a golden master file.
func (gm *GoldenMaster) Remove(name string) error {
	path := gm.GetPath(name)
	return os.Remove(path)
}

// RemoveAll removes all golden master files.
func (gm *GoldenMaster) RemoveAll() error {
	files, err := gm.List()
	if err != nil {
		return err
	}

	for _, file := range files {
		if err := gm.Remove(file); err != nil {
			return err
		}
	}

	return nil
}

// Validate checks if all golden masters are valid.
func (gm *GoldenMaster) Validate() error {
	files, err := gm.List()
	if err != nil {
		return err
	}

	for _, file := range files {
		_, err := gm.Load(file)
		if err != nil {
			return fmt.Errorf("invalid golden master %s: %w", file, err)
		}
	}

	return nil
}

// GetMetadata returns metadata about golden masters.
func (gm *GoldenMaster) GetMetadata() (map[string]interface{}, error) {
	files, err := gm.List()
	if err != nil {
		return nil, err
	}

	totalSize := int64(0)
	sizes := make(map[string]int64)
	versions := make(map[string]int)
	platforms := make(map[string]int)

	for _, file := range files {
		path := gm.GetPath(file)
		info, err := os.Stat(path)
		if err != nil {
			continue
		}

		totalSize += info.Size()

		// Extract version and platform from path
		parts := strings.Split(file, string(filepath.Separator))
		if len(parts) >= 2 {
			platforms[parts[0]]++
			if len(parts) >= 3 {
				versions[parts[1]]++
			}
		}
	}

	return map[string]interface{}{
		"count":      len(files),
		"total_size": totalSize,
		"sizes":      sizes,
		"versions":   versions,
		"platforms":  platforms,
		"version":    gm.version,
		"platform":   gm.platform,
	}, nil
}

// CreateBlank creates a blank golden master (for testing).
func (gm *GoldenMaster) CreateBlank(name string, width, height int) error {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	return gm.Capture(name, img)
}

// CreatePattern creates a patterned golden master (for testing).
func (gm *GoldenMaster) CreatePattern(name string, width, height int) error {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Create checkerboard pattern
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if ((x/50)+(y/50))%2 == 0 {
				img.Set(x, y, color.White)
			} else {
				img.Set(x, y, color.Black)
			}
		}
	}

	return gm.Capture(name, img)
}

// CaptureMultiple captures multiple screenshots for a test.
func (gm *GoldenMaster) CaptureMultiple(prefix string, images []image.Image) error {
	for i, img := range images {
		name := fmt.Sprintf("%s_%03d", prefix, i)
		if err := gm.Capture(name, img); err != nil {
			return fmt.Errorf("failed to capture %s: %w", name, err)
		}
	}
	return nil
}

// Export exports golden masters to a tarball.
func (gm *GoldenMaster) Export(outputPath string) error {
	// TODO: Implement tarball export
	return fmt.Errorf("not yet implemented")
}

// Import imports golden masters from a tarball.
func (gm *GoldenMaster) Import(inputPath string) error {
	// TODO: Implement tarball import
	return fmt.Errorf("not yet implemented")
}

// Clone creates a copy of a golden master.
func (gm *GoldenMaster) Clone(source, target string) error {
	img, err := gm.Load(source)
	if err != nil {
		return err
	}
	return gm.Capture(target, img)
}

// Merge merges golden masters from another path.
func (gm *GoldenMaster) Merge(otherPath string) error {
	other := NewGoldenMaster(otherPath)
	files, err := other.List()
	if err != nil {
		return err
	}

	for _, file := range files {
		img, err := other.Load(file)
		if err != nil {
			continue
		}

		if !gm.Exists(file) {
			if err := gm.Capture(file, img); err != nil {
				return err
			}
		}
	}

	return nil
}

// Backup creates a backup of golden masters.
func (gm *GoldenMaster) Backup(backupPath string) error {
	return gm.Merge(backupPath)
}

// Restore restores golden masters from a backup.
func (gm *GoldenMaster) Restore(backupPath string) error {
	other := NewGoldenMaster(backupPath)
	return other.Merge(gm.basePath)
}
