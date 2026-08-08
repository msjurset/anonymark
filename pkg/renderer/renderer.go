package renderer

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"os/exec"

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

// TargetItem represents a target PII string and its synthetic replacement for rendering.
type TargetItem struct {
	Original    string `json:"original"`
	Replacement string `json:"replacement"`
	Type        string `json:"type"`
}

// AppKitRenderer handles image canvas operations for anonymization.
type AppKitRenderer struct{}

// NewRenderer creates a new AppKitRenderer instance.
func NewRenderer() *AppKitRenderer {
	return &AppKitRenderer{}
}

// SampleBackgroundColor calculates the average color along the perimeter of the bounding box.
func (r *AppKitRenderer) SampleBackgroundColor(img image.Image, bounds image.Rectangle) color.Color {
	var rSum, gSum, bSum, count uint64

	minX, maxX := bounds.Min.X, bounds.Max.X
	minY, maxY := bounds.Min.Y, bounds.Max.Y

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
		return color.RGBA{R: 30, G: 30, B: 30, A: 255}
	}

	return color.RGBA{
		R: uint8(rSum / count),
		G: uint8(gSum / count),
		B: uint8(bSum / count),
		A: 255,
	}
}

// SampleForegroundColor finds the text color inside the bounding box distinct from the background color.
func (r *AppKitRenderer) SampleForegroundColor(img image.Image, bounds image.Rectangle, bg color.Color) color.Color {
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
		if (bgR8+bgG8+bgB8)/3 > 128 {
			return color.RGBA{R: 20, G: 20, B: 20, A: 255}
		}
		return color.RGBA{R: 240, G: 240, B: 240, A: 255}
	}

	return bestColor
}

// RedactRegion applies the chosen redaction mode over the target bounding box.
func (r *AppKitRenderer) RedactRegion(dst draw.Image, bounds image.Rectangle, replacementText string, mode Mode) {
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

func (r *AppKitRenderer) applySynthetic(dst draw.Image, bounds image.Rectangle, text string, fg, bg color.Color) {
	draw.Draw(dst, bounds, &image.Uniform{C: bg}, image.Point{}, draw.Src)

	d := &font.Drawer{
		Dst:  dst,
		Src:  &image.Uniform{C: fg},
		Face: basicfont.Face7x13,
	}

	centerY := bounds.Min.Y + (bounds.Dy() / 2) + 4
	d.Dot = fixed.Point26_6{
		X: fixed.I(bounds.Min.X + 2),
		Y: fixed.I(centerY),
	}
	d.DrawString(text)
}

func (r *AppKitRenderer) applyPill(dst draw.Image, bounds image.Rectangle, fg, bg color.Color) {
	pillColor := color.RGBA{R: 50, G: 50, B: 50, A: 200}
	draw.Draw(dst, bounds, &image.Uniform{C: pillColor}, image.Point{}, draw.Over)
}

func (r *AppKitRenderer) applyBlur(dst draw.Image, bounds image.Rectangle) {
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

// RenderNativeRedactions performs Vision OCR character-level bounding box matching and in-place SF Pro text rendering.
func (r *AppKitRenderer) RenderNativeRedactions(inputPath, outputPath string, targets []TargetItem, mode Mode) error {
	if len(targets) == 0 {
		inputData, err := os.ReadFile(inputPath)
		if err != nil {
			return fmt.Errorf("failed to read input image: %w", err)
		}
		if err := os.WriteFile(outputPath, inputData, 0644); err != nil {
			return fmt.Errorf("failed to write output image: %w", err)
		}
		return nil
	}

	jsonBytes, err := json.Marshal(targets)
	if err != nil {
		return fmt.Errorf("failed to marshal targets: %w", err)
	}
	b64Targets := base64.StdEncoding.EncodeToString(jsonBytes)

	script := fmt.Sprintf(`
import Foundation
import Vision
import AppKit
import CoreGraphics

struct TargetItem: Codable {
    let original: String
    let replacement: String
    let type: String
}

let inputPath = "%s"
let outputPath = "%s"
let mode = "%s"
let b64 = "%s"

guard let data = Data(base64Encoded: b64),
      let targets = try? JSONDecoder().decode([TargetItem].self, from: data) else {
    exit(1)
}

guard let image = NSImage(contentsOfFile: inputPath),
      let cgImage = image.cgImage(forProposedRect: nil, context: nil, hints: nil) else {
    print("Error loading image")
    exit(1)
}

let width = cgImage.width
let height = cgImage.height

guard let colorSpace = CGColorSpace(name: CGColorSpace.sRGB),
      let context = CGContext(data: nil,
                              width: width,
                              height: height,
                              bitsPerComponent: 8,
                              bytesPerRow: width * 4,
                              space: colorSpace,
                              bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue) else {
    print("Error creating CGContext")
    exit(1)
}

context.draw(cgImage, in: CGRect(x: 0, y: 0, width: width, height: height))

func samplePixelColor(context: CGContext, x: Int, y: Int, width: Int, height: Int) -> NSColor {
    guard let dataPtr = context.data else { return NSColor.black }
    let pointer = dataPtr.bindMemory(to: UInt8.self, capacity: width * height * 4)
    let safeX = max(0, min(width - 1, x))
    let safeY = max(0, min(height - 1, y))
    let offset = (safeY * width + safeX) * 4
    let r = CGFloat(pointer[offset]) / 255.0
    let g = CGFloat(pointer[offset + 1]) / 255.0
    let b = CGFloat(pointer[offset + 2]) / 255.0
    let a = CGFloat(pointer[offset + 3]) / 255.0
    return NSColor(srgbRed: r, green: g, blue: b, alpha: a)
}

func sampleBackgroundColor(context: CGContext, rect: CGRect, width: Int, height: Int) -> NSColor {
    let margin: CGFloat = 3.0
    let points: [CGPoint] = [
        CGPoint(x: rect.minX - margin, y: rect.midY),
        CGPoint(x: rect.maxX + margin, y: rect.midY),
        CGPoint(x: rect.midX, y: rect.minY - margin),
        CGPoint(x: rect.midX, y: rect.maxY + margin),
        CGPoint(x: rect.minX - margin, y: rect.minY - margin),
        CGPoint(x: rect.maxX + margin, y: rect.minY - margin),
        CGPoint(x: rect.minX - margin, y: rect.maxY + margin),
        CGPoint(x: rect.maxX + margin, y: rect.maxY + margin)
    ]
    
    var rSum: CGFloat = 0, gSum: CGFloat = 0, bSum: CGFloat = 0, aSum: CGFloat = 0
    var count: CGFloat = 0
    
    for pt in points {
        let x = max(0, min(width - 1, Int(pt.x)))
        let y = max(0, min(height - 1, Int(pt.y)))
        let c = samplePixelColor(context: context, x: x, y: y, width: width, height: height)
        var r: CGFloat = 0, g: CGFloat = 0, b: CGFloat = 0, a: CGFloat = 0
        c.getRed(&r, green: &g, blue: &b, alpha: &a)
        rSum += r
        gSum += g
        bSum += b
        aSum += a
        count += 1
    }
    
    return NSColor(srgbRed: rSum / count, green: gSum / count, blue: bSum / count, alpha: aSum / count)
}

struct MatchRegion {
    let original: String
    let replacement: String
    let type: String
    let rect: CGRect
    let bgColor: NSColor
}

let requestHandler = VNImageRequestHandler(cgImage: cgImage, options: [:])
let request = VNRecognizeTextRequest { request, error in
    guard let observations = request.results as? [VNRecognizedTextObservation] else { return }

    var matchRegions: [MatchRegion] = []
    var matchedRects: [CGRect] = []

    for obs in observations {
        guard let topCandidate = obs.topCandidates(1).first else { continue }
        let text = topCandidate.string

        let lineY = obs.boundingBox.origin.y * CGFloat(height)
        let lineH = obs.boundingBox.height * CGFloat(height)

        for t in targets {
            var searchRange = text.startIndex..<text.endIndex
            while let range = text.range(of: t.original, options: [], range: searchRange) {
                defer { searchRange = range.upperBound..<text.endIndex }
                guard let box = try? topCandidate.boundingBox(for: range) else { continue }

                let x = box.boundingBox.origin.x * CGFloat(width)
                let w = box.boundingBox.width * CGFloat(width)
                let r = CGRect(x: x, y: lineY, width: w, height: lineH)

                let isDuplicate = matchedRects.contains { existing in
                    let dx = abs(existing.midX - r.midX)
                    let dy = abs(existing.midY - r.midY)
                    return dx < 15.0 && dy < 6.0
                }
                if isDuplicate { continue }

                let bg = sampleBackgroundColor(context: context, rect: r, width: width, height: height)
                matchedRects.append(r)
                matchRegions.append(MatchRegion(original: t.original, replacement: t.replacement, type: t.type, rect: r, bgColor: bg))
            }
        }
    }

    NSGraphicsContext.saveGraphicsState()
    let nsContext = NSGraphicsContext(cgContext: context, flipped: false)
    NSGraphicsContext.current = nsContext

    // PASS 1: Erase ALL original target bounding boxes first
    for region in matchRegions {
        region.bgColor.setFill()
        let eraseRect = CGRect(x: region.rect.minX, y: region.rect.minY, width: region.rect.width, height: region.rect.height)
        NSBezierPath.fill(eraseRect)

        if mode == "blur" {
            NSColor.gray.withAlphaComponent(0.8).setFill()
            NSBezierPath.fill(region.rect)
        } else if mode == "pill" {
            NSColor.darkGray.setFill()
            NSBezierPath.fill(region.rect)
        }
    }

    // PASS 2: Draw ALL synthetic replacement text cleanly over erased canvas
    if mode == "synthetic" {
        for region in matchRegions {
            let r = region.rect
            var bgR: CGFloat = 0, bgG: CGFloat = 0, bgB: CGFloat = 0, bgA: CGFloat = 0
            region.bgColor.getRed(&bgR, green: &bgG, blue: &bgB, alpha: &bgA)
            let bgBrightness = (bgR * 0.299 + bgG * 0.587 + bgB * 0.114)

            var textColor: NSColor
            var fontWeight: NSFont.Weight = .regular

            if region.type == "hostname" {
                fontWeight = .bold
            } else if region.type == "mac" || region.type == "token" {
                fontWeight = .medium
            }

            if bgBrightness > 0.5 {
                textColor = (region.type == "ipv4") ? NSColor(srgbRed: 0.35, green: 0.35, blue: 0.37, alpha: 1.0) : NSColor.black
            } else if bgR < 0.1 && bgB > 0.5 {
                textColor = NSColor.white
            } else {
                textColor = (region.type == "ipv4") ? NSColor(srgbRed: 0.75, green: 0.75, blue: 0.77, alpha: 1.0) : NSColor.white
            }

            var fontSize: CGFloat = (region.type == "ipv4") ? 11.0 : (region.type == "hostname" ? 13.0 : 12.0)

            var font = NSFont.systemFont(ofSize: fontSize, weight: fontWeight)
            var attributes: [NSAttributedString.Key: Any] = [
                .font: font,
                .foregroundColor: textColor
            ]
            var attrStr = NSAttributedString(string: region.replacement, attributes: attributes)
            var strSize = attrStr.size()

            while (strSize.height > r.height || strSize.width > r.width + 12.0) && fontSize > 8.0 {
                fontSize -= 0.5
                font = NSFont.systemFont(ofSize: fontSize, weight: fontWeight)
                attributes[.font] = font
                attrStr = NSAttributedString(string: region.replacement, attributes: attributes)
                strSize = attrStr.size()
            }

            let textY = r.minY + (r.height - strSize.height) / 2.0
            let drawPoint = CGPoint(x: r.minX, y: textY)
            attrStr.draw(at: drawPoint)
        }
    }

    NSGraphicsContext.restoreGraphicsState()
}

request.recognitionLevel = .accurate
try? requestHandler.perform([request])

guard let outputCGImage = context.makeImage() else { exit(1) }
let imageRep = NSBitmapImageRep(cgImage: outputCGImage)
guard let pngData = imageRep.representation(using: .png, properties: [:]) else { exit(1) }
try? pngData.write(to: URL(fileURLWithPath: outputPath))
`, inputPath, outputPath, string(mode), b64Targets)

	cmd := exec.Command("swift", "-")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	go func() {
		defer stdin.Close()
		_, _ = stdin.Write([]byte(script))
	}()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("AppKit native renderer failed: %w (output: %s)", err, string(output))
	}

	return nil
}
