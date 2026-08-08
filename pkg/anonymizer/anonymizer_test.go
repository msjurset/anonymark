package anonymizer

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/msjurset/anonymark/pkg/renderer"
)

func TestNewAnonymizer(t *testing.T) {
	anon := NewAnonymizer()
	if anon == nil {
		t.Fatalf("expected non-nil Anonymizer")
	}
	if anon.Detector == nil {
		t.Errorf("expected non-nil Detector")
	}
	if anon.Renderer == nil {
		t.Errorf("expected non-nil Renderer")
	}
}

func TestProcessImageFileBlank(t *testing.T) {
	tempDir := t.TempDir()
	inputPath := filepath.Join(tempDir, "input.png")
	outputPath := filepath.Join(tempDir, "output.png")

	// Create a 100x100 white PNG image
	img := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
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

	anon := NewAnonymizer()
	err = anon.ProcessImageFile(inputPath, outputPath, renderer.ModeSynthetic)
	if err != nil {
		t.Fatalf("unexpected error processing blank image: %v", err)
	}

	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Errorf("output file was not created: %s", outputPath)
	}
}
