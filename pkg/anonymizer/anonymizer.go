package anonymizer

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"

	"github.com/msjurset/anonymark/pkg/detector"
	"github.com/msjurset/anonymark/pkg/renderer"
)

// Anonymizer manages image anonymization workflows.
type Anonymizer struct {
	Detector *detector.Detector
	Renderer *renderer.Renderer
}

// NewAnonymizer creates an Anonymizer instance.
func NewAnonymizer() *Anonymizer {
	return &Anonymizer{
		Detector: detector.NewDetector(),
		Renderer: renderer.NewRenderer(),
	}
}

// ProcessImageFile loads an input PNG/JPEG file, applies redaction, and saves to output path.
func (a *Anonymizer) ProcessImageFile(inputPath, outputPath string, mode renderer.Mode, regions []image.Rectangle) error {
	inFile, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("failed to open input image: %w", err)
	}
	defer inFile.Close()

	srcImg, _, err := image.Decode(inFile)
	if err != nil {
		return fmt.Errorf("failed to decode image: %w", err)
	}

	// Convert srcImg to a mutable draw.Image
	bounds := srcImg.Bounds()
	dstImg := image.NewRGBA(bounds)
	draw.Draw(dstImg, bounds, srcImg, bounds.Min, draw.Src)

	// Apply redaction to each specified region
	for _, rect := range regions {
		a.Renderer.RedactRegion(dstImg, rect, "10.0.4.12", mode)
	}

	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	if err := png.Encode(outFile, dstImg); err != nil {
		return fmt.Errorf("failed to encode output PNG: %w", err)
	}

	return nil
}
