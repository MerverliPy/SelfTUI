// Package ui is the Bubble Tea application: root model, tab/status bar,
// responsive layout system, and (from later milestones) the per-tab views.
package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Styles is the centralized theme (PLAN.md §7): one palette, shared by every
// view. Only layout geometry differs by breakpoint, never the theme objects.
type Styles struct {
	// colors
	accent   color.Color
	bg       color.Color
	fg       color.Color
	muted    color.Color
	selected color.Color

	// composed styles
	Tab         lipgloss.Style
	TabActive   lipgloss.Style
	Status      lipgloss.Style
	Body        lipgloss.Style
	Placeholder lipgloss.Style
}

// NewStyles builds the theme for "dark" or "light" (any other value → dark).
func NewStyles(theme string) Styles {
	dark := theme != "light"
	s := Styles{}

	if dark {
		s.accent = lipgloss.Color("63")   // violet
		s.bg = lipgloss.Color("0")        // black
		s.fg = lipgloss.Color("15")       // white
		s.muted = lipgloss.Color("245")   // grey
		s.selected = lipgloss.Color("62") // blue
	} else {
		s.accent = lipgloss.Color("57") // violet (darker for contrast on white)
		s.bg = lipgloss.Color("15")     // white
		s.fg = lipgloss.Color("0")      // black
		s.muted = lipgloss.Color("240") // grey
		s.selected = lipgloss.Color("33")
	}

	s.Tab = lipgloss.NewStyle().Padding(0, 1).Foreground(s.muted)
	s.TabActive = lipgloss.NewStyle().Padding(0, 1).Background(s.accent).Foreground(s.fg).Bold(true)
	s.Status = lipgloss.NewStyle().Padding(0, 1).Foreground(s.muted).Border(lipgloss.RoundedBorder(), false).
		BorderForeground(s.muted)
	s.Body = lipgloss.NewStyle().Padding(1).Foreground(s.fg)
	s.Placeholder = lipgloss.NewStyle().Foreground(s.muted).Italic(true)
	return s
}
