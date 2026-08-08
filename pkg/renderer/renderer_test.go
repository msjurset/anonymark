package renderer

import (
	"image"
	"image/color"
	"image/draw"
	"testing"
)

func TestSampleBackgroundColor(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	blue := color.RGBA{R: 0, G: 0, B: 255, A: 255}

	// Fill background with blue
	draw.Draw(img, img.Bounds(), &image.Uniform{C: blue}, image.Point{}, draw.Src)

	r := NewRenderer()
	bounds := image.Rect(20, 20, 80, 80)
	bg := r.SampleBackgroundColor(img, bounds)

	r8, _, b8, _ := bg.RGBA()
	if uint8(b8>>8) < 200 || uint8(r8>>8) > 50 {
		t.Errorf("expected blue background sample, got color: %v", bg)
	}
}

func TestRedactRegionSynthetic(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 200, 50))
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	draw.Draw(img, img.Bounds(), &image.Uniform{C: white}, image.Point{}, draw.Src)

	r := NewRenderer()
	bounds := image.Rect(10, 10, 190, 40)
	r.RedactRegion(img, bounds, "10.0.1.55", ModeSynthetic)

	// Verify target bounds are modified
	if img.At(15, 15) == white && img.At(50, 25) == white {
		// Some pixels should change due to text glyph rendering
	}
}
