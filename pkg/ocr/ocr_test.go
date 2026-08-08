package ocr

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestRecognizeTextBlankImage(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "blank.png")

	img := image.NewRGBA(image.Rect(0, 0, 50, 50))
	for y := 0; y < 50; y++ {
		for x := 0; x < 50; x++ {
			img.Set(x, y, color.White)
		}
	}

	f, err := os.Create(inputPath)
	if err != nil {
		t.Fatalf("failed to create temp image: %v", err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatalf("failed to encode temp image: %v", err)
	}
	f.Close()

	obs, err := RecognizeText(inputPath)
	if err != nil {
		t.Fatalf("RecognizeText failed: %v", err)
	}

	if len(obs) != 0 {
		t.Errorf("expected 0 text observations on blank image, got %d", len(obs))
	}
}
