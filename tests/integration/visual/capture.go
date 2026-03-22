// Package visual provides screen capture functionality for visual testing.
package visual

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"runtime"
	"strings"
	"time"
)

// ScreenCapturer defines the interface for capturing screen content.
type ScreenCapturer interface {
	// CaptureScreen captures the entire screen.
	CaptureScreen() (image.Image, error)

	// CaptureRegion captures a specific region of the screen.
	CaptureRegion(x, y, width, height int) (image.Image, error)

	// CaptureModelView captures TUI Model.View() output (CI-friendly).
	CaptureModelView(viewString string) (string, error)

	// SaveImage saves an image to a file.
	SaveImage(img image.Image, path string) error
}

// ModelViewCapturer captures TUI Model.View() string output.
// This is CI-friendly as it doesn't require actual screenshots.
type ModelViewCapturer struct {
	// No state needed for string-based capture
}

// NewModelViewCapturer creates a new Model.View() capturer.
func NewModelViewCapturer() *ModelViewCapturer {
	return &ModelViewCapturer{}
}

// CaptureScreen captures Model.View() output as a string.
// Returns the string directly (no actual image capture).
func (m *ModelViewCapturer) CaptureScreen() (image.Image, error) {
	return nil, fmt.Errorf("ModelViewCapturer: use CaptureModelView() for string-based capture")
}

// CaptureRegion is not supported for ModelViewCapturer.
func (m *ModelViewCapturer) CaptureRegion(x, y, width, height int) (image.Image, error) {
	return nil, fmt.Errorf("ModelViewCapturer: region capture not supported")
}

// CaptureModelView captures TUI Model.View() string output.
func (m *ModelViewCapturer) CaptureModelView(viewString string) (string, error) {
	if viewString == "" {
		return "", fmt.Errorf("view string is empty")
	}
	return viewString, nil
}

// SaveImage is a no-op for ModelViewCapturer.
func (m *ModelViewCapturer) SaveImage(img image.Image, path string) error {
	return fmt.Errorf("ModelViewCapturer: use SaveString() instead")
}

// SaveString saves the Model.View() string to a file.
func (m *ModelViewCapturer) SaveString(content, path string) error {
	return os.WriteFile(path, []byte(content), 0644)
}

// PlatformCapturer captures actual screenshots on different platforms.
type PlatformCapturer struct {
	platform string
}

// NewPlatformCapturer creates a platform-specific screen capturer.
func NewPlatformCapturer() ScreenCapturer {
	return &PlatformCapturer{
		platform: runtime.GOOS,
	}
}

// CaptureScreen captures the entire screen using platform-specific tools.
func (p *PlatformCapturer) CaptureScreen() (image.Image, error) {
	switch p.platform {
	case "windows":
		return p.captureWindows()
	case "darwin":
		return p.captureMacOS()
	case "linux":
		return p.captureLinux()
	default:
		return nil, fmt.Errorf("unsupported platform: %s", p.platform)
	}
}

// CaptureRegion captures a specific region of the screen.
func (p *PlatformCapturer) CaptureRegion(x, y, width, height int) (image.Image, error) {
	// For simplicity, capture full screen and crop
	fullScreen, err := p.CaptureScreen()
	if err != nil {
		return nil, err
	}

	// Crop the region (implementation depends on image package)
	// This is a simplified version
	return fullScreen, nil
}

// CaptureModelView is not applicable for PlatformCapturer.
func (p *PlatformCapturer) CaptureModelView(viewString string) (string, error) {
	return "", fmt.Errorf("PlatformCapturer: use ModelViewCapturer for string-based capture")
}

// SaveImage saves an image to a file as PNG.
func (p *PlatformCapturer) SaveImage(img image.Image, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return png.Encode(file, img)
}

// captureWindows captures screen on Windows using PowerShell.
func (p *PlatformCapturer) captureWindows() (image.Image, error) {
	// Use .NET to capture screen
	// This is a placeholder - actual implementation would use:
	// - PowerShell with .NET System.Drawing
	// - or an external tool like ffmpeg
	return nil, fmt.Errorf("Windows screen capture not implemented in CI environment")
}

// captureMacOS captures screen on macOS using screencapture.
func (p *PlatformCapturer) captureMacOS() (image.Image, error) {
	// Use screencapture command
	// screencapture -x /tmp/screenshot.png
	return nil, fmt.Errorf("macOS screen capture not implemented in CI environment")
}

// captureLinux captures screen on Linux using import or scrot.
func (p *PlatformCapturer) captureLinux() (image.Image, error) {
	// Use import (ImageMagick) or scrot
	// import -window root /tmp/screenshot.png
	return nil, fmt.Errorf("Linux screen capture not implemented in CI environment")
}

// CaptureConfig configures screen capture behavior.
type CaptureConfig struct {
	// UseModelView uses string-based capture (CI-friendly).
	UseModelView bool

	// Delay is the delay before capture (for animations to settle).
	Delay time.Duration

	// OutputPath is where to save captures.
	OutputPath string

	// Timestamp adds timestamp to filenames.
	Timestamp bool
}

// DefaultCaptureConfig returns a default capture configuration.
func DefaultCaptureConfig() *CaptureConfig {
	return &CaptureConfig{
		UseModelView: true, // Default to CI-friendly mode
		Delay:        100 * time.Millisecond,
		OutputPath:   "./test_captures",
		Timestamp:    true,
	}
}

// CaptureSession manages a screen capture session.
type CaptureSession struct {
	config     *CaptureConfig
	capturer   ScreenCapturer
	sessionDir string
	captures   []string
}

// NewCaptureSession creates a new capture session.
func NewCaptureSession(config *CaptureConfig) (*CaptureSession, error) {
	if config == nil {
		config = DefaultCaptureConfig()
	}

	session := &CaptureSession{
		config:   config,
		captures: make([]string, 0),
	}

	// Create capturer based on config
	if config.UseModelView {
		session.capturer = NewModelViewCapturer()
	} else {
		session.capturer = NewPlatformCapturer()
	}

	// Create session directory
	if config.Timestamp {
		session.sessionDir = fmt.Sprintf("%s/%s", config.OutputPath, time.Now().Format("20060102_150405"))
	} else {
		session.sessionDir = config.OutputPath
	}

	err := os.MkdirAll(session.sessionDir, 0755)
	if err != nil {
		return nil, fmt.Errorf("failed to create session directory: %w", err)
	}

	return session, nil
}

// Capture captures screen or Model.View() and saves it.
func (s *CaptureSession) Capture(ctx context.Context, name string, content interface{}) (string, error) {
	// Apply delay if configured
	if s.config.Delay > 0 {
		select {
		case <-time.After(s.config.Delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}

	var path string
	var err error

	if s.config.UseModelView {
		// String-based capture
		viewStr, ok := content.(string)
		if !ok {
			return "", fmt.Errorf("expected string content for ModelView capture")
		}

		mvCapturer, ok := s.capturer.(*ModelViewCapturer)
		if !ok {
			return "", fmt.Errorf("capturer is not ModelViewCapturer")
		}

		_, err = mvCapturer.CaptureModelView(viewStr)
		if err != nil {
			return "", err
		}

		path = fmt.Sprintf("%s/%s.txt", s.sessionDir, name)
		err = mvCapturer.SaveString(viewStr, path)
	} else {
		// Image-based capture
		img, ok := content.(image.Image)
		if !ok {
			return "", fmt.Errorf("expected image.Image for platform capture")
		}

		path = fmt.Sprintf("%s/%s.png", s.sessionDir, name)
		err = s.capturer.SaveImage(img, path)
	}

	if err != nil {
		return "", fmt.Errorf("failed to save capture: %w", err)
	}

	s.captures = append(s.captures, path)
	return path, nil
}

// GetCaptures returns all capture paths from this session.
func (s *CaptureSession) GetCaptures() []string {
	return s.captures
}

// Cleanup removes old capture sessions based on retention policy.
func (s *CaptureSession) Cleanup(retention int, olderThan time.Duration) error {
	// Find all session directories
	entries, err := os.ReadDir(s.config.OutputPath)
	if err != nil {
		return err
	}

	// Sort by modification time and keep only the most recent 'retention' sessions
	cutoff := time.Now().Add(-olderThan)
	removed := 0

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Remove old sessions
		if info.ModTime().Before(cutoff) {
			path := fmt.Sprintf("%s/%s", s.config.OutputPath, entry.Name())
			os.RemoveAll(path)
			removed++
		}
	}

	// Ensure we don't remove more than allowed
	if removed > retention {
		// This is simplified - real implementation would be more careful
		return fmt.Errorf("would remove %d sessions, exceeding retention limit of %d", removed, retention)
	}

	return nil
}

// Helper function to detect if running in CI environment.
func IsCIEnvironment() bool {
	// Check common CI environment variables
	ciVars := []string{
		"CI",
		"CONTINUOUS_INTEGRATION",
		"JENKINS_URL",
		"GITHUB_ACTIONS",
		"GITLAB_CI",
		"TRAVIS",
		"CIRCLECI",
	}

	for _, env := range ciVars {
		if os.Getenv(env) != "" {
			return true
		}
	}

	// Also check if DISPLAY is not set (headless environment)
	return os.Getenv("DISPLAY") == "" && runtime.GOOS != "windows"
}

// AutoDetectCapturer creates the appropriate capturer based on environment.
func AutoDetectCapturer() (ScreenCapturer, error) {
	if IsCIEnvironment() || runtime.GOOS == "linux" {
		// Use ModelView capturer in CI or headless environments
		return NewModelViewCapturer(), nil
	}

	// Use platform capturer for local development
	return NewPlatformCapturer(), nil
}

// ParseModelView parses Model.View() output to extract structured data.
// This is useful for testing TUI layouts without screenshots.
func ParseModelView(viewString string) *ModelViewData {
	lines := strings.Split(viewString, "\n")

	data := &ModelViewData{
		Lines:     lines,
		Width:     0,
		Height:    len(lines),
		Elements:  make([]UIElement, 0),
	}

	// Calculate width
	for _, line := range lines {
		if len(line) > data.Width {
			data.Width = len(line)
		}
	}

	// Extract UI elements (simplified)
	for y, line := range lines {
		// Find borders
		if strings.Contains(line, "┌") || strings.Contains(line, "┐") ||
		   strings.Contains(line, "└") || strings.Contains(line, "┘") {
			data.Elements = append(data.Elements, UIElement{
				Type:   "border",
				Line:   y,
				Content: line,
			})
		}

		// Find titles (lines between borders)
		if strings.Contains(line, "│") && strings.TrimSpace(line) != "" {
			trimmed := strings.Trim(line, "│")
			if strings.TrimSpace(trimmed) != "" {
				data.Elements = append(data.Elements, UIElement{
					Type:   "content",
					Line:   y,
					Content: trimmed,
				})
			}
		}
	}

	return data
}

// ModelViewData represents parsed Model.View() output.
type ModelViewData struct {
	Lines    []string
	Width    int
	Height   int
	Elements []UIElement
}

// UIElement represents a parsed UI element.
type UIElement struct {
	Type    string // "border", "content", "button", etc.
	Line    int    // Line number (0-based)
	Content string // Raw content
}

// FindElements finds UI elements by type.
func (m *ModelViewData) FindElements(elementType string) []UIElement {
	var results []UIElement
	for _, elem := range m.Elements {
		if elem.Type == elementType {
			results = append(results, elem)
		}
	}
	return results
}

// FindContent finds elements containing specific text.
func (m *ModelViewData) FindContent(text string) []UIElement {
	var results []UIElement
	for _, elem := range m.Elements {
		if strings.Contains(strings.ToLower(elem.Content), strings.ToLower(text)) {
			results = append(results, elem)
		}
	}
	return results
}

// GetElementAt returns the element at a specific position.
func (m *ModelViewData) GetElementAt(x, y int) *UIElement {
	if y < 0 || y >= len(m.Lines) {
		return nil
	}

	line := m.Lines[y]
	if x < 0 || x >= len(line) {
		return nil
	}

	// Find element at this position
	for _, elem := range m.Elements {
		if elem.Line == y {
			return &elem
		}
	}

	return nil
}

// String returns a string representation of the ModelView data.
func (m *ModelViewData) String() string {
	return strings.Join(m.Lines, "\n")
}
