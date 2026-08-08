package renderer

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// Mode defines the visual redaction style.
type Mode string

const (
	ModeSynthetic Mode = "synthetic"
	ModeBlur      Mode = "blur"
	ModePill      Mode = "pill"
)

// Renderer handles image canvas operations for anonymization.
type Renderer struct{}

// NewRenderer creates a new Renderer instance.
func NewRenderer() *Renderer {
	return &Renderer{}
}

// SampleBackgroundColor calculates the average color along the perimeter of the bounding box.
func (r *Renderer) SampleBackgroundColor(img image.Image, bounds image.Rectangle) color.Color {
	var rSum, gSum, bSum, count uint64

	minX, maxX := bounds.Min.X, bounds.Max.X
	minY, maxY := bounds.Min.Y, bounds.Max.Y

	// Sample perimeter pixels slightly outside the bounding box
	samplePixel := func(x, y int) {
		if (image.Point{X: x, Y: y}).In(img.Bounds()) {
			cr, cg, cb, _ := img.At(x, y).RGBA()
			rSum += uint64(cr >> 8)
			gSum += uint64(cg >> 8)
			bSum += uint64(cb >> 8)
			count++
		}
	}

	for x := minX; x < maxX; x++ {
		samplePixel(x, minY-1)
		samplePixel(x, maxY)
	}
	for y := minY; y < maxY; y++ {
		samplePixel(minX-1, y)
		samplePixel(maxX, y)
	}

	if count == 0 {
		return color.RGBA{R: 30, G: 30, B: 30, A: 255} // Default dark background
	}

	return color.RGBA{
		R: uint8(rSum / count),
		G: uint8(gSum / count),
		B: uint8(bSum / count),
		A: 255,
	}
}

// SampleForegroundColor finds the text color inside the bounding box distinct from the background color.
func (r *Renderer) SampleForegroundColor(img image.Image, bounds image.Rectangle, bg color.Color) color.Color {
	bgR, bgG, bgB, _ := bg.RGBA()
	bgR8, bgG8, bgB8 := float64(bgR>>8), float64(bgG>>8), float64(bgB>>8)

	var bestColor color.Color
	maxDist := -1.0

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := img.At(x, y)
			cr, cg, cb, _ := c.RGBA()
			r8, g8, b8 := float64(cr>>8), float64(cg>>8), float64(cb>>8)

			dist := math.Sqrt(math.Pow(r8-bgR8, 2) + math.Pow(g8-bgG8, 2) + math.Pow(b8-bgB8, 2))
			if dist > maxDist {
				maxDist = dist
				bestColor = c
			}
		}
	}

	if maxDist < 30 || bestColor == nil {
		// Return contrasting white or dark gray if distance is negligible
		if (bgR8+bgG8+bgB8)/3 > 128 {
			return color.RGBA{R: 20, G: 20, B: 20, A: 255}
		}
		return color.RGBA{R: 240, G: 240, B: 240, A: 255}
	}

	return bestColor
}

// RedactRegion applies the chosen redaction mode over the target bounding box.
func (r *Renderer) RedactRegion(dst draw.Image, bounds image.Rectangle, replacementText string, mode Mode) {
	bgColor := r.SampleBackgroundColor(dst, bounds)
	fgColor := r.SampleForegroundColor(dst, bounds, bgColor)

	switch mode {
	case ModeBlur:
		r.applyBlur(dst, bounds)
	case ModePill:
		r.applyPill(dst, bounds, fgColor, bgColor)
	case ModeSynthetic:
		fallthrough
	default:
		r.applySynthetic(dst, bounds, replacementText, fgColor, bgColor)
	}
}

func (r *Renderer) applySynthetic(dst draw.Image, bounds image.Rectangle, text string, fg, bg color.Color) {
	// Erase original text with sampled background color
	draw.Draw(dst, bounds, &image.Uniform{C: bg}, image.Point{}, draw.Src)

	// Draw synthetic text centered in bounds
	d := &font.Drawer{
		Dst:  dst,
		Src:  &image.Uniform{C: fg},
		Face: basicfont.Face7x13,
	}

	// Calculate vertical baseline centering
	centerY := bounds.Min.Y + (bounds.Dy() / 2) + 4
	d.Dot = fixed.Point26_6{
		X: fixed.I(bounds.Min.X + 2),
		Y: fixed.I(centerY),
	}
	d.DrawString(text)
}

func (r *Renderer) applyPill(dst draw.Image, bounds image.Rectangle, fg, bg color.Color) {
	pillColor := color.RGBA{R: 50, G: 50, B: 50, A: 200}
	draw.Draw(dst, bounds, &image.Uniform{C: pillColor}, image.Point{}, draw.Over)
}

func (r *Renderer) applyBlur(dst draw.Image, bounds image.Rectangle) {
	blockSize := 6
	for y := bounds.Min.Y; y < bounds.Max.Y; y += blockSize {
		for x := bounds.Min.X; x < bounds.Max.X; x += blockSize {
			blockRect := image.Rect(x, y, x+blockSize, y+blockSize).Intersect(bounds)
			if !blockRect.Empty() {
				c := dst.At(x, y)
				draw.Draw(dst, blockRect, &image.Uniform{C: c}, image.Point{}, draw.Src)
			}
		}
	}
}
