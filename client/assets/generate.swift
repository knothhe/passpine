import CoreGraphics
import Foundation
import ImageIO
import UniformTypeIdentifiers

// Canonical vector artwork for the native app and browser extension.
// Run from the repository root:
// swift -module-cache-path /tmp/passpine-icon-module-cache client/assets/generate.swift
let repository = URL(fileURLWithPath: #filePath)
    .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
let accent = "#bd481b"
let paper = "#fcfbf9"
let canvas: CGFloat = 1024

struct Palette {
    let tile: String
    let ink: String
}

let applicationPalette = Palette(tile: paper, ink: accent)
let monochromePalette = Palette(tile: "#ffffff", ink: "#1c1c21")
let toolbarPalette = Palette(tile: accent, ink: "#ffffff")
let monochromeToolbarPalette = Palette(tile: "#1c1c21", ink: "#ffffff")

// Keep path commands shared by the bitmap renderer and the SVG export.
enum Command {
    case move(CGFloat, CGFloat)
    case line(CGFloat, CGFloat)
    case curve(CGFloat, CGFloat, CGFloat, CGFloat, CGFloat, CGFloat)
    case close
}

struct Shape {
    let commands: [Command]

    var path: CGPath {
        let path = CGMutablePath()
        for command in commands {
            switch command {
            case let .move(x, y): path.move(to: CGPoint(x: x, y: y))
            case let .line(x, y): path.addLine(to: CGPoint(x: x, y: y))
            case let .curve(x1, y1, x2, y2, x, y):
                path.addCurve(to: CGPoint(x: x, y: y),
                              control1: CGPoint(x: x1, y: y1),
                              control2: CGPoint(x: x2, y: y2))
            case .close: path.closeSubpath()
            }
        }
        return path
    }

    var svg: String {
        commands.map { command in
            switch command {
            case let .move(x, y): return "M\(Int(x)) \(Int(y))"
            case let .line(x, y): return "L\(Int(x)) \(Int(y))"
            case let .curve(x1, y1, x2, y2, x, y):
                return "C\(Int(x1)) \(Int(y1)) \(Int(x2)) \(Int(y2)) \(Int(x)) \(Int(y))"
            case .close: return "Z"
            }
        }.joined(separator: " ")
    }
}

let stem = Shape(commands: [.move(402, 613), .line(695, 320)])
let leaves = [
    Shape(commands: [.move(467, 548), .curve(371, 536, 344, 446, 366, 382),
                     .curve(444, 398, 489, 466, 467, 548), .close]),
    Shape(commands: [.move(501, 514), .curve(545, 441, 632, 430, 689, 458),
                     .curve(657, 533, 571, 558, 501, 514), .close]),
    Shape(commands: [.move(563, 451), .curve(478, 428, 464, 346, 490, 288),
                     .curve(564, 315, 594, 379, 563, 451), .close]),
    Shape(commands: [.move(599, 415), .curve(635, 347, 707, 334, 757, 358),
                     .curve(731, 425, 660, 453, 599, 415), .close]),
    Shape(commands: [.move(658, 357), .curve(631, 281, 684, 226, 746, 219),
                     .curve(766, 286, 727, 345, 658, 357), .close]),
]
// At 16px, three larger leaves preserve the branch rhythm and open spaces.
let smallLeaves = [
    Shape(commands: [.move(489, 529), .curve(366, 507, 351, 392, 382, 334),
                     .curve(469, 358, 513, 439, 489, 529), .close]),
    Shape(commands: [.move(543, 475), .curve(585, 388, 690, 374, 754, 404),
                     .curve(714, 490, 623, 523, 543, 475), .close]),
    leaves[4],
]
let bow = CGRect(x: 275, y: 600, width: 140, height: 140)
let markBounds = CGRect(x: 249, y: 219, width: 510, height: 547)

func color(_ hex: String) -> CGColor {
    let value = UInt32(hex.dropFirst(), radix: 16)!
    return CGColor(srgbRed: CGFloat((value >> 16) & 255) / 255,
                   green: CGFloat((value >> 8) & 255) / 255,
                   blue: CGFloat(value & 255) / 255, alpha: 1)
}

func context(_ size: Int) -> CGContext {
    guard let result = CGContext(data: nil, width: size, height: size,
                                bitsPerComponent: 8, bytesPerRow: size * 4,
                                space: CGColorSpace(name: CGColorSpace.sRGB)!,
                                bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue) else {
        fatalError("Cannot create icon bitmap")
    }
    result.interpolationQuality = .high
    return result
}

func drawMark(_ bitmap: CGContext, small: Bool, ink: CGColor) {
    bitmap.setFillColor(ink)
    bitmap.setStrokeColor(ink)
    bitmap.setLineWidth(small ? 60 : 52)
    bitmap.setLineCap(.round)
    bitmap.addPath(stem.path)
    bitmap.strokePath()
    bitmap.strokeEllipse(in: bow)
    for leaf in small ? smallLeaves : leaves {
        bitmap.addPath(leaf.path)
        bitmap.fillPath()
    }
}

enum IconUsage {
    case application, toolbar, menuBar
}

func render(_ size: Int, usage: IconUsage = .application, palette: Palette = applicationPalette) -> CGImage {
    let sampling = size <= 128 ? 4 : 1
    let bitmap = context(size * sampling)
    bitmap.translateBy(x: 0, y: CGFloat(size * sampling))
    bitmap.scaleBy(x: CGFloat(size * sampling) / canvas,
                   y: -CGFloat(size * sampling) / canvas)
    if usage == .menuBar {
        // Use geometry directly; the menu bar symbol is independent of tile colors.
        let scale = 896 / max(markBounds.width, markBounds.height)
        bitmap.translateBy(x: (canvas - markBounds.width * scale) / 2,
                           y: (canvas - markBounds.height * scale) / 2)
        bitmap.scaleBy(x: scale, y: scale)
        bitmap.translateBy(x: -markBounds.minX, y: -markBounds.minY)
        drawMark(bitmap, small: true, ink: color("#000000"))
    } else {
        // Application artwork has macOS canvas padding. Browser action icons
        // already sit inside a toolbar button, so their tile fills the bitmap.
        let inset: CGFloat = usage == .toolbar ? 0 : 96
        let corner: CGFloat = usage == .toolbar ? 224 : 184
        bitmap.setFillColor(color(palette.tile))
        bitmap.addPath(CGPath(roundedRect: CGRect(x: inset, y: inset,
                                                width: canvas - 2 * inset, height: canvas - 2 * inset),
                              cornerWidth: corner, cornerHeight: corner, transform: nil))
        bitmap.fillPath()
        if usage == .toolbar {
            // Give the mark 72% of the canvas height and center its ink bounds.
            let scale = 736 / max(markBounds.width, markBounds.height)
            bitmap.translateBy(x: (canvas - markBounds.width * scale) / 2,
                               y: (canvas - markBounds.height * scale) / 2)
            bitmap.scaleBy(x: scale, y: scale)
            bitmap.translateBy(x: -markBounds.minX, y: -markBounds.minY)
        }
        // 32px is also the Retina representation of a 16px browser action.
        let small = size <= (usage == .toolbar ? 32 : 16)
        drawMark(bitmap, small: small, ink: color(palette.ink))
    }
    guard let image = bitmap.makeImage() else { fatalError("Cannot render application icon") }
    if sampling == 1 { return image }
    let output = context(size)
    output.draw(image, in: CGRect(x: 0, y: 0, width: size, height: size))
    guard let resized = output.makeImage() else { fatalError("Cannot resize application icon") }
    return resized
}

func png(_ image: CGImage) -> Data {
    let data = NSMutableData()
    guard let destination = CGImageDestinationCreateWithData(
        data as CFMutableData, UTType.png.identifier as CFString, 1, nil
    ) else { fatalError("Cannot create icon output") }
    CGImageDestinationAddImage(destination, image, nil)
    guard CGImageDestinationFinalize(destination) else { fatalError("Cannot encode icon output") }
    return data as Data
}

func export(_ image: CGImage, to path: String) {
    try! png(image).write(to: repository.appendingPathComponent(path), options: .atomic)
    print(path)
}

func exportSVG(_ palette: Palette, suffix: String) throws {
    let svg = """
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1024 1024" fill="none">
  <title>Passpine — botanical key</title>
  <rect x="96" y="96" width="832" height="832" rx="184" fill="\(palette.tile)"/>
  <g fill="\(palette.ink)">
\(leaves.map { "    <path d=\"\($0.svg)\"/>" }.joined(separator: "\n"))
  </g>
  <g stroke="\(palette.ink)" stroke-width="52" stroke-linecap="round">
    <path d="\(stem.svg)"/>
    <circle cx="345" cy="670" r="70"/>
  </g>
</svg>

"""
    let path = "client/assets/app-icon\(suffix).svg"
    try svg.write(to: repository.appendingPathComponent(path), atomically: true, encoding: .utf8)
    print(path)
}

for (palette, toolbar, suffix) in [(applicationPalette, toolbarPalette, ""),
                                  (monochromePalette, monochromeToolbarPalette, "-monochrome")] {
    try exportSVG(palette, suffix: suffix)
    let image = render(256, palette: palette)
    export(image, to: "client/assets/app-icon\(suffix).png")
    export(image, to: "browser/src/assets/app-icon\(suffix).png")
    for size in [16, 32, 48, 128] {
        export(render(size, usage: .toolbar, palette: toolbar), to: "browser/public/icon\(size)\(suffix).png")
    }
}
export(render(32, usage: .menuBar), to: "client/assets/menu-bar-template.png")

// ICNS stores PNG representations in typed, big-endian length-prefixed chunks.
// Include every standard and Retina size without an external conversion tool.
func chunk(_ type: String, payload: Data) -> Data {
    var data = Data(type.utf8)
    var length = UInt32(payload.count + 8).bigEndian
    withUnsafeBytes(of: &length) { data.append(contentsOf: $0) }
    data.append(payload)
    return data
}
let representations = [("icp4", 16), ("icp5", 32), ("ic07", 128),
                       ("ic08", 256), ("ic09", 512), ("ic11", 32),
                       ("ic12", 64), ("ic13", 256), ("ic14", 512), ("ic10", 1024)]
for (palette, suffix) in [(applicationPalette, ""), (monochromePalette, "-monochrome")] {
    var images: [Int: Data] = [:]
    var payload = Data()
    for (type, size) in representations {
        let data = images[size] ?? png(render(size, palette: palette))
        images[size] = data
        payload.append(chunk(type, payload: data))
    }
    let path = "client/assets/Passpine\(suffix).icns"
    try chunk("icns", payload: payload).write(to: repository.appendingPathComponent(path), options: .atomic)
    print(path)
}
