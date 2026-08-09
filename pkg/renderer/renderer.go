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
	"sort"

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

	// Sort targets by length descending to match longest PII strings first (e.g. 192.168.86.134 before 192.168.86.1)
	sortedTargets := make([]TargetItem, len(targets))
	copy(sortedTargets, targets)
	sort.Slice(sortedTargets, func(i, j int) bool {
		return len(sortedTargets[i].Original) > len(sortedTargets[j].Original)
	})

	jsonBytes, err := json.Marshal(sortedTargets)
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

// Calculate Retina 2x scale factor relative to standard 1x macOS point coordinates
let scaleFactor = max(1.0, CGFloat(height) / 800.0)

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

// Flips Y coordinate when indexing raw bitmap byte buffer (since CGContext memory starts at top-left)
func samplePixelColor(context: CGContext, x: Int, y: Int, width: Int, height: Int) -> NSColor {
    guard let dataPtr = context.data else { return NSColor.black }
    let pointer = dataPtr.bindMemory(to: UInt8.self, capacity: width * height * 4)
    let safeX = max(0, min(width - 1, x))
    let safeY = max(0, min(height - 1, (height - 1) - y))
    let offset = (safeY * width + safeX) * 4
    let r = CGFloat(pointer[offset]) / 255.0
    let g = CGFloat(pointer[offset + 1]) / 255.0
    let b = CGFloat(pointer[offset + 2]) / 255.0
    let a = CGFloat(pointer[offset + 3]) / 255.0
    return NSColor(srgbRed: r, green: g, blue: b, alpha: a)
}

func sampleBackgroundColor(context: CGContext, rect: CGRect, width: Int, height: Int) -> NSColor {
    let minX = max(0, min(width - 1, Int(rect.minX)))
    let maxX = max(0, min(width - 1, Int(rect.maxX)))
    let minY = max(0, min(height - 1, Int(rect.minY)))
    let maxY = max(0, min(height - 1, Int(rect.maxY)))

    var samples: [NSColor] = []
    let margin = 2
    let sMinX = max(0, minX - margin)
    let sMaxX = min(width - 1, maxX + margin)
    let sMinY = max(0, minY - margin)
    let sMaxY = min(height - 1, maxY + margin)

    let stepX = max(1, (sMaxX - sMinX) / 8)
    let stepY = max(1, (sMaxY - sMinY) / 4)

    for x in stride(from: sMinX, to: sMaxX, by: stepX) {
        samples.append(samplePixelColor(context: context, x: x, y: sMaxY, width: width, height: height))
        samples.append(samplePixelColor(context: context, x: x, y: sMinY, width: width, height: height))
    }
    for y in stride(from: sMinY, to: sMaxY, by: stepY) {
        samples.append(samplePixelColor(context: context, x: sMinX, y: y, width: width, height: height))
        samples.append(samplePixelColor(context: context, x: sMaxX, y: y, width: width, height: height))
    }

    if samples.isEmpty {
        return NSColor(srgbRed: 0.18, green: 0.18, blue: 0.18, alpha: 1.0)
    }

    var maxClusterCount = 0
    var dominantColor = samples[0]

    for c1 in samples {
        var r1: CGFloat = 0, g1: CGFloat = 0, b1: CGFloat = 0, a1: CGFloat = 0
        c1.getRed(&r1, green: &g1, blue: &b1, alpha: &a1)
        var count = 0
        for c2 in samples {
            var r2: CGFloat = 0, g2: CGFloat = 0, b2: CGFloat = 0, a2: CGFloat = 0
            c2.getRed(&r2, green: &g2, blue: &b2, alpha: &a2)
            let dist = abs(r1 - r2) + abs(g1 - g2) + abs(b1 - b2)
            if dist < 0.20 {
                count += 1
            }
        }
        if count > maxClusterCount {
            maxClusterCount = count
            dominantColor = c1
        }
    }

    return dominantColor
}

enum LayoutCategory {
    case headerTitle
    case headerSubtitle
    case listPrimary
    case listSecondary
    case detailLabel
}

struct MatchRegion {
    let original: String
    let replacement: String
    let type: String
    let rect: CGRect
    let lineY: CGFloat
    let lineH: CGFloat
    let bgColor: NSColor
    let category: LayoutCategory
}

let requestHandler = VNImageRequestHandler(cgImage: cgImage, options: [:])
let request = VNRecognizeTextRequest { request, error in
    guard let observations = request.results as? [VNRecognizedTextObservation] else { return }

    var rawMatches: [(target: TargetItem, rect: CGRect, lineY: CGFloat, lineH: CGFloat, isStandaloneTitle: Bool)] = []

    for obs in observations {
        guard let topCandidate = obs.topCandidates(1).first else { continue }
        let text = topCandidate.string

        let lineY = obs.boundingBox.origin.y * CGFloat(height)
        let lineH = obs.boundingBox.height * CGFloat(height)

        for t in targets {
            var searchRange = text.startIndex..<text.endIndex
            while let range = text.range(of: t.original, options: [], range: searchRange) {
                if range.upperBound < text.endIndex {
                    let nextChar = text[range.upperBound]
                    if nextChar.isNumber || nextChar.isLetter {
                        searchRange = range.upperBound..<text.endIndex
                        continue
                    }
                }
                if range.lowerBound > text.startIndex {
                    let prevChar = text[text.index(before: range.lowerBound)]
                    if prevChar.isNumber || prevChar.isLetter {
                        searchRange = range.upperBound..<text.endIndex
                        continue
                    }
                }

                defer { searchRange = range.upperBound..<text.endIndex }
                guard let box = try? topCandidate.boundingBox(for: range) else { continue }

                let x = box.boundingBox.origin.x * CGFloat(width)
                let w = box.boundingBox.width * CGFloat(width)
                let r = CGRect(x: x, y: lineY, width: w, height: lineH)

                let isPrecededByDescription = text.lowercased().contains("discovered at")
                let isStandaloneTitle = !isPrecededByDescription

                let isDuplicate = rawMatches.contains { existing in
                    let dx = abs(existing.rect.midX - r.midX)
                    let dy = abs(existing.rect.midY - r.midY)
                    return dx < 15.0 && dy < 6.0
                }
                if isDuplicate { continue }
                rawMatches.append((target: t, rect: r, lineY: lineY, lineH: lineH, isStandaloneTitle: isStandaloneTitle))
            }
        }
    }

    rawMatches.sort { $0.rect.minY > $1.rect.minY }

    var matchRegions: [MatchRegion] = []

    for m in rawMatches {
        let r = m.rect
        let t = m.target
        let xRatio = r.minX / CGFloat(width)
        let yRatio = r.minY / CGFloat(height)

        // Pure spatial layout classification: determine primary title line vs secondary line based on spatial geometry
        let isLine1 = m.isStandaloneTitle || rawMatches.contains { other in
            let dy = m.rect.minY - other.rect.minY
            let dx = abs(m.rect.minX - other.rect.minX)
            return dy > 18.0 && dy < 45.0 && dx < 60.0
        }

        var category: LayoutCategory = .listPrimary
        if xRatio > 0.45 && yRatio > 0.80 {
            category = .headerTitle
        } else if isLine1 {
            category = .listPrimary
        } else {
            category = .listSecondary
        }

        let bg = sampleBackgroundColor(context: context, rect: r, width: width, height: height)
        matchRegions.append(MatchRegion(original: t.original, replacement: t.replacement, type: t.type, rect: r, lineY: m.lineY, lineH: m.lineH, bgColor: bg, category: category))
    }

    // CLUSTER UNIFICATION: Compute median line heights and unified main list left X-margin
    // This guarantees that all list items (e.g. device names) get identical font sizes and align vertically
    var categoryLineHeights: [LayoutCategory: [CGFloat]] = [:]
    var allListLeftX: [CGFloat] = []

    for m in matchRegions {
        categoryLineHeights[m.category, default: []].append(m.lineH)
        if m.category == .listPrimary || m.category == .listSecondary {
            allListLeftX.append(m.rect.minX)
        }
    }

    var medianLineHeights: [LayoutCategory: CGFloat] = [:]
    var unifiedListX: CGFloat? = nil

    for (cat, heights) in categoryLineHeights {
        let sortedH = heights.sorted()
        let midH = sortedH.count / 2
        medianLineHeights[cat] = sortedH.count %% 2 == 0 ? (sortedH[midH - 1] + sortedH[midH]) / 2.0 : sortedH[midH]
    }
    if !allListLeftX.isEmpty {
        let sortedX = allListLeftX.sorted()
        let midX = sortedX.count / 2
        unifiedListX = sortedX.count %% 2 == 0 ? (sortedX[midX - 1] + sortedX[midX]) / 2.0 : sortedX[midX]
    }

    NSGraphicsContext.saveGraphicsState()
    let nsContext = NSGraphicsContext(cgContext: context, flipped: false)
    NSGraphicsContext.current = nsContext

    // PASS 1: Cleanly erase original target bounding boxes with +4px padding
    for region in matchRegions {
        region.bgColor.setFill()
        let eraseRect = CGRect(x: region.rect.minX - 2, y: region.lineY - 2, width: region.rect.width + 4, height: region.lineH + 4)
        NSBezierPath.fill(eraseRect)

        if mode == "blur" {
            NSColor.gray.withAlphaComponent(0.8).setFill()
            NSBezierPath.fill(region.rect)
        } else if mode == "pill" {
            NSColor.darkGray.setFill()
            NSBezierPath.fill(region.rect)
        }
    }

    // PASS 2: Render synthetic replacement text using EXACT UNIFORM TYPOGRAPHY & VERTICAL ALIGNMENT
    if mode == "synthetic" {
        for region in matchRegions {
            let r = region.rect
            var bgR: CGFloat = 0, bgG: CGFloat = 0, bgB: CGFloat = 0, bgA: CGFloat = 0
            region.bgColor.getRed(&bgR, green: &bgG, blue: &bgB, alpha: &bgA)
            let bgBrightness = (bgR * 0.299 + bgG * 0.587 + bgB * 0.114)
            let isBlueBg = (bgR < 0.15 && bgB > 0.55)

            var baseFontSize: CGFloat = 14.0
            var fontWeight: NSFont.Weight = .semibold
            var textColor: NSColor = .white

            switch region.category {
            case .headerTitle:
                baseFontSize = 18.0
                fontWeight = .bold
                textColor = .white
            case .headerSubtitle:
                baseFontSize = 13.0
                fontWeight = .regular
                textColor = NSColor(srgbRed: 0.68, green: 0.68, blue: 0.72, alpha: 1.0)
            case .listPrimary:
                baseFontSize = 14.0
                fontWeight = .semibold
                textColor = bgBrightness > 0.6 ? .black : .white
            case .listSecondary:
                baseFontSize = 13.0
                fontWeight = .regular
                textColor = isBlueBg ? .white : (bgBrightness > 0.6 ? NSColor(srgbRed: 0.35, green: 0.35, blue: 0.37, alpha: 1.0) : NSColor(srgbRed: 0.68, green: 0.68, blue: 0.72, alpha: 1.0))
            case .detailLabel:
                baseFontSize = 12.0
                fontWeight = .medium
                textColor = bgBrightness > 0.6 ? .black : NSColor(srgbRed: 0.85, green: 0.85, blue: 0.88, alpha: 1.0)
            }

            // Derive UNIFORM font size from category median line height (Line 1 primary title > Line 2 secondary IP detail)
            var fontScaleMultiplier: CGFloat = 0.86
            if region.category == .listSecondary {
                fontScaleMultiplier = 0.82
            }

            let effectiveLineH = medianLineHeights[region.category] ?? region.lineH
            let fontSizeFromLineH = effectiveLineH * fontScaleMultiplier
            let scaledFontSize = max(11.0, fontSizeFromLineH)

            // Snap left X position to unified main list column margin if within 25px (guarantees perfect vertical bullet alignment across both lines)
            var drawX = r.minX
            if (region.category == .listPrimary || region.category == .listSecondary),
               let targetX = unifiedListX, abs(r.minX - targetX) < 25.0 {
                drawX = targetX
            }

            // Font selection: monospaced SF Mono for IPs/MACs/tokens; proportional SF Pro for UI names/labels
            var font: NSFont
            if region.type == "ipv4" || region.type == "mac" || region.type == "token" {
                font = NSFont.monospacedSystemFont(ofSize: scaledFontSize, weight: fontWeight)
            } else {
                font = NSFont.systemFont(ofSize: scaledFontSize, weight: fontWeight)
            }

            // Measure unkerned width
            let unkernedAttr = NSAttributedString(string: region.replacement, attributes: [.font: font])
            let naturalWidth = unkernedAttr.size().width
            let targetWidth = region.rect.width

            // Compute subtle letter-spacing (kern) clamped to [-0.8pt, +0.8pt] to match original bounding box length exactly
            var kernValue: CGFloat = 0.0
            if region.replacement.count > 1 && abs(targetWidth - naturalWidth) > 0.5 {
                let charCount = CGFloat(region.replacement.count - 1)
                let diff = targetWidth - naturalWidth
                let calcKern = diff / charCount
                kernValue = max(-0.8, min(0.8, calcKern))
            }

            let attributes: [NSAttributedString.Key: Any] = [
                .font: font,
                .foregroundColor: textColor,
                .kern: kernValue as NSNumber
            ]
            let attrStr = NSAttributedString(string: region.replacement, attributes: attributes)
            let strSize = attrStr.size()

            // Exact font baseline alignment in pixel space with snapped X column margin
            let textY = region.lineY + (region.lineH - strSize.height) / 2.0
            let drawPoint = CGPoint(x: drawX, y: textY)
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
