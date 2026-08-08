package anonymizer

import (
	"fmt"

	"github.com/msjurset/anonymark/pkg/detector"
	"github.com/msjurset/anonymark/pkg/ocr"
	"github.com/msjurset/anonymark/pkg/renderer"
)

// Anonymizer manages image anonymization workflows.
type Anonymizer struct {
	Detector *detector.Detector
	Renderer *renderer.AppKitRenderer
}

// NewAnonymizer creates an Anonymizer instance.
func NewAnonymizer() *Anonymizer {
	return &Anonymizer{
		Detector: detector.NewDetector(),
		Renderer: renderer.NewRenderer(),
	}
}

// ProcessImageFile loads an input PNG/JPEG file, performs OCR, detects PII, applies redaction, and saves output.
func (a *Anonymizer) ProcessImageFile(inputPath, outputPath string, mode renderer.Mode) error {
	// Perform OCR text & bounding box recognition
	observations, err := ocr.RecognizeText(inputPath)
	if err != nil {
		return fmt.Errorf("OCR detection failed: %w", err)
	}

	var items []renderer.RedactionItem

	for _, obs := range observations {
		matches := a.Detector.DetectMatches(obs.Text)
		if len(matches) > 0 {
			items = append(items, renderer.RedactionItem{
				X:           obs.X,
				Y:           obs.Y,
				W:           obs.W,
				H:           obs.H,
				Replacement: matches[0].Replacement,
			})
		}
	}

	fmt.Printf("[anonymark] Redacting %d sensitive PII text regions in screenshot...\n", len(items))

	if err := a.Renderer.RenderNativeRedactions(inputPath, outputPath, items, mode); err != nil {
		return fmt.Errorf("rendering failed: %w", err)
	}

	return nil
}
