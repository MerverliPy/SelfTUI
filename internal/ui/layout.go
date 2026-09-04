package ui

// Breakpoint system (PLAN.md §7 + §10 M0/M0a).
//
// Thresholds are evidence-based, not assumed: M0a MEASURED the target SSH
// client (Moshi, iPhone 16 Pro) at 72 cols x 30 rows portrait with the
// default font — see docs/m0a-gate-evidence.md; the local control harness is
// scripts/probe-local.sh, instrument = cmd/size-probe. Moshi landscape at the
// default font is ~150+ cols (Wide); with a large font ~100–119 (Medium).
// PC terminals are the wide reference.

const (
	// devicePortraitCols is the measured narrow geometry: Moshi on an iPhone
	// 16 Pro in portrait, default font. Anchor for the compact range.
	devicePortraitCols = 72

	// minTermW/minTermH are the smallest window SelfTUI renders its shell in.
	// Below either bound the shell chrome cannot lay out, so the App shows a
	// bounded "terminal too small" message instead (phase 7 hardening).
	minTermW = 40
	minTermH = 12

	// compactMax is the widest "compact" layout: stacked panels, full-width
	// controls. Holds the measured portrait width (72) with margin for
	// zoomed-in fonts (~60–78 cols).
	compactMax = 79
	// mediumMax is the widest "medium" layout: split panels where sensible.
	// Portrait never reaches it; landscape-with-large-font and mid-size PC
	// windows do.
	mediumMax = 119
	// mediumSplitMin: below this, a medium-width screen still stacks the
	// Models panes — a 60/40 split would leave the detail pane <50 cols,
	// which is not readable on the phone. Roughly 1.25x the measured
	// portrait width.
	mediumSplitMin = 90
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
		// narrow-but-split: below mediumSplitMin, keep text readable by
		// stacking (a split leaves the detail pane <50 cols).
		if width < mediumSplitMin {
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
