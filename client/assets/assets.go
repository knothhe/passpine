// Package assets shares the application artwork with native integrations.
package assets

import _ "embed"

// AppIconPNG contains the terracotta and warm-white squirrel artwork.
// generate.swift exports it from the shared vector geometry at 256 pixels.
//
//go:embed app-icon.png
var AppIconPNG []byte

// AppIconMonochromePNG contains the light tile and black squirrel artwork.
//
//go:embed app-icon-monochrome.png
var AppIconMonochromePNG []byte

// Dock ICNS artwork preserves optical small drawings and Retina representations.
// The macOS backend loads these bytes with NSImage.initWithData.
//
//go:embed Passpine.icns
var appIconICNS []byte

//go:embed Passpine-monochrome.icns
var monochromeIconICNS []byte

// DockIconForStyle returns the multi-resolution macOS application artwork.
func DockIconForStyle(style string) []byte {
	if style == "onevra" {
		return appIconICNS
	}
	return monochromeIconICNS
}

// IconForStyle returns the brand artwork paired with a theme style.
func IconForStyle(style string) []byte {
	if style == "onevra" {
		return AppIconPNG
	}
	return AppIconMonochromePNG
}

// MenuBarTemplatePNG is the optical small squirrel drawn in black on transparency.
// macOS applies the appropriate menu bar foreground color.
//
//go:embed menu-bar-template.png
var MenuBarTemplatePNG []byte
