/*
 * Copyright Metaplay. Licensed under the Apache-2.0 license.
 */
package styles

import (
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/jwalton/go-supportscolor"
)

var (
	// Define colors based on platform
	ColorNeutral lipgloss.Color
	ColorOrange  lipgloss.Color
	ColorGreen   lipgloss.Color
	ColorBlue    lipgloss.Color
	ColorRed     lipgloss.Color
	ColorYellow  lipgloss.Color

	// Styles using the colors
	StyleTitle     lipgloss.Style
	StyleSuccess   lipgloss.Style
	StyleError     lipgloss.Style
	StyleWarning   lipgloss.Style
	StyleTechnical lipgloss.Style
	StyleMuted     lipgloss.Style
	StylePrompt    lipgloss.Style

	ListStyle = lipgloss.NewStyle()
)

func init() {
	// Check terminal color support
	colorSupport := supportscolor.SupportsColor(os.Stdout.Fd())

	// Use appropriate colors based on terminal capabilities
	if colorSupport.Has16m {
		// Terminal supports true color (24-bit)
		ColorNeutral = lipgloss.Color("#737373")
		ColorOrange = lipgloss.Color("#ff7a00")
		ColorGreen = lipgloss.Color("#28a745") // Metaplay green: lipgloss.Color("#3f6730")
		ColorBlue = lipgloss.Color("#2d90dc")
		ColorRed = lipgloss.Color("#ef4444")
		ColorYellow = lipgloss.Color("#ffff55")
	} else if colorSupport.Has256 {
		// Terminal supports 256 colors (8-bit)
		ColorNeutral = lipgloss.Color("240") // Gray
		ColorOrange = lipgloss.Color("208")  // Orange
		ColorGreen = lipgloss.Color("34")    // Green
		ColorBlue = lipgloss.Color("33")     // Blue
		ColorRed = lipgloss.Color("196")     // Red
		ColorYellow = lipgloss.Color("226")  // Yellow
	} else if colorSupport.SupportsColor {
		// Terminal only supports basic 16 colors
		ColorNeutral = lipgloss.Color("darkgray")
		ColorOrange = lipgloss.Color("yellow") // Basic terminals don't have orange
		ColorGreen = lipgloss.Color("green")
		ColorBlue = lipgloss.Color("blue")
		ColorRed = lipgloss.Color("red")
		ColorYellow = lipgloss.Color("yellow")
	} else {
		// Fallback for terminals with no color support
		// Using basic ANSI named colors which will be ignored in terminals without color
		ColorNeutral = lipgloss.Color("white")
		ColorOrange = lipgloss.Color("white")
		ColorGreen = lipgloss.Color("white")
		ColorBlue = lipgloss.Color("white")
		ColorRed = lipgloss.Color("white")
		ColorYellow = lipgloss.Color("white")
	}

	// Initialize styles with the appropriate colors
	// Explicitly set background to "default" to ensure proper rendering in macOS Terminal
	StyleTitle = lipgloss.NewStyle().Foreground(ColorBlue).Bold(true)
	StyleSuccess = lipgloss.NewStyle().Foreground(ColorGreen)
	StyleError = lipgloss.NewStyle().Foreground(ColorRed)
	StyleWarning = lipgloss.NewStyle().Foreground(ColorYellow)
	StyleTechnical = lipgloss.NewStyle().Foreground(ColorBlue)
	StyleMuted = lipgloss.NewStyle().Foreground(ColorNeutral)
	StylePrompt = lipgloss.NewStyle().Foreground(ColorOrange).Bold(true)
}
