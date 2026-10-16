// Package assets shares the application artwork with native integrations.
package assets

import _ "embed"

// AppIconPNG contains the terracotta and warm-white botanical key artwork.
// generate.swift exports it from the shared vector geometry at 256 pixels.
//
//go:embed app-icon.png
var AppIconPNG []byte

// AppIconMonochromePNG contains the white tile and black botanical key artwork.
//
//go:embed app-icon-monochrome.png
var AppIconMonochromePNG []byte

// IconForStyle returns the brand artwork paired with a theme style.
func IconForStyle(style string) []byte {
	if style == "monochrome" {
		return AppIconMonochromePNG
	}
	return AppIconPNG
}

// MenuBarTemplatePNG is the botanical key drawn in black on transparency.
// macOS applies the appropriate menu bar foreground color.
//
//go:embed menu-bar-template.png
var MenuBarTemplatePNG []byte
