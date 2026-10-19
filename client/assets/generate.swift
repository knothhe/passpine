import CoreGraphics
import Foundation
import ImageIO
import UniformTypeIdentifiers

// The approved squirrel vectors live in icon-artwork.json, independent of design previews.
// swift -module-cache-path /tmp/passpine-icon-module-cache client/assets/generate.swift
let repository = URL(fileURLWithPath: #filePath)
    .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
let canvas: CGFloat = 256

// SVG path support needed by the canonical artwork: M/L/H/V/C/A/Z, absolute or relative.
func vectorPath(_ source: String) -> CGPath {
    let regex = try! NSRegularExpression(pattern: "[a-zA-Z]|[-+]?(?:[0-9]*\\.)?[0-9]+(?:[eE][-+]?[0-9]+)?")
    let text = source as NSString
    let tokens = regex.matches(in: source, range: NSRange(location: 0, length: text.length)).map { text.substring(with: $0.range) }
    let result = CGMutablePath()
    var i = 0, command = "", current = CGPoint.zero, start = CGPoint.zero
    func number() -> CGFloat {
        precondition(i < tokens.count && Double(tokens[i]) != nil, "Invalid SVG path: \(source)")
        defer { i += 1 }
        return CGFloat(Double(tokens[i])!)
    }
    func arc(to end: CGPoint, rx inputRX: CGFloat, ry inputRY: CGFloat, rotation: CGFloat, large: Bool, sweep: Bool) {
        if end == current { return }
        var rx = abs(inputRX), ry = abs(inputRY)
        if rx == 0 || ry == 0 { result.addLine(to: end); return }
        let phi = rotation * .pi / 180, c = cos(phi), s = sin(phi)
        let dx = (current.x - end.x) / 2, dy = (current.y - end.y) / 2
        let xp = c * dx + s * dy, yp = -s * dx + c * dy
        let stretch = xp * xp / (rx * rx) + yp * yp / (ry * ry)
        if stretch > 1 { rx *= sqrt(stretch); ry *= sqrt(stretch) }
        let rx2 = rx * rx, ry2 = ry * ry
        let denominator = rx2 * yp * yp + ry2 * xp * xp
        let factor = (large == sweep ? -1.0 : 1.0) * sqrt(max(0, (rx2 * ry2 - denominator) / denominator))
        let cxp = factor * rx * yp / ry, cyp = -factor * ry * xp / rx
        let center = CGPoint(x: c * cxp - s * cyp + (current.x + end.x) / 2,
                             y: s * cxp + c * cyp + (current.y + end.y) / 2)
        let ux = (xp - cxp) / rx, uy = (yp - cyp) / ry
        let vx = (-xp - cxp) / rx, vy = (-yp - cyp) / ry
        let angle = atan2(uy, ux)
        var delta = atan2(ux * vy - uy * vx, ux * vx + uy * vy)
        if !sweep && delta > 0 { delta -= 2 * .pi }
        if sweep && delta < 0 { delta += 2 * .pi }
        let segments = max(1, Int(ceil(abs(delta) / (.pi / 2))))
        let step = delta / CGFloat(segments)
        func point(_ t: CGFloat) -> CGPoint {
            CGPoint(x: center.x + c * rx * cos(t) - s * ry * sin(t),
                    y: center.y + s * rx * cos(t) + c * ry * sin(t))
        }
        func tangent(_ t: CGFloat) -> CGPoint {
            CGPoint(x: -c * rx * sin(t) - s * ry * cos(t),
                    y: -s * rx * sin(t) + c * ry * cos(t))
        }
        for segment in 0..<segments {
            let a = angle + CGFloat(segment) * step, b = a + step
            let k = 4 / 3 * tan(step / 4)
            let p = point(a), q = point(b), u = tangent(a), v = tangent(b)
            result.addCurve(to: segment == segments - 1 ? end : q,
                            control1: CGPoint(x: p.x + k * u.x, y: p.y + k * u.y),
                            control2: CGPoint(x: q.x - k * v.x, y: q.y - k * v.y))
        }
    }
    while i < tokens.count {
        if tokens[i].first!.isLetter { command = tokens[i]; i += 1 }
        let relative = command == command.lowercased()
        let origin = relative ? current : .zero
        switch command.uppercased() {
        case "M", "L":
            let p = CGPoint(x: origin.x + number(), y: origin.y + number())
            if command.uppercased() == "M" {
                result.move(to: p); start = p; command = relative ? "l" : "L"
            } else { result.addLine(to: p) }
            current = p
        case "H": current.x = origin.x + number(); result.addLine(to: current)
        case "V": current.y = origin.y + number(); result.addLine(to: current)
        case "C":
            let a = CGPoint(x: origin.x + number(), y: origin.y + number())
            let b = CGPoint(x: origin.x + number(), y: origin.y + number())
            let end = CGPoint(x: origin.x + number(), y: origin.y + number())
            result.addCurve(to: end, control1: a, control2: b); current = end
        case "A":
            let rx = number(), ry = number(), rotation = number()
            let large = number() != 0, sweep = number() != 0
            let end = CGPoint(x: origin.x + number(), y: origin.y + number())
            arc(to: end, rx: rx, ry: ry, rotation: rotation, large: large, sweep: sweep); current = end
        case "Z": result.closeSubpath(); current = start; command = ""
        default: fatalError("Unsupported SVG path command: \(command)")
        }
    }
    return result
}

struct Shape: Decodable {
    let kind: String
    let d: String?
    let cx: CGFloat?, cy: CGFloat?, r: CGFloat?
    let strokeWidth: CGFloat?
    var path: CGPath {
        if kind == "circle" {
            return CGPath(ellipseIn: CGRect(x: cx! - r!, y: cy! - r!, width: 2 * r!, height: 2 * r!), transform: nil)
        }
        precondition(kind == "path")
        return vectorPath(d!)
    }
    var bounds: CGRect {
        if let width = strokeWidth { return path.copy(strokingWithWidth: width, lineCap: .round, lineJoin: .round, miterLimit: 10).boundingBoxOfPath }
        return path.boundingBoxOfPath
    }
    func draw(_ context: CGContext) {
        context.addPath(path)
        if let width = strokeWidth {
            context.setLineWidth(width); context.setLineCap(.round); context.strokePath()
        } else { context.fillPath() }
    }
    func svg(_ paint: String) -> String {
        if kind == "circle" { return "<circle cx=\"\(cx!)\" cy=\"\(cy!)\" r=\"\(r!)\" fill=\"\(paint)\"/>" }
        if let width = strokeWidth { return "<path d=\"\(d!)\" fill=\"none\" stroke=\"\(paint)\" stroke-width=\"\(width)\" stroke-linecap=\"round\"/>" }
        return "<path d=\"\(d!)\" fill=\"\(paint)\"/>"
    }
}
struct Layer: Decodable { let shapes: [Shape]; let cutouts: [Shape] }
struct Artwork: Decodable {
    let width: CGFloat, height: CGFloat, scale: CGFloat
    let layers: [Layer]
    var bounds: CGRect {
        layers.flatMap(\.shapes).reduce(CGRect.null) { $0.union($1.bounds) }.applying(CGAffineTransform(scaleX: scale, y: scale))
    }
    func draw(_ context: CGContext, ink: CGColor) {
        context.saveGState(); context.scaleBy(x: scale, y: scale)
        for layer in layers {
            context.saveGState()
            if !layer.cutouts.isEmpty { context.beginTransparencyLayer(auxiliaryInfo: nil) }
            context.setFillColor(ink); context.setStrokeColor(ink)
            layer.shapes.forEach { $0.draw(context) }
            if !layer.cutouts.isEmpty {
                context.setBlendMode(.clear)
                layer.cutouts.forEach { $0.draw(context) }
                context.setBlendMode(.normal)
                context.endTransparencyLayer()
            }
            context.restoreGState()
        }
        context.restoreGState()
    }
    func svg(_ ink: String) -> String {
        var defs = "", groups = ""
        for (index, layer) in layers.enumerated() {
            let mask = "cut\(index)"
            if !layer.cutouts.isEmpty {
                defs += "<mask id=\"\(mask)\" maskUnits=\"userSpaceOnUse\" x=\"0\" y=\"0\" width=\"\(width)\" height=\"\(height)\"><rect width=\"\(width)\" height=\"\(height)\" fill=\"white\"/>\(layer.cutouts.map { $0.svg("black") }.joined())</mask>"
            }
            let attribute = layer.cutouts.isEmpty ? "" : " mask=\"url(#\(mask))\""
            groups += "<g\(attribute)>\(layer.shapes.map { $0.svg(ink) }.joined())</g>"
        }
        return "<defs>\(defs)</defs><g transform=\"scale(\(scale))\">\(groups)</g>"
    }
}
struct Sources: Decodable { let master: Artwork; let small16: Artwork; let small24: Artwork }
let sources = try JSONDecoder().decode(Sources.self, from: Data(contentsOf: repository.appendingPathComponent("client/assets/icon-artwork.json")))
struct Palette { let tile: String; let bottom: String; let ink: String }
let terracotta = Palette(tile: "#fcfbf9", bottom: "#f1eae3", ink: "#bd481b")
let monochrome = Palette(tile: "#ffffff", bottom: "#efefef", ink: "#171918")
func color(_ hex: String) -> CGColor {
    let value = UInt32(hex.dropFirst(), radix: 16)!
    return CGColor(srgbRed: CGFloat((value >> 16) & 255) / 255, green: CGFloat((value >> 8) & 255) / 255,
                   blue: CGFloat(value & 255) / 255, alpha: 1)
}
func context(_ size: Int) -> CGContext {
    guard let output = CGContext(data: nil, width: size, height: size, bitsPerComponent: 8, bytesPerRow: size * 4,
                                 space: CGColorSpace(name: CGColorSpace.sRGB)!, bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue) else { fatalError("Cannot create icon bitmap") }
    output.interpolationQuality = .high
    return output
}
enum Usage { case application, toolbar, menuBar }
func opticalArtwork(_ size: Int) -> Artwork {
    // 32px is the Retina representation of 16px; 48px is the Retina representation of 24px.
    if size == 24 || size == 48 || size == 64 { return sources.small24 }
    return size <= 32 ? sources.small16 : sources.master
}
func fit(_ artwork: Artwork, in target: CGRect, context: CGContext, ink: CGColor) {
    let bounds = artwork.bounds
    let scale = min(target.width / bounds.width, target.height / bounds.height)
    context.saveGState()
    context.translateBy(x: target.midX - bounds.midX * scale, y: target.midY - bounds.midY * scale)
    context.scaleBy(x: scale, y: scale)
    artwork.draw(context, ink: ink)
    context.restoreGState()
}
func render(_ size: Int, usage: Usage = .application, palette: Palette = monochrome) -> CGImage {
    let sampling = size <= 128 ? 4 : 1
    let bitmap = context(size * sampling)
    bitmap.translateBy(x: 0, y: CGFloat(size * sampling))
    bitmap.scaleBy(x: CGFloat(size * sampling) / canvas, y: -CGFloat(size * sampling) / canvas)
    let artwork = opticalArtwork(size)
    if usage == .menuBar {
        bitmap.scaleBy(x: canvas / sources.small16.width, y: canvas / sources.small16.height)
        sources.small16.draw(bitmap, ink: color("#000000"))
    } else {
        let tile = usage == .toolbar ? CGRect(x: 0, y: 0, width: canvas, height: canvas) : CGRect(x: 26, y: 26, width: 204, height: 204)
        let corner: CGFloat = usage == .toolbar ? 48 : 45
        bitmap.saveGState()
        bitmap.addPath(CGPath(roundedRect: tile, cornerWidth: corner, cornerHeight: corner, transform: nil))
        bitmap.clip()
        if usage == .application {
            let gradient = CGGradient(colorsSpace: CGColorSpace(name: CGColorSpace.sRGB), colors: [color(palette.tile), color(palette.bottom)] as CFArray, locations: [0, 1])!
            bitmap.drawLinearGradient(gradient, start: CGPoint(x: 128, y: 26), end: CGPoint(x: 128, y: 230), options: [.drawsBeforeStartLocation, .drawsAfterEndLocation])
        } else { bitmap.setFillColor(color(palette.ink)); bitmap.fill(tile) }
        if usage == .toolbar && size <= 48 {
            bitmap.scaleBy(x: canvas / artwork.width, y: canvas / artwork.height)
            artwork.draw(bitmap, ink: color("#ffffff"))
        } else if usage == .toolbar {
            fit(artwork, in: CGRect(x: 16, y: 24, width: 224, height: 208), context: bitmap, ink: color("#ffffff"))
        } else if size <= 64 {
            fit(artwork, in: sources.master.bounds, context: bitmap, ink: color(palette.ink))
        } else { sources.master.draw(bitmap, ink: color(palette.ink)) }
        bitmap.restoreGState()
    }
    guard let image = bitmap.makeImage() else { fatalError("Cannot render icon") }
    if sampling == 1 { return image }
    let output = context(size)
    output.draw(image, in: CGRect(x: 0, y: 0, width: size, height: size))
    return output.makeImage()!
}
func png(_ image: CGImage) -> Data {
    let data = NSMutableData()
    let destination = CGImageDestinationCreateWithData(data as CFMutableData, UTType.png.identifier as CFString, 1, nil)!
    CGImageDestinationAddImage(destination, image, nil)
    precondition(CGImageDestinationFinalize(destination), "Cannot encode icon")
    return data as Data
}
func export(_ image: CGImage, to path: String) throws {
    try png(image).write(to: repository.appendingPathComponent(path), options: .atomic)
    print(path)
}
func chunk(_ type: String, payload: Data) -> Data {
    var data = Data(type.utf8), length = UInt32(payload.count + 8).bigEndian
    withUnsafeBytes(of: &length) { data.append(contentsOf: $0) }
    data.append(payload)
    return data
}
let representations = [("icp4", 16), ("icp5", 32), ("ic07", 128), ("ic08", 256), ("ic09", 512),
                       ("ic11", 32), ("ic12", 64), ("ic13", 256), ("ic14", 512), ("ic10", 1024)]
for (palette, suffix) in [(terracotta, ""), (monochrome, "-monochrome")] {
    let vector = "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 256 256\" fill=\"none\"><title>Passpine</title><defs><linearGradient id=\"paper\" x1=\"128\" y1=\"26\" x2=\"128\" y2=\"230\" gradientUnits=\"userSpaceOnUse\"><stop stop-color=\"\(palette.tile)\"/><stop offset=\"1\" stop-color=\"\(palette.bottom)\"/></linearGradient></defs><rect x=\"26\" y=\"26\" width=\"204\" height=\"204\" rx=\"45\" fill=\"url(#paper)\"/>\(sources.master.svg(palette.ink))</svg>\n"
    try vector.write(to: repository.appendingPathComponent("client/assets/app-icon\(suffix).svg"), atomically: true, encoding: .utf8)
    let image = render(256, palette: palette)
    try export(image, to: "client/assets/app-icon\(suffix).png")
    try export(image, to: "browser/src/assets/app-icon\(suffix).png")
    for size in [16, 24, 32, 48, 128] {
        try export(render(size, usage: .toolbar, palette: palette), to: "browser/public/icon\(size)\(suffix).png")
    }
    var payload = Data(), images = [Int: Data]()
    for (type, size) in representations {
        let data = images[size] ?? png(render(size, palette: palette))
        images[size] = data; payload.append(chunk(type, payload: data))
    }
    try chunk("icns", payload: payload).write(to: repository.appendingPathComponent("client/assets/Passpine\(suffix).icns"), options: .atomic)
}
try export(render(32, usage: .menuBar), to: "client/assets/menu-bar-template.png")
let menuBarVector = "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 \(sources.small16.width) \(sources.small16.height)\" fill=\"none\">\(sources.small16.svg("#000000"))</svg>\n"
try menuBarVector.write(to: repository.appendingPathComponent("client/assets/menu-bar-template.svg"), atomically: true, encoding: .utf8)
