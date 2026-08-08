package renderer

import (
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/msjurset/anonymark/pkg/ocr"
)

// Mode defines the visual redaction style.
type Mode string

const (
	ModeSynthetic Mode = "synthetic"
	ModeBlur      Mode = "blur"
	ModePill      Mode = "pill"
)

// RedactionItem specifies a target region and its replacement text.
type RedactionItem struct {
	X           int    `json:"x"`
	Y           int    `json:"y"`
	W           int    `json:"w"`
	H           int    `json:"h"`
	Replacement string `json:"replacement"`
}

// AppKitRenderer uses native macOS CoreGraphics and CoreText APIs for pixel-perfect font rendering.
type AppKitRenderer struct{}

// NewRenderer creates a new AppKitRenderer instance.
func NewRenderer() *AppKitRenderer {
	return &AppKitRenderer{}
}

// RenderNativeRedactions renders high-resolution text matching original bounding box heights and UI colors.
func (r *AppKitRenderer) RenderNativeRedactions(inputPath, outputPath string, items []RedactionItem, mode Mode) error {
	itemsJSON, err := json.Marshal(items)
	if err != nil {
		return fmt.Errorf("failed to marshal redaction items: %w", err)
	}

	script := fmt.Sprintf(`
import Foundation
import AppKit
import CoreGraphics

struct RedactionItem: Codable {
    let x: Int
    let y: Int
    let w: Int
    let h: Int
    let replacement: String
}

let inputPath = "%s"
let outputPath = "%s"
let mode = "%s"
let itemsJSON = """
%s
"""

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

// Draw original image into bitmap context
let rect = CGRect(x: 0, y: 0, width: width, height: height)
context.draw(cgImage, in: rect)

guard let data = itemsJSON.data(using: .utf8),
      let items = try? JSONDecoder().decode([RedactionItem].self, from: data) else {
    print("Error decoding items")
    exit(1)
}

func samplePerimeterColor(context: CGContext, rect: CGRect) -> NSColor {
    let minX = Int(max(0, rect.minX - 2))
    let maxX = Int(min(CGFloat(width - 1), rect.maxX + 2))
    let minY = Int(max(0, rect.minY - 2))
    let maxY = Int(min(CGFloat(height - 1), rect.maxY + 2))

    guard let dataPtr = context.data else { return NSColor.black }
    let pointer = dataPtr.bindMemory(to: UInt8.self, capacity: width * height * 4)

    var rSum = 0, gSum = 0, bSum = 0, count = 0

    let samplePixel = { (x: Int, y: Int) in
        let offset = (y * width + x) * 4
        rSum += Int(pointer[offset])
        gSum += Int(pointer[offset + 1])
        bSum += Int(pointer[offset + 2])
        count += 1
    }

    for x in minX...maxX {
        samplePixel(x, minY)
        samplePixel(x, maxY)
    }
    for y in minY...maxY {
        samplePixel(minX, y)
        samplePixel(maxX, y)
    }

    if count == 0 { return NSColor.black }
    return NSColor(srgbRed: CGFloat(rSum/count)/255.0,
                   green: CGFloat(gSum/count)/255.0,
                   blue: CGFloat(bSum/count)/255.0,
                   alpha: 1.0)
}

func sampleForegroundColor(context: CGContext, rect: CGRect, bg: NSColor) -> NSColor {
    guard let dataPtr = context.data else { return NSColor.white }
    let pointer = dataPtr.bindMemory(to: UInt8.self, capacity: width * height * 4)

    var bgR: CGFloat = 0, bgG: CGFloat = 0, bgB: CGFloat = 0, bgA: CGFloat = 0
    bg.getRed(&bgR, green: &bgG, blue: &bgB, alpha: &bgA)

    var maxDist: CGFloat = -1.0
    var bestColor = NSColor.white

    let minX = Int(rect.minX)
    let maxX = Int(rect.maxX)
    let minY = Int(rect.minY)
    let maxY = Int(rect.maxY)

    for y in minY...maxY {
        for x in minX...maxX {
            let offset = (y * width + x) * 4
            let r = CGFloat(pointer[offset]) / 255.0
            let g = CGFloat(pointer[offset + 1]) / 255.0
            let b = CGFloat(pointer[offset + 2]) / 255.0

            let dist = sqrt(pow(r - bgR, 2) + pow(g - bgG, 2) + pow(b - bgB, 2))
            if dist > maxDist {
                maxDist = dist
                bestColor = NSColor(srgbRed: r, green: g, blue: b, alpha: 1.0)
            }
        }
    }

    if maxDist < 0.2 {
        let brightness = (bgR + bgG + bgB) / 3.0
        return brightness > 0.5 ? NSColor.black : NSColor.white
    }
    return bestColor
}

NSGraphicsContext.saveGraphicsState()
let nsContext = NSGraphicsContext(cgContext: context, flipped: false)
NSGraphicsContext.current = nsContext

for item in items {
    // Note: CoreGraphics origin is bottom-left, so flip Y axis
    let flipY = height - item.y - item.h
    let itemRect = CGRect(x: item.x, y: flipY, width: item.w, height: item.h)

    let bgColor = samplePerimeterColor(context: context, rect: itemRect)
    let fgColor = sampleForegroundColor(context: context, rect: itemRect, bg: bgColor)

    if mode == "synthetic" {
        // Erase background cleanly
        bgColor.setFill()
        let eraseRect = itemRect.insetBy(dx: -1, dy: -1)
        NSBezierPath.fill(eraseRect)

        // Choose SF Pro system font with height matching bounding box
        let fontSize = max(10.0, CGFloat(item.h) * 0.72)
        let font = NSFont.systemFont(ofSize: fontSize, weight: .regular)

        let attributes: [NSAttributedString.Key: Any] = [
            .font: font,
            .foregroundColor: fgColor
        ]

        let attrStr = NSAttributedString(string: item.replacement, attributes: attributes)

        // Center vertically inside bounding box
        let strSize = attrStr.size()
        let textY = itemRect.minY + (itemRect.height - strSize.height) / 2.0
        let textRect = CGRect(x: itemRect.minX, y: textY, width: strSize.width + 10, height: strSize.height)

        attrStr.draw(in: textRect)
    } else if mode == "blur" {
        NSColor.gray.withAlphaComponent(0.8).setFill()
        NSBezierPath.fill(itemRect)
    } else {
        NSColor.darkGray.setFill()
        NSBezierPath.fill(itemRect)
    }
}

NSGraphicsContext.restoreGraphicsState()

guard let outputCGImage = context.makeImage() else {
    print("Error creating output image")
    exit(1)
}

let imageRep = NSBitmapImageRep(cgImage: outputCGImage)
guard let pngData = imageRep.representation(using: .png, properties: [:]) else {
    print("Error encoding PNG")
    exit(1)
}

try? pngData.write(to: URL(fileURLWithPath: outputPath))
`, inputPath, outputPath, string(mode), string(itemsJSON))

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

// RedactObservedText maps OCR observations to native AppKit redaction items.
func RedactObservedText(inputPath, outputPath string, observations []ocr.TextObservation, detectMatches func(string) []string, mode Mode) error {
	var items []RedactionItem

	for _, obs := range observations {
		repls := detectMatches(obs.Text)
		if len(repls) > 0 {
			items = append(items, RedactionItem{
				X:           obs.X,
				Y:           obs.Y,
				W:           obs.W,
				H:           obs.H,
				Replacement: repls[0],
			})
		}
	}

	r := NewRenderer()
	return r.RenderNativeRedactions(inputPath, outputPath, items, mode)
}
