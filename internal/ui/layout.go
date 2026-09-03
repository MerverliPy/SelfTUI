package ui

// Breakpoint system (PLAN.md §7 + §10 M0).
//
// Thresholds are deliberately centralized here. M0a will MEASURE real
// WindowSizeMsg widths on the target SSH clients (Blink/Termius) and drive
// these ranges from that measurement — they are explicitly NOT an assumed
// 88-col value.

const (
	// compactMax is the widest "compact" layout: stacked panels, full-width
	// controls. Target: narrow SSH windows (iPhone portrait).
	compactMax = 79
	// mediumMax is the widest "medium" layout: split panels where sensible.
	mediumMax = 119
)

// Breakpoint classifies a width for layout selection.
type Breakpoint int

const (
	Compact Breakpoint = iota // ≤ compactMax: stacked, foldable
	Medium                    // compactMax+1..mediumMax: split, still narrow
	Wide                      // ≥ mediumMax+1: full side-by-side panels
)

// BreakpointFor classifies width.
func BreakpointFor(width int) Breakpoint {
	switch {
	case width <= compactMax:
		return Compact
	case width <= mediumMax:
		return Medium
	default:
		return Wide
	}
}

// Breakpoint names for display/status (and golden tests later).
func (b Breakpoint) String() string {
	switch b {
	case Compact:
		return "compact"
	case Medium:
		return "medium"
	default:
		return "wide"
	}
}

// ModelsLayout describes how the Models view arranges its two panes
// (list + detail) for a given width. Other views get their own helpers as
// they land (M1+). This is the M0 responsive-shell proof that geometry
// adapts without touching theme objects.
type ModelsLayout struct {
	// SideBySide is true when list and detail sit side by side (Wide, and
	// Medium when the pane fits); false → stacked, detail toggled.
	SideBySide bool
	// ListWidth is the list pane width when SideBySide.
	ListWidth int
}

// ForModels returns the Models geometry for a width.
func ForModels(width int) ModelsLayout {
	bp := BreakpointFor(width)
	switch bp {
	case Wide:
		return ModelsLayout{SideBySide: true, ListWidth: width * 40 / 100}
	case Medium:
		// narrow-but-split: stack below 90 cols to keep text readable.
		if width < 90 {
			return ModelsLayout{SideBySide: false}
		}
		return ModelsLayout{SideBySide: true, ListWidth: width * 40 / 100}
	default:
		return ModelsLayout{SideBySide: false}
	}
}

// DetailWidth returns the detail pane width for a side-by-side layout
// (the remaining space); 0 when stacked.
func (m ModelsLayout) DetailWidth(total int) int {
	if !m.SideBySide {
		return 0
	}
	return total - m.ListWidth
}
