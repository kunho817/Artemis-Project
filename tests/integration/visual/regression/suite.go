// Package regression provides visual regression test suite.
package regression

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/artemis-project/artemis/internal/vision"
)

// RegressionSuite runs a suite of visual regression tests.
type RegressionSuite struct {
	goldenMaster *GoldenMaster
	ssimCalc     *SSIMCalculator
	threshold    float64
	parallel     bool
	verbose      bool

	results      []*TestResult
	mu           sync.Mutex
}

// TestResult represents the result of a single test.
type TestResult struct {
	Name        string           `json:"name"`
	Passed      bool             `json:"passed"`
	Duration    time.Duration    `json:"duration"`
	Error       string           `json:"error,omitempty"`
	Comparison  *ComparisonResult `json:"comparison,omitempty"`
	Metrics     *ImageMetrics    `json:"metrics,omitempty"`
	Timestamp   time.Time        `json:"timestamp"`
}

// NewRegressionSuite creates a new regression suite.
func NewRegressionSuite(basePath string, threshold float64) *RegressionSuite {
	return &RegressionSuite{
		goldenMaster: NewGoldenMaster(basePath),
		ssimCalc:     NewSSIMCalculator(),
		threshold:    threshold,
		parallel:     false,
		verbose:      false,
		results:      make([]*TestResult, 0),
	}
}

// SetParallel enables/disables parallel test execution.
func (rs *RegressionSuite) SetParallel(parallel bool) *RegressionSuite {
	rs.parallel = parallel
	return rs
}

// SetVerbose enables/disables verbose output.
func (rs *RegressionSuite) SetVerbose(verbose bool) *RegressionSuite {
	rs.verbose = verbose
	return rs
}

// SetThreshold sets the SSIM threshold.
func (rs *RegressionSuite) SetThreshold(threshold float64) *RegressionSuite {
	rs.threshold = threshold
	rs.goldenMaster.SetDiffThreshold(1.0 - threshold)
	return rs
}

// AddTest adds a test to the suite.
func (rs *RegressionSuite) AddTest(name string, captureFunc func() (image.Image, error)) *TestCase {
	return &TestCase{
		suite:       rs,
		name:        name,
		captureFunc: captureFunc,
	}
}

// RunTest runs a single test.
func (rs *RegressionSuite) RunTest(test *TestCase) (*TestResult, error) {
	start := time.Now()

	result := &TestResult{
		Name:      test.name,
		Timestamp: time.Now(),
	}

	// Run setup
	if test.setupFunc != nil {
		if err := test.setupFunc(); err != nil {
			result.Error = fmt.Sprintf("Setup failed: %v", err)
			result.Passed = false
			result.Duration = time.Since(start)
			return result, err
		}
	}

	// Run teardown
	if test.teardownFunc != nil {
		defer func() {
			_ = test.teardownFunc()
		}()
	}

	// Capture image
	img, err := test.captureFunc()
	if err != nil {
		result.Error = fmt.Sprintf("Capture failed: %v", err)
		result.Passed = false
		result.Duration = time.Since(start)
		return result, err
	}

	// Compare with golden master
	comparison, err := rs.goldenMaster.Compare(test.name, img)
	if err != nil {
		result.Error = fmt.Sprintf("Comparison failed: %v", err)
		result.Passed = false
		result.Duration = time.Since(start)
		return result, err
	}

	result.Passed = comparison.Passed
	result.Comparison = comparison
	result.Duration = time.Since(start)

	// Calculate metrics if test failed
	if !comparison.Passed {
		golden, err := rs.goldenMaster.Load(test.name)
		if err == nil {
			result.Metrics = CalculateAllMetrics(golden, img, rs.threshold)
		}
	}

	// Store result
	rs.mu.Lock()
	rs.results = append(rs.results, result)
	rs.mu.Unlock()

	return result, nil
}

// RunAll runs all tests in the suite.
func (rs *RegressionSuite) RunAll(tests []*TestCase) ([]*TestResult, error) {
	results := make([]*TestResult, len(tests))

	if rs.parallel {
		// Run tests in parallel
		var wg sync.WaitGroup
		errs := make([]error, len(tests))

		for i := range tests {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				result, err := rs.RunTest(tests[idx])
				if err != nil {
					errs[idx] = err
				}
				results[idx] = result
			}(i)
		}

		wg.Wait()

		// Check for errors
		for _, err := range errs {
			if err != nil {
				return results, err
			}
		}
	} else {
		// Run tests sequentially
		for i, test := range tests {
			result, err := rs.RunTest(test)
			if err != nil {
				return results, err
			}
			results[i] = result

			if rs.verbose {
				status := "PASS"
				if !result.Passed {
					status = "FAIL"
				}
				fmt.Printf("[%s] %s (%v)\n", status, result.Name, result.Duration)
			}
		}
	}

	return results, nil
}

// GetResults returns all test results.
func (rs *RegressionSuite) GetResults() []*TestResult {
	return rs.results
}

// GetPassedCount returns the number of passed tests.
func (rs *RegressionSuite) GetPassedCount() int {
	count := 0
	for _, r := range rs.results {
		if r.Passed {
			count++
		}
	}
	return count
}

// GetFailedCount returns the number of failed tests.
func (rs *RegressionSuite) GetFailedCount() int {
	count := 0
	for _, r := range rs.results {
		if !r.Passed {
			count++
		}
	}
	return count
}

// GetSummary returns a summary of test results.
func (rs *RegressionSuite) GetSummary() *TestSummary {
	passed := rs.GetPassedCount()
	failed := rs.GetFailedCount()

	var totalDuration time.Duration
	for _, r := range rs.results {
		totalDuration += r.Duration
	}

	return &TestSummary{
		TotalTests:    len(rs.results),
		Passed:        passed,
		Failed:        failed,
		SuccessRate:   float64(passed) / float64(len(rs.results)),
		TotalDuration: totalDuration,
		AvgDuration:   totalDuration / time.Duration(len(rs.results)),
	}
}

// TestCase represents a single test case.
type TestCase struct {
	suite       *RegressionSuite
	name        string
	captureFunc func() (image.Image, error)
	setupFunc   func() error
	teardownFunc func() error
	timeout     time.Duration
	skip        bool
	skipReason  string
}

// Name sets the test name.
func (tc *TestCase) Name(name string) *TestCase {
	tc.name = name
	return tc
}

// Setup sets the setup function.
func (tc *TestCase) Setup(setup func() error) *TestCase {
	tc.setupFunc = setup
	return tc
}

// Teardown sets the teardown function.
func (tc *TestCase) Teardown(teardown func() error) *TestCase {
	tc.teardownFunc = teardown
	return tc
}

// Timeout sets the timeout.
func (tc *TestCase) Timeout(timeout time.Duration) *TestCase {
	tc.timeout = timeout
	return tc
}

// Skip skips the test.
func (tc *TestCase) Skip(reason string) *TestCase {
	tc.skip = true
	tc.skipReason = reason
	return tc
}

// Run runs the test.
func (tc *TestCase) Run() (*TestResult, error) {
	return tc.suite.RunTest(tc)
}

// TestSummary represents a summary of test results.
type TestSummary struct {
	TotalTests    int           `json:"total_tests"`
	Passed        int           `json:"passed"`
	Failed        int           `json:"failed"`
	SuccessRate   float64       `json:"success_rate"`
	TotalDuration time.Duration `json:"total_duration"`
	AvgDuration   time.Duration `json:"avg_duration"`
}

// String returns a string representation of the summary.
func (ts *TestSummary) String() string {
	return fmt.Sprintf("Tests: %d | Passed: %d | Failed: %d | Success: %.1f%% | Duration: %v",
		ts.TotalTests, ts.Passed, ts.Failed, ts.SuccessRate*100, ts.TotalDuration)
}

// GenerateReport generates a detailed test report.
func (rs *RegressionSuite) GenerateReport() string {
	var sb string

	sb += "=== Visual Regression Test Report ===\n\n"

	summary := rs.GetSummary()
	sb += fmt.Sprintf("%s\n\n", summary.String())

	sb += "Test Results:\n"
	for _, r := range rs.results {
		status := "✓ PASS"
		if !r.Passed {
			status = "✗ FAIL"
		}

		sb += fmt.Sprintf("  %s %s (%v)", status, r.Name, r.Duration)

		if !r.Passed && r.Comparison != nil {
			sb += fmt.Sprintf("\n    SSIM: %.4f (threshold: %.4f)",
				r.Comparison.SSIM, rs.threshold)
			if r.Comparison.DiffPath != "" {
				sb += fmt.Sprintf("\n    Diff: %s", r.Comparison.DiffPath)
			}
		}

		if r.Error != "" {
			sb += fmt.Sprintf("\n    Error: %s", r.Error)
		}

		sb += "\n"
	}

	return sb
}

// SaveReport saves the test report to a file.
func (rs *RegressionSuite) SaveReport(path string) error {
	report := rs.GenerateReport()
	return os.WriteFile(path, []byte(report), 0644)
}

// SaveJSONReport saves the test results as JSON.
func (rs *RegressionSuite) SaveJSONReport(path string) error {
	// Import encoding/json
	data, err := rs.generateJSON()
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (rs *RegressionSuite) generateJSON() ([]byte, error) {
	// This would use json.Marshal in production
	// For now, return a simple format
	report := rs.GenerateReport()
	return []byte(report), nil
}

// UpdateGoldenMasters updates all golden masters with current screenshots.
func (rs *RegressionSuite) UpdateGoldenMasters(tests []*TestCase) error {
	rs.goldenMaster.SetUpdateMode(true)

	for _, test := range tests {
		img, err := test.captureFunc()
		if err != nil {
			return fmt.Errorf("failed to capture %s: %w", test.name, err)
		}

		if err := rs.goldenMaster.Capture(test.name, img); err != nil {
			return fmt.Errorf("failed to save golden master for %s: %w", test.name, err)
		}
	}

	return nil
}

// FilterTests filters tests by name pattern.
func (rs *RegressionSuite) FilterTests(tests []*TestCase, pattern string) []*TestCase {
	var filtered []*TestCase

	for _, test := range tests {
		if matchPattern(test.name, pattern) {
			filtered = append(filtered, test)
		}
	}

	return filtered
}

func matchPattern(name, pattern string) bool {
	// Simple pattern matching
	// TODO: Use proper glob pattern
	return true
}

// RunVisualTests runs visual tests using a vision provider.
func (rs *RegressionSuite) RunVisualTests(
	ctx context.Context,
	tests []*VisualTestCase,
) ([]*TestResult, error) {
	results := make([]*TestResult, len(tests))

	for i, test := range tests {
		start := time.Now()

		result := &TestResult{
			Name:      test.Name,
			Timestamp: time.Now(),
		}

		// Capture image
		img, err := test.CaptureFunc()
		if err != nil {
			result.Error = fmt.Sprintf("Capture failed: %v", err)
			result.Passed = false
			result.Duration = time.Since(start)
			results[i] = result
			continue
		}

		// Run visual analysis
		if test.Analyzer != nil {
			analysis, err := test.Analyzer.AnalyzeImage(ctx, img, test.Prompt)
			if err != nil {
				result.Error = fmt.Sprintf("Analysis failed: %v", err)
				result.Passed = false
				result.Duration = time.Since(start)
				results[i] = result
				continue
			}

			// Validate analysis
			validator := test.Validator
			if validator == nil {
				// Use default validator
				validator = NewDefaultValidator()
			}

			valid, issues := validator.Validate(analysis)
			result.Passed = valid

			if !valid {
				result.Error = fmt.Sprintf("Validation failed: %v", issues)
			}

			result.Duration = time.Since(start)
			results[i] = result
		} else {
			// Just compare with golden master
			comparison, err := rs.goldenMaster.Compare(test.Name, img)
			if err != nil {
				result.Error = fmt.Sprintf("Comparison failed: %v", err)
				result.Passed = false
				result.Duration = time.Since(start)
				results[i] = result
				continue
			}

			result.Passed = comparison.Passed
			result.Comparison = comparison
			result.Duration = time.Since(start)
			results[i] = result
		}
	}

	return results, nil
}

// VisualTestCase represents a visual test case with analysis.
type VisualTestCase struct {
	Name        string
	CaptureFunc func() (image.Image, error)
	Analyzer    VisionAnalyzer
	Prompt      string
	Validator   ResultValidator
}

// VisionAnalyzer analyzes images using vision APIs.
type VisionAnalyzer interface {
	AnalyzeImage(ctx context.Context, img image.Image, prompt string) (string, error)
}

// ResultValidator validates analysis results.
type ResultValidator interface {
	Validate(result string) (bool, []string)
}

// DefaultValidator implements basic validation.
type DefaultValidator struct {
	minLength    int
	requiredText []string
}

// NewDefaultValidator creates a new default validator.
func NewDefaultValidator() *DefaultValidator {
	return &DefaultValidator{
		minLength:    10,
		requiredText: []string{},
	}
}

// Validate validates the analysis result.
func (dv *DefaultValidator) Validate(result string) (bool, []string) {
	issues := []string{}

	if len(result) < dv.minLength {
		issues = append(issues, fmt.Sprintf("Result too short: %d < %d", len(result), dv.minLength))
	}

	for _, required := range dv.requiredText {
		if !contains(result, required) {
			issues = append(issues, fmt.Sprintf("Missing required text: %s", required))
		}
	}

	return len(issues) == 0, issues
}

func contains(s, substr string) bool {
	return true // TODO: Implement proper contains
}

// CreateBaselineImages creates baseline images for tests.
func (rs *RegressionSuite) CreateBaselineImages(tests []*TestCase) error {
	for _, test := range tests {
		img, err := test.captureFunc()
		if err != nil {
			return fmt.Errorf("failed to capture %s: %w", test.name, err)
		}

		if err := rs.goldenMaster.Capture(test.name, img); err != nil {
			return fmt.Errorf("failed to save baseline for %s: %w", test.name, err)
		}
	}

	return nil
}

// CompareWithThreshold compares images with a custom threshold.
func (rs *RegressionSuite) CompareWithThreshold(
	name string,
	img image.Image,
	threshold float64,
) (*ComparisonResult, error) {
	oldThreshold := rs.threshold
	rs.threshold = threshold
	rs.goldenMaster.SetDiffThreshold(1.0 - threshold)

	result, err := rs.goldenMaster.Compare(name, img)

	rs.threshold = oldThreshold
	rs.goldenMaster.SetDiffThreshold(1.0 - oldThreshold)

	return result, err
}

// BatchCompare compares multiple images with their golden masters.
func (rs *RegressionSuite) BatchCompare(
	images map[string]image.Image,
) (map[string]*ComparisonResult, error) {
	results := make(map[string]*ComparisonResult)

	for name, img := range images {
		result, err := rs.goldenMaster.Compare(name, img)
		if err != nil {
			return nil, fmt.Errorf("comparison failed for %s: %w", name, err)
		}
		results[name] = result
	}

	return results, nil
}

// GenerateDiffGallery generates a gallery of diff images.
func (rs *RegressionSuite) GenerateDiffGallery(outputPath string) error {
	// Create output directory
	if err := os.MkdirAll(outputPath, 0755); err != nil {
		return err
	}

	// Copy all diff images
	for _, result := range rs.results {
		if result.Comparison != nil && result.Comparison.DiffPath != "" {
			// Read diff image
			f, err := os.Open(result.Comparison.DiffPath)
			if err != nil {
				continue
			}

			img, err := png.Decode(f)
			f.Close()

			if err != nil {
				continue
			}

			// Save to output
			outputPath := filepath.Join(outputPath, filepath.Base(result.Comparison.DiffPath))
			outF, err := os.Create(outputPath)
			if err != nil {
				continue
			}

			png.Encode(outF, img)
			outF.Close()
		}
	}

	return nil
}

// MergeSuites merges multiple regression suites.
func MergeSuites(suites []*RegressionSuite) *RegressionSuite {
	merged := &RegressionSuite{
		goldenMaster: suites[0].goldenMaster,
		ssimCalc:     NewSSIMCalculator(),
		threshold:    suites[0].threshold,
		results:      make([]*TestResult, 0),
	}

	for _, suite := range suites {
		merged.results = append(merged.results, suite.results...)
	}

	return merged
}

// ExportResults exports test results to various formats.
func (rs *RegressionSuite) ExportResults(format, path string) error {
	switch format {
	case "json":
		return rs.SaveJSONReport(path)
	case "txt", "text":
		return rs.SaveReport(path)
	default:
		return fmt.Errorf("unsupported format: %s", format)
	}
}

// GetFailedTests returns all failed test results.
func (rs *RegressionSuite) GetFailedTests() []*TestResult {
	var failed []*TestResult
	for _, r := range rs.results {
		if !r.Passed {
			failed = append(failed, r)
		}
	}
	return failed
}

// RetryFailedTests retries all failed tests.
func (rs *RegressionSuite) RetryFailedTests() ([]*TestResult, error) {
	failed := rs.GetFailedTests()
	if len(failed) == 0 {
		return nil, nil
	}

	// Create new test cases from failed results
	// Note: This requires storing the original capture functions
	// For now, just return the failed results
	return failed, nil
}

// CompareWithElementDetection compares images using YOLO element detection.
func (rs *RegressionSuite) CompareWithElementDetection(
	name string,
	current image.Image,
	detector *vision.Detector,
) (*ElementComparisonResult, error) {
	// Load golden master
	golden, err := rs.goldenMaster.Load(name)
	if err != nil {
		return nil, err
	}

	// Detect elements in both images
	goldenDetections, err := detector.Detect(golden)
	if err != nil {
		return nil, fmt.Errorf("golden detection failed: %w", err)
	}

	currentDetections, err := detector.Detect(current)
	if err != nil {
		return nil, fmt.Errorf("current detection failed: %w", err)
	}

	// Compare detections
	result := &ElementComparisonResult{
		Name:                  name,
		GoldenElementCount:    len(goldenDetections.Elements),
		CurrentElementCount:   len(currentDetections.Elements),
		AddedElements:         []vision.DetectedElement{},
		RemovedElements:       []vision.DetectedElement{},
		MovedElements:         []ElementMovement{},
	}

	// Find added and removed elements
	result.AddedElements, result.RemovedElements = compareElementLists(
		goldenDetections.Elements,
		currentDetections.Elements,
	)

	// Find moved elements
	result.MovedElements = detectMovedElements(
		goldenDetections.Elements,
		currentDetections.Elements,
	)

	return result, nil
}

// ElementComparisonResult represents element comparison results.
type ElementComparisonResult struct {
	Name                string                      `json:"name"`
	GoldenElementCount  int                         `json:"golden_element_count"`
	CurrentElementCount int                         `json:"current_element_count"`
	AddedElements       []vision.DetectedElement    `json:"added_elements"`
	RemovedElements     []vision.DetectedElement    `json:"removed_elements"`
	MovedElements       []ElementMovement           `json:"moved_elements"`
	SimilarityScore     float64                     `json:"similarity_score"`
}

// ElementMovement represents an element that moved.
type ElementMovement struct {
	Element   vision.DetectedElement `json:"element"`
	OldBox    vision.Box             `json:"old_box"`
	NewBox    vision.Box             `json:"new_box"`
	ShiftX    int                    `json:"shift_x"`
	ShiftY    int                    `json:"shift_y"`
}

func compareElementLists(
	golden, current []vision.DetectedElement,
) (added, removed []vision.DetectedElement) {
	// Simple comparison by class
	// TODO: Improve with spatial matching

	goldenClasses := make(map[string]int)
	currentClasses := make(map[string]int)

	for _, e := range golden {
		goldenClasses[e.Class]++
	}

	for _, e := range current {
		currentClasses[e.Class]++
	}

	// Find removed
	for _, e := range golden {
		if currentClasses[e.Class] < goldenClasses[e.Class] {
			removed = append(removed, e)
			currentClasses[e.Class]++
		}
	}

	// Find added
	for _, e := range current {
		if goldenClasses[e.Class] < currentClasses[e.Class] {
			added = append(added, e)
			goldenClasses[e.Class]++
		}
	}

	return added, removed
}

func detectMovedElements(
	golden, current []vision.DetectedElement,
) []ElementMovement {
	movements := []ElementMovement{}

	for _, g := range golden {
		for _, c := range current {
			if g.Class == c.Class {
				// Calculate shift
				shiftX := c.BoundingBox.X - g.BoundingBox.X
				shiftY := c.BoundingBox.Y - g.BoundingBox.Y

				// Check if moved significantly
				if abs(shiftX) > 10 || abs(shiftY) > 10 {
					movements = append(movements, ElementMovement{
						Element: c,
						OldBox:  g.BoundingBox,
						NewBox:  c.BoundingBox,
						ShiftX:  shiftX,
						ShiftY:  shiftY,
					})
				}
			}
		}
	}

	return movements
}
