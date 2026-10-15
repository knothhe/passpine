// Package assets shares the application artwork with native integrations.
package assets

import _ "embed"

// AppIconPNG is generated from the bundle's Passpine.icns artwork.
//
//go:embed app-icon.png
var AppIconPNG []byte

// MenuBarTemplatePNG contains the application's leaves and stem on transparency.
// macOS applies the appropriate menu bar foreground color.
//
//go:embed menu-bar-template.png
var MenuBarTemplatePNG []byte
