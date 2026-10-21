//go:build !darwin

// Package traybutton keeps a macOS status button selected while its panel is
// visible. Other platforms retain their native tray behavior.
package traybutton

type Controller struct{}

func Install(string, func()) (*Controller, error) { return &Controller{}, nil }
func (*Controller) SetHighlighted(bool)           {}
func (*Controller) Close()                        {}
