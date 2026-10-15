import CoreGraphics
import Foundation
import ImageIO
import UniformTypeIdentifiers

// Run from the repository root:
// swift -module-cache-path /tmp/passpine-icon-module-cache client/assets/generate.swift
// The bundle artwork is the single source for browser and native branding.
let repository = URL(fileURLWithPath: #filePath)
    .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
let sourceURL = repository.appendingPathComponent("client/assets/Passpine.icns")
guard let source = CGImageSourceCreateWithURL(sourceURL as CFURL, nil) else {
    fatalError("Cannot load application icon")
}
let artwork = (0..<CGImageSourceGetCount(source)).compactMap {
    CGImageSourceCreateImageAtIndex(source, $0, nil)
}.max { $0.width < $1.width }
guard let artwork else { fatalError("Application icon has no image representation") }

func context(_ size: Int) -> CGContext {
    guard let result = CGContext(data: nil, width: size, height: size,
                                bitsPerComponent: 8, bytesPerRow: size * 4,
                                space: CGColorSpaceCreateDeviceRGB(),
                                bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue) else {
        fatalError("Cannot create icon bitmap")
    }
    result.interpolationQuality = .high
    return result
}

func write(_ image: CGImage, to path: String) {
    let destinationURL = repository.appendingPathComponent(path)
    try! FileManager.default.createDirectory(at: destinationURL.deletingLastPathComponent(),
                                           withIntermediateDirectories: true)
    guard let destination = CGImageDestinationCreateWithURL(
        destinationURL as CFURL, UTType.png.identifier as CFString, 1, nil
    ) else { fatalError("Cannot create icon output") }
    CGImageDestinationAddImage(destination, image, nil)
    guard CGImageDestinationFinalize(destination) else { fatalError("Cannot encode icon output") }
    print(path)
}

for size in [16, 32, 48, 128, 256] {
    let bitmap = context(size)
    bitmap.draw(artwork, in: CGRect(x: 0, y: 0, width: size, height: size))
    guard let image = bitmap.makeImage() else { fatalError("Cannot resize application icon") }
    if size == 256 {
        write(image, to: "client/assets/app-icon.png")
        write(image, to: "browser/src/assets/app-icon.png")
    } else {
        write(image, to: "browser/public/icon\(size).png")
    }
}

// Match the original native client's template extraction: keep the green
// leaves and stem, remove the tile, then center their existing silhouette.
let size = artwork.width
let bitmap = context(size)
bitmap.draw(artwork, in: CGRect(x: 0, y: 0, width: size, height: size))
guard let data = bitmap.data else { fatalError("Cannot access application pixels") }
let pixels = data.bindMemory(to: UInt8.self, capacity: size * size * 4)
var bounds = CGRect.null
for y in 0..<size {
    for x in 0..<size {
        let offset = (y * size + x) * 4
        let dominance = Int(pixels[offset + 1]) - max(Int(pixels[offset]), Int(pixels[offset + 2]))
        let mask = max(0, min(255, (dominance - 8) * 6)) * Int(pixels[offset + 3]) / 255
        pixels[offset] = 0
        pixels[offset + 1] = 0
        pixels[offset + 2] = 0
        pixels[offset + 3] = UInt8(mask)
        if mask > 12 { bounds = bounds.union(CGRect(x: x, y: y, width: 1, height: 1)) }
    }
}
guard !bounds.isNull, let foreground = bitmap.makeImage()?.cropping(to: bounds) else {
    fatalError("Cannot extract application mark")
}
let template = context(32)
let scale = min(28 / CGFloat(foreground.width), 28 / CGFloat(foreground.height))
let width = CGFloat(foreground.width) * scale
let height = CGFloat(foreground.height) * scale
template.draw(foreground, in: CGRect(x: (32 - width) / 2, y: (32 - height) / 2,
                                    width: width, height: height))
guard let templateImage = template.makeImage() else { fatalError("Cannot create menu bar icon") }
write(templateImage, to: "client/assets/menu-bar-template.png")
