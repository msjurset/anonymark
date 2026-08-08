package ocr

import (
	"encoding/json"
	"fmt"
	"image"
	"os/exec"
)

// TextObservation contains recognized text string and its bounding box on the image canvas.
type TextObservation struct {
	Text string          `json:"text"`
	Rect image.Rectangle `json:"-"`
	X    int             `json:"x"`
	Y    int             `json:"y"`
	W    int             `json:"w"`
	H    int             `json:"h"`
}

// RecognizeText uses macOS Vision.framework to extract all text strings and bounding boxes.
func RecognizeText(imagePath string) ([]TextObservation, error) {
	script := fmt.Sprintf(`
import Foundation
import Vision
import AppKit

struct TextObs: Codable {
    let text: String
    let x: Int
    let y: Int
    let w: Int
    let h: Int
}

let imagePath = "%s"
guard let image = NSImage(contentsOfFile: imagePath),
      let cgImage = image.cgImage(forProposedRect: nil, context: nil, hints: nil) else {
    exit(1)
}

let width = CGFloat(cgImage.width)
let height = CGFloat(cgImage.height)

var results: [TextObs] = []
let requestHandler = VNImageRequestHandler(cgImage: cgImage, options: [:])
let request = VNRecognizeTextRequest { request, error in
    guard let observations = request.results as? [VNRecognizedTextObservation] else { return }
    for obs in observations {
        guard let topCandidate = obs.topCandidates(1).first else { continue }
        let bbox = obs.boundingBox
        let x = Int(bbox.origin.x * width)
        let y = Int((1.0 - bbox.origin.y - bbox.height) * height)
        let w = Int(bbox.width * width)
        let h = Int(bbox.height * height)
        results.append(TextObs(text: topCandidate.string, x: x, y: y, w: w, h: h))
    }
}
request.recognitionLevel = .accurate
try? requestHandler.perform([request])

let encoder = JSONEncoder()
if let data = try? encoder.encode(results) {
    print(String(data: data, encoding: .utf8) ?? "[]")
}
`, imagePath)

	cmd := exec.Command("swift", "-")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	go func() {
		defer stdin.Close()
		_, _ = stdin.Write([]byte(script))
	}()

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("macOS Vision OCR failed: %w", err)
	}

	var rawObs []TextObservation
	if err := json.Unmarshal(output, &rawObs); err != nil {
		return nil, fmt.Errorf("failed to parse OCR JSON: %w", err)
	}

	for i := range rawObs {
		rawObs[i].Rect = image.Rect(
			rawObs[i].X,
			rawObs[i].Y,
			rawObs[i].X+rawObs[i].W,
			rawObs[i].Y+rawObs[i].H,
		)
	}

	return rawObs, nil
}
