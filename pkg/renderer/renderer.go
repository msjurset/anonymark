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
import CoreText

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

struct RGBAPixel {
    let r: CGFloat
    let g: CGFloat
    let b: CGFloat
    let a: CGFloat

    func distance(to other: RGBAPixel) -> CGFloat {
        let dr = r - other.r
        let dg = g - other.g
        let db = b - other.b
        return sqrt(dr * dr + dg * dg + db * db)
    }

    var nsColor: NSColor {
        NSColor(srgbRed: r, green: g, blue: b, alpha: a)
    }
}

func readPixel(context: CGContext, x: Int, y: Int, width: Int, height: Int) -> RGBAPixel {
    guard let dataPtr = context.data else {
        return RGBAPixel(r: 0, g: 0, b: 0, a: 1)
    }
    let pointer = dataPtr.bindMemory(to: UInt8.self, capacity: width * height * 4)
    let safeX = max(0, min(width - 1, x))
    let safeY = max(0, min(height - 1, (height - 1) - y))
    let offset = (safeY * width + safeX) * 4
    return RGBAPixel(
        r: CGFloat(pointer[offset]) / 255.0,
        g: CGFloat(pointer[offset + 1]) / 255.0,
        b: CGFloat(pointer[offset + 2]) / 255.0,
        a: CGFloat(pointer[offset + 3]) / 255.0
    )
}

func sampleBackgroundPixel(context: CGContext, rect: CGRect, width: Int, height: Int) -> RGBAPixel {
    let minX = max(0, min(width - 1, Int(rect.minX) - 2))
    let maxX = max(0, min(width - 1, Int(rect.maxX) + 2))
    let minY = max(0, min(height - 1, Int(rect.minY) - 2))
    let maxY = max(0, min(height - 1, Int(rect.maxY) + 2))

    var samples: [RGBAPixel] = []
    let stepX = max(1, (maxX - minX) / 16)
    let stepY = max(1, (maxY - minY) / 6)

    for x in stride(from: minX, through: maxX, by: stepX) {
        samples.append(readPixel(context: context, x: x, y: maxY, width: width, height: height))
        samples.append(readPixel(context: context, x: x, y: minY, width: width, height: height))
    }
    for y in stride(from: minY, through: maxY, by: stepY) {
        samples.append(readPixel(context: context, x: minX, y: y, width: width, height: height))
        samples.append(readPixel(context: context, x: maxX, y: y, width: width, height: height))
    }

    if samples.isEmpty {
        return RGBAPixel(r: 0.15, g: 0.15, b: 0.16, a: 1.0)
    }

    var bestCluster: [RGBAPixel] = []
    for s1 in samples {
        let cluster = samples.filter { s1.distance(to: $0) < 0.06 }
        if cluster.count > bestCluster.count {
            bestCluster = cluster
        }
    }
    let chosen = bestCluster.isEmpty ? samples : bestCluster
    let count = CGFloat(chosen.count)
    return RGBAPixel(
        r: chosen.reduce(0) { $0 + $1.r } / count,
        g: chosen.reduce(0) { $0 + $1.g } / count,
        b: chosen.reduce(0) { $0 + $1.b } / count,
        a: 1.0
    )
}

struct InkMetrics {
    let bgPixel: RGBAPixel
    let fgPixel: RGBAPixel
    let inkMinX: CGFloat
    let inkMaxX: CGFloat
    let inkMinY: CGFloat
    let inkMaxY: CGFloat
    let inkCoverage: CGFloat
}

func measureInkMetrics(context: CGContext, rect: CGRect, width: Int, height: Int) -> InkMetrics {
    let bg = sampleBackgroundPixel(context: context, rect: rect, width: width, height: height)
    let minX = max(0, min(width - 1, Int(floor(rect.minX))))
    let maxX = max(0, min(width - 1, Int(ceil(rect.maxX))))
    let minY = max(0, min(height - 1, Int(floor(rect.minY))))
    let maxY = max(0, min(height - 1, Int(ceil(rect.maxY))))

    var maxDist: CGFloat = 0
    for y in minY...maxY {
        for x in minX...maxX {
            let p = readPixel(context: context, x: x, y: y, width: width, height: height)
            let d = p.distance(to: bg)
            if d > maxDist {
                maxDist = d
            }
        }
    }

    let inkThreshold = max(0.12, maxDist * 0.38)
    let coreInkThreshold = max(0.18, maxDist * 0.68)

    var iMinX = maxX
    var iMaxX = minX
    var iMinY = maxY
    var iMaxY = minY
    var inkCount = 0
    var corePixels: [RGBAPixel] = []

    for y in minY...maxY {
        for x in minX...maxX {
            let p = readPixel(context: context, x: x, y: y, width: width, height: height)
            let d = p.distance(to: bg)
            if d >= inkThreshold {
                if x < iMinX { iMinX = x }
                if x > iMaxX { iMaxX = x }
                if y < iMinY { iMinY = y }
                if y > iMaxY { iMaxY = y }
                inkCount += 1
                if d >= coreInkThreshold {
                    corePixels.append(p)
                }
            }
        }
    }

    if inkCount == 0 {
        let fallbackFg = (bg.r + bg.g + bg.b) / 3.0 > 0.55
            ? RGBAPixel(r: 0.1, g: 0.1, b: 0.1, a: 1.0)
            : RGBAPixel(r: 0.92, g: 0.92, b: 0.94, a: 1.0)
        return InkMetrics(
            bgPixel: bg,
            fgPixel: fallbackFg,
            inkMinX: rect.minX,
            inkMaxX: rect.maxX,
            inkMinY: rect.minY,
            inkMaxY: rect.maxY,
            inkCoverage: 0.15
        )
    }

    let sortedR = corePixels.map { $0.r }.sorted()
    let sortedG = corePixels.map { $0.g }.sorted()
    let sortedB = corePixels.map { $0.b }.sorted()
    let fg: RGBAPixel
    if !sortedR.isEmpty {
        let mid = sortedR.count / 2
        fg = RGBAPixel(r: sortedR[mid], g: sortedG[mid], b: sortedB[mid], a: 1.0)
    } else {
        fg = (bg.r + bg.g + bg.b) / 3.0 > 0.55
            ? RGBAPixel(r: 0.1, g: 0.1, b: 0.1, a: 1.0)
            : RGBAPixel(r: 0.92, g: 0.92, b: 0.94, a: 1.0)
    }

    let area = max(1.0, CGFloat(( iMaxX - iMinX + 1 ) * ( iMaxY - iMinY + 1 )))
    return InkMetrics(
        bgPixel: bg,
        fgPixel: fg,
        inkMinX: CGFloat(iMinX),
        inkMaxX: CGFloat(iMaxX),
        inkMinY: CGFloat(iMinY),
        inkMaxY: CGFloat(iMaxY),
        inkCoverage: CGFloat(inkCount) / area
    )
}

func isTokenBoundaryChar(_ ch: Character) -> Bool {
    return !(ch.isLetter || ch.isNumber || ch == "-" || ch == "_")
}

func hasDescenders(_ s: String) -> Bool {
    let descenderSet: Set<Character> = ["g", "j", "p", "q", "y", "(", ")", "[", "]", "{", "}", ",", ";"]
    return s.contains { descenderSet.contains($0) }
}

struct ObservationEdit {
    let originalText: String
    let replacedText: String
    let obsRect: CGRect
    var metrics: InkMetrics
    var pointSize: CGFloat
    var fontWeight: NSFont.Weight
    var isMonospaced: Bool
    var snappedMinX: CGFloat
}

let requestHandler = VNImageRequestHandler(cgImage: cgImage, options: [:])
let request = VNRecognizeTextRequest { request, error in
    guard let observations = request.results as? [VNRecognizedTextObservation] else { return }

    var edits: [ObservationEdit] = []

    for obs in observations {
        guard let topCandidate = obs.topCandidates(1).first else { continue }
        let originalText = topCandidate.string

        // Find all non-overlapping matches of targets within originalText (targets are already sorted longest-first)
        var matchedRanges: [(range: Range<String.Index>, target: TargetItem)] = []

        for t in targets {
            var searchStart = originalText.startIndex
            while searchStart < originalText.endIndex,
                  let range = originalText.range(of: t.original, options: [.caseInsensitive], range: searchStart..<originalText.endIndex) {
                defer { searchStart = range.upperBound }

                // Check token boundaries so a shorter hostname doesn't split a longer hostname or word
                if range.lowerBound > originalText.startIndex {
                    let prev = originalText[originalText.index(before: range.lowerBound)]
                    if !isTokenBoundaryChar(prev) {
                        continue
                    }
                }
                if range.upperBound < originalText.endIndex {
                    let next = originalText[range.upperBound]
                    if !isTokenBoundaryChar(next) {
                        continue
                    }
                }

                // Ensure no overlap with an already claimed longer match
                let overlaps = matchedRanges.contains { existing in
                    existing.range.overlaps(range)
                }
                if !overlaps {
                    matchedRanges.append((range: range, target: t))
                }
            }
        }

        if matchedRanges.isEmpty {
            continue
        }

        // Sort matched ranges in forward string order and assemble the complete anonymized observation string
        matchedRanges.sort { $0.range.lowerBound < $1.range.lowerBound }

        var replacedText = ""
        var cursor = originalText.startIndex
        for m in matchedRanges {
            replacedText += String(originalText[cursor..<m.range.lowerBound])
            replacedText += m.target.replacement
            cursor = m.range.upperBound
        }
        replacedText += String(originalText[cursor..<originalText.endIndex])

        let obsRect = CGRect(
            x: obs.boundingBox.origin.x * CGFloat(width),
            y: obs.boundingBox.origin.y * CGFloat(height),
            width: obs.boundingBox.width * CGFloat(width),
            height: obs.boundingBox.height * CGFloat(height)
        )

        let metrics = measureInkMetrics(context: context, rect: obsRect, width: width, height: height)
        let inkHeight = max(8.0, metrics.inkMaxY - metrics.inkMinY + 1.0)
        let inkWidth = max(8.0, metrics.inkMaxX - metrics.inkMinX + 1.0)

        // Compute exact point size from measured glyph ink height
        // Reference SF Pro capHeight ratio is ~0.705; with descenders total ink height ratio is ~0.91
        let heightRatio: CGFloat = hasDescenders(originalText) ? 0.89 : 0.715
        let estimatedPointSize = max(10.0, min(44.0, inkHeight / heightRatio))

        // Determine best matching font family & weight by comparing rendered width of originalText against measured inkWidth
        let candidateSpecs: [(mono: Bool, weight: NSFont.Weight)] = [
            (false, .regular),
            (false, .medium),
            (false, .semibold),
            (false, .bold),
            (true, .regular),
            (true, .medium)
        ]

        let onlyTechnicalToken = matchedRanges.count == 1 &&
            matchedRanges[0].range == (originalText.startIndex..<originalText.endIndex) &&
            (matchedRanges[0].target.type == "mac" || matchedRanges[0].target.type == "token")

        var bestSpec = (mono: onlyTechnicalToken, weight: NSFont.Weight.regular)
        var bestScore = CGFloat.greatestFiniteMagnitude

        for spec in candidateSpecs {
            if spec.mono && !onlyTechnicalToken {
                continue
            }
            let testFont = spec.mono
                ? NSFont.monospacedSystemFont(ofSize: estimatedPointSize, weight: spec.weight)
                : NSFont.systemFont(ofSize: estimatedPointSize, weight: spec.weight)
            let attr = NSAttributedString(string: originalText, attributes: [.font: testFont])
            let w = attr.size().width
            let widthError = abs(w - inkWidth) / max(1.0, inkWidth)

            // Prefer semibold/bold when inkCoverage is higher or when width matches closely
            var weightPenalty: CGFloat = 0.0
            if metrics.inkCoverage > 0.24 && (spec.weight == .regular) {
                weightPenalty = 0.04
            } else if metrics.inkCoverage < 0.19 && (spec.weight == .bold) {
                weightPenalty = 0.05
            }

            let score = widthError + weightPenalty
            if score < bestScore {
                bestScore = score
                bestSpec = spec
            }
        }

        edits.append(ObservationEdit(
            originalText: originalText,
            replacedText: replacedText,
            obsRect: obsRect,
            metrics: metrics,
            pointSize: estimatedPointSize,
            fontWeight: bestSpec.weight,
            isMonospaced: bestSpec.mono,
            snappedMinX: metrics.inkMinX
        ))
    }

    // Align left X and pointSize across vertically aligned rows in the same column
    for i in 0..<edits.count {
        var colX: [CGFloat] = [edits[i].metrics.inkMinX]
        var colSizes: [CGFloat] = [edits[i].pointSize]
        for j in 0..<edits.count where i != j {
            if abs(edits[i].metrics.inkMinX - edits[j].metrics.inkMinX) <= 8.0 {
                colX.append(edits[j].metrics.inkMinX)
                if abs(edits[i].pointSize - edits[j].pointSize) <= 2.5 {
                    colSizes.append(edits[j].pointSize)
                }
            }
        }
        colX.sort()
        edits[i].snappedMinX = colX[colX.count / 2]
        colSizes.sort()
        edits[i].pointSize = colSizes[colSizes.count / 2]
    }

    NSGraphicsContext.saveGraphicsState()
    let nsContext = NSGraphicsContext(cgContext: context, flipped: false)
    NSGraphicsContext.current = nsContext

    // PASS 1: Erase the full observation ink/bounding box cleanly with sampled background color
    for edit in edits {
        edit.metrics.bgPixel.nsColor.setFill()
        let eraseMinX = min(edit.obsRect.minX, edit.metrics.inkMinX) - 2.0
        let eraseMaxX = max(edit.obsRect.maxX, edit.metrics.inkMaxX) + 3.0
        let eraseMinY = min(edit.obsRect.minY, edit.metrics.inkMinY) - 2.0
        let eraseMaxY = max(edit.obsRect.maxY, edit.metrics.inkMaxY) + 2.0
        let eraseRect = CGRect(
            x: eraseMinX,
            y: eraseMinY,
            width: eraseMaxX - eraseMinX,
            height: eraseMaxY - eraseMinY
        )
        NSBezierPath.fill(eraseRect)

        if mode == "blur" {
            NSColor.gray.withAlphaComponent(0.8).setFill()
            NSBezierPath.fill(edit.obsRect)
        } else if mode == "pill" {
            NSColor.darkGray.setFill()
            NSBezierPath.fill(edit.obsRect)
        }
    }

    // PASS 2: Draw the complete anonymized observation line on the exact original typographic baseline using CoreText
    if mode == "synthetic" {
        for edit in edits {
            let font: NSFont = edit.isMonospaced
                ? NSFont.monospacedSystemFont(ofSize: edit.pointSize, weight: edit.fontWeight)
                : NSFont.systemFont(ofSize: edit.pointSize, weight: edit.fontWeight)

            let attributes: [NSAttributedString.Key: Any] = [
                .font: font,
                .foregroundColor: edit.metrics.fgPixel.nsColor
            ]
            let attrStr = NSAttributedString(string: edit.replacedText, attributes: attributes)
            let ctLine = CTLineCreateWithAttributedString(attrStr)

            // Calculate exact baseline Y from the top of the measured ink capHeight
            let baselineY: CGFloat
            if hasDescenders(edit.originalText) {
                baselineY = edit.metrics.inkMaxY - font.capHeight
            } else {
                baselineY = edit.metrics.inkMinY
            }

            context.saveGState()
            context.textMatrix = .identity
            context.textPosition = CGPoint(x: edit.snappedMinX, y: baselineY)
            CTLineDraw(ctLine, context)
            context.restoreGState()
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
