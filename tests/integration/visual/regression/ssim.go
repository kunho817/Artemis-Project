// Package regression provides SSIM (Structural Similarity Index) calculation.
package regression

import (
	"image"
	"image/color"
	"math"
)

// SSIMCalculator calculates Structural Similarity Index between images.
// SSIM ranges from -1 to 1, where 1 is identical.
// Formula: SSIM = (2μ₁μ₂ + C₁)(2σ₁₂ + C₂) / (μ₁² + μ₂² + C₁)(σ₁² + σ₂² + C₂)
type SSIMCalculator struct {
	// C1, C2 are constants to stabilize division
	C1 float64
	C2 float64
	// Window size for local statistics
	WindowSize int
	// K1, K2 are constants for C1 and C2 calculation
	K1 float64
	K2 float64
}

// NewSSIMCalculator creates a new SSIM calculator with default parameters.
func NewSSIMCalculator() *SSIMCalculator {
	return &SSIMCalculator{
		K1:         0.01,
		K2:         0.03,
		WindowSize: 8, // 8x8 window
	}
}

// Calculate calculates the SSIM between two images.
func (sc *SSIMCalculator) Calculate(img1, img2 image.Image) float64 {
	// Convert images to grayscale
	gray1 := toGrayscale(img1)
	gray2 := toGrayscale(img2)

	// Calculate constants based on dynamic range
	L := 255.0 // For 8-bit images
	sc.C1 = (sc.K1 * L) * (sc.K1 * L)
	sc.C2 = (sc.K2 * L) * (sc.K2 * L)

	// Calculate SSIM using sliding window
	return sc.calculateWindowSSIM(gray1, gray2)
}

// calculateWindowSSIM calculates SSIM using a sliding window approach.
func (sc *SSIMCalculator) calculateWindowSSIM(img1, img2 *image.Gray) float64 {
	bounds := img1.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	windowSize := sc.WindowSize
	if windowSize <= 0 {
		windowSize = 8
	}

	// Calculate number of windows
	numWindowsX := width / windowSize
	numWindowsY := height / windowSize

	if numWindowsX == 0 || numWindowsY == 0 {
		// Image too small, use global SSIM
		return sc.calculateGlobalSSIM(img1, img2)
	}

	// Calculate SSIM for each window
	ssimSum := 0.0
	count := 0

	for y := 0; y < numWindowsY; y++ {
		for x := 0; x < numWindowsX; x++ {
			// Extract window
			window1 := sc.extractWindow(img1, x*windowSize, y*windowSize, windowSize)
			window2 := sc.extractWindow(img2, x*windowSize, y*windowSize, windowSize)

			// Calculate local SSIM
			ssim := sc.calculateLocalSSIM(window1, window2)
			ssimSum += ssim
			count++
		}
	}

	// Return average SSIM
	if count == 0 {
		return 0.0
	}
	return ssimSum / float64(count)
}

// extractWindow extracts a window from the image.
func (sc *SSIMCalculator) extractWindow(img *image.Gray, startX, startY, size int) []float64 {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	window := make([]float64, size*size)

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			imgX := startX + x
			imgY := startY + y

			if imgX >= 0 && imgX < width && imgY >= 0 && imgY < height {
				idx := img.PixOffset(imgX, imgY)
				window[y*size+x] = float64(img.Pix[idx])
			} else {
				window[y*size+x] = 0.0
			}
		}
	}

	return window
}

// calculateLocalSSIM calculates SSIM for a single window.
func (sc *SSIMCalculator) calculateLocalSSIM(window1, window2 []float64) float64 {
	// Calculate mean
	mean1 := sc.mean(window1)
	mean2 := sc.mean(window2)

	// Calculate variance and covariance
	variance1 := sc.variance(window1, mean1)
	variance2 := sc.variance(window2, mean2)
	covariance := sc.covariance(window1, window2, mean1, mean2)

	// Calculate SSIM
	numerator := (2*mean1*mean2 + sc.C1) * (2*covariance + sc.C2)
	denominator := (mean1*mean1 + mean2*mean2 + sc.C1) * (variance1 + variance2 + sc.C2)

	if denominator == 0 {
		return 0.0
	}

	return numerator / denominator
}

// calculateGlobalSSIM calculates SSIM for the entire image.
func (sc *SSIMCalculator) calculateGlobalSSIM(img1, img2 *image.Gray) float64 {
	bounds := img1.Bounds()
	pixels1 := make([]float64, bounds.Dx()*bounds.Dy())
	pixels2 := make([]float64, bounds.Dx()*bounds.Dy())

	idx := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			i1 := img1.PixOffset(x, y)
			i2 := img2.PixOffset(x, y)
			pixels1[idx] = float64(img1.Pix[i1])
			pixels2[idx] = float64(img2.Pix[i2])
			idx++
		}
	}

	return sc.calculateLocalSSIM(pixels1, pixels2)
}

// mean calculates the mean of a slice.
func (sc *SSIMCalculator) mean(data []float64) float64 {
	if len(data) == 0 {
		return 0.0
	}

	sum := 0.0
	for _, v := range data {
		sum += v
	}
	return sum / float64(len(data))
}

// variance calculates the variance of a slice.
func (sc *SSIMCalculator) variance(data []float64, mean float64) float64 {
	if len(data) == 0 {
		return 0.0
	}

	sum := 0.0
	for _, v := range data {
		diff := v - mean
		sum += diff * diff
	}
	return sum / float64(len(data))
}

// covariance calculates the covariance between two slices.
func (sc *SSIMCalculator) covariance(data1, data2 []float64, mean1, mean2 float64) float64 {
	if len(data1) != len(data2) || len(data1) == 0 {
		return 0.0
	}

	sum := 0.0
	for i := range data1 {
		sum += (data1[i] - mean1) * (data2[i] - mean2)
	}
	return sum / float64(len(data1))
}

// toGrayscale converts an image to grayscale.
func toGrayscale(img image.Image) *image.Gray {
	bounds := img.Bounds()
	gray := image.NewGray(bounds)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := img.At(x, y)
			gray.Set(x, y, c)
		}
	}

	return gray
}

// MSECalculator calculates Mean Squared Error between images.
type MSECalculator struct{}

// NewMSECalculator creates a new MSE calculator.
func NewMSECalculator() *MSECalculator {
	return &MSECalculator{}
}

// Calculate calculates the MSE between two images.
func (mc *MSECalculator) Calculate(img1, img2 image.Image) float64 {
	gray1 := toGrayscale(img1)
	gray2 := toGrayscale(img2)

	bounds := img1.Bounds()
	sum := 0.0
	count := 0

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			i1 := gray1.PixOffset(x, y)
			i2 := gray2.PixOffset(x, y)

			diff := float64(gray1.Pix[i1]) - float64(gray2.Pix[i2])
			sum += diff * diff
			count++
		}
	}

	if count == 0 {
		return 0.0
	}

	return sum / float64(count)
}

// PSNRCalculator calculates Peak Signal-to-Noise Ratio.
type PSNRCalculator struct {
	MaxPixelValue float64
}

// NewPSNRCalculator creates a new PSNR calculator.
func NewPSNRCalculator() *PSNRCalculator {
	return &PSNRCalculator{
		MaxPixelValue: 255.0,
	}
}

// Calculate calculates the PSNR between two images.
// PSNR = 20 * log10(MAX) - 10 * log10(MSE)
func (pc *PSNRCalculator) Calculate(img1, img2 image.Image) float64 {
	mseCalc := NewMSECalculator()
	mse := mseCalc.Calculate(img1, img2)

	if mse == 0 {
		return math.Inf(1) // Perfect match
	}

	psnr := 20*math.Log10(pc.MaxPixelValue) - 10*math.Log10(mse)
	return psnr
}

// ImageMetrics represents various image quality metrics.
type ImageMetrics struct {
	SSIM      float64 `json:"ssim"`
	MSE       float64 `json:"mse"`
	PSNR      float64 `json:"psnr"`
	Equal     bool    `json:"equal"`
	Threshold float64 `json:"threshold"`
}

// CalculateAllMetrics calculates all quality metrics between two images.
func CalculateAllMetrics(img1, img2 image.Image, threshold float64) *ImageMetrics {
	ssimCalc := NewSSIMCalculator()
	mseCalc := NewMSECalculator()
	psnrCalc := NewPSNRCalculator()

	return &ImageMetrics{
		SSIM:      ssimCalc.Calculate(img1, img2),
		MSE:       mseCalc.Calculate(img1, img2),
		PSNR:      psnrCalc.Calculate(img1, img2),
		Equal:     imagesEqual(img1, img2),
		Threshold: threshold,
	}
}

// imagesEqual checks if two images are exactly equal.
func imagesEqual(img1, img2 image.Image) bool {
	bounds1 := img1.Bounds()
	bounds2 := img2.Bounds()

	if bounds1 != bounds2 {
		return false
	}

	for y := bounds1.Min.Y; y < bounds1.Max.Y; y++ {
		for x := bounds1.Min.X; x < bounds1.Max.X; x++ {
			c1 := img1.At(x, y)
			c2 := img2.At(x, y)

			r1, g1, b1, a1 := c1.RGBA()
			r2, g2, b2, a2 := c2.RGBA()

			if r1 != r2 || g1 != g2 || b1 != b2 || a1 != a2 {
				return false
			}
		}
	}

	return true
}

// DifferenceImage creates an image showing the differences between two images.
func DifferenceImage(img1, img2 image.Image, highlightColor color.Color) image.Image {
	bounds := img1.Bounds()
	diff := image.NewRGBA(bounds)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c1 := img1.At(x, y)
			c2 := img2.At(x, y)

			r1, g1, b1, _ := c1.RGBA()
			r2, g2, b2, _ := c2.RGBA()

			if r1 != r2 || g1 != g2 || b1 != b2 {
				diff.Set(x, y, highlightColor)
			} else {
				diff.Set(x, y, c1)
			}
		}
	}

	return diff
}

// BlendImage creates a blended image from two images.
func BlendImage(img1, img2 image.Image, alpha float64) image.Image {
	bounds := img1.Bounds()
	blended := image.NewRGBA(bounds)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c1 := img1.At(x, y)
			c2 := img2.At(x, y)

			r1, g1, b1, a1 := c1.RGBA()
			r2, g2, b2, a2 := c2.RGBA()

			// Convert from 16-bit to 8-bit
			r1_8 := uint8(r1 >> 8)
			g1_8 := uint8(g1 >> 8)
			b1_8 := uint8(b1 >> 8)
			a1_8 := uint8(a1 >> 8)

			r2_8 := uint8(r2 >> 8)
			g2_8 := uint8(g2 >> 8)
			b2_8 := uint8(b2 >> 8)
			a2_8 := uint8(a2 >> 8)

			// Blend
			r := uint8(float64(r1_8)*(1-alpha) + float64(r2_8)*alpha)
			g := uint8(float64(g1_8)*(1-alpha) + float64(g2_8)*alpha)
			b := uint8(float64(b1_8)*(1-alpha) + float64(b2_8)*alpha)
			a := uint8(float64(a1_8)*(1-alpha) + float64(a2_8)*alpha)

			blended.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: a})
		}
	}

	return blended
}

// ScaleImage scales an image by the given factor.
func ScaleImage(img image.Image, scaleX, scaleY float64) image.Image {
	bounds := img.Bounds()
	newWidth := int(float64(bounds.Dx()) * scaleX)
	newHeight := int(float64(bounds.Dy()) * scaleY)

	scaled := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))

	// Simple nearest-neighbor scaling
	for y := 0; y < newHeight; y++ {
		for x := 0; x < newWidth; x++ {
			srcX := int(float64(x) / scaleX)
			srcY := int(float64(y) / scaleY)

			if srcX < bounds.Dx() && srcY < bounds.Dy() {
				c := img.At(bounds.Min.X+srcX, bounds.Min.Y+srcY)
				scaled.Set(x, y, c)
			}
		}
	}

	return scaled
}

// CropImage crops an image to the specified bounds.
func CropImage(img image.Image, cropBounds image.Rectangle) image.Image {
	cropped := image.NewRGBA(cropBounds)

	for y := cropBounds.Min.Y; y < cropBounds.Max.Y; y++ {
		for x := cropBounds.Min.X; x < cropBounds.Max.X; x++ {
			c := img.At(x, y)
			cropped.Set(x-cropBounds.Min.X, y-cropBounds.Min.Y, c)
		}
	}

	return cropped
}

// ResizeImage resizes an image to the specified dimensions.
func ResizeImage(img image.Image, newWidth, newHeight int) image.Image {
	bounds := img.Bounds()
	scaleX := float64(newWidth) / float64(bounds.Dx())
	scaleY := float64(newHeight) / float64(bounds.Dy())
	return ScaleImage(img, scaleX, scaleY)
}

// GetSimilarityScore returns a normalized similarity score (0-1).
// 1 = identical, 0 = completely different.
func GetSimilarityScore(img1, img2 image.Image) float64 {
	ssimCalc := NewSSIMCalculator()
	ssim := ssimCalc.Calculate(img1, img2)

	// SSIM ranges from -1 to 1, normalize to 0-1
	return (ssim + 1) / 2
}

// AreImagesSimilar checks if two images are similar within threshold.
func AreImagesSimilar(img1, img2 image.Image, threshold float64) bool {
	ssimCalc := NewSSIMCalculator()
	ssim := ssimCalc.Calculate(img1, img2)
	return ssim >= threshold
}

// GetImageHash returns a simple perceptual hash of an image.
func GetImageHash(img image.Image) string {
	// Convert to grayscale
	gray := toGrayscale(img)
	bounds := gray.Bounds()

	// Calculate average pixel value
	sum := 0
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			idx := gray.PixOffset(x, y)
			sum += int(gray.Pix[idx])
		}
	}

	avg := sum / (bounds.Dx() * bounds.Dy())

	// Generate hash based on comparison with average
	hash := make([]byte, 0, bounds.Dx()*bounds.Dy()/8)
	currentByte := byte(0)
	bitCount := 0

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			idx := gray.PixOffset(x, y)
			if int(gray.Pix[idx]) > avg {
				currentByte |= 1 << (7 - bitCount)
			}
			bitCount++

			if bitCount == 8 {
				hash = append(hash, currentByte)
				currentByte = 0
				bitCount = 0
			}
		}
	}

	// Convert to hex string
	hexChars := "0123456789abcdef"
	result := make([]byte, len(hash)*2)
	for i, b := range hash {
		result[i*2] = hexChars[b>>4]
		result[i*2+1] = hexChars[b&0x0f]
	}

	return string(result)
}

// HammingDistance calculates the Hamming distance between two strings.
func HammingDistance(s1, s2 string) int {
	if len(s1) != len(s2) {
		return -1
	}

	distance := 0
	for i := range s1 {
		if s1[i] != s2[i] {
			distance++
		}
	}

	return distance
}
