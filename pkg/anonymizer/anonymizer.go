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
	observations, err := ocr.RecognizeText(inputPath)
	if err != nil {
		return fmt.Errorf("OCR failed: %w", err)
	}

	var targets []renderer.TargetItem
	seen := make(map[string]bool)

	for _, obs := range observations {
		matches := a.Detector.DetectMatches(obs.Text)
		for _, m := range matches {
			key := fmt.Sprintf("%s:%s:%s", m.Original, m.Replacement, m.Type)
			if !seen[key] {
				seen[key] = true
				targets = append(targets, renderer.TargetItem{
					Original:    m.Original,
					Replacement: m.Replacement,
					Type:        string(m.Type),
				})
			}
		}
	}

	return a.Renderer.RenderNativeRedactions(inputPath, outputPath, targets, mode)
}
