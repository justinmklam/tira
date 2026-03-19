package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// FixedWidth returns s padded or truncated to exactly n runes.
func FixedWidth(s string, n int) string {
	r := []rune(s)
	if len(r) == n {
		return s
	}
	if len(r) > n {
		if n <= 1 {
			return string(r[:n])
		}
		return string(r[:n-1]) + "…"
	}
	return s + strings.Repeat(" ", n-len(r))
}

// Clamp constrains v to the range [lo, hi].
func Clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// SplitPanes renders left and right string blocks side-by-side, separated by
// a dim vertical bar, each block padded/trimmed to exactly height lines.
func SplitPanes(left, right string, leftWidth, height int) string {
	div := lipgloss.NewStyle().Foreground(ColorDimmer).Render("│")
	leftLines := strings.Split(left, "\n")
	rightLines := strings.Split(right, "\n")
	rows := make([]string, height)
	for i := 0; i < height; i++ {
		var l, r string
		if i < len(leftLines) {
			l = leftLines[i]
		}
		if i < len(rightLines) {
			r = rightLines[i]
		}
		rows[i] = lipgloss.NewStyle().Width(leftWidth).Render(l) + div + r
	}
	return strings.Join(rows, "\n")
}

// ListPaneWidth returns the width of the list pane in a split layout.
func ListPaneWidth(totalWidth int) int {
	w := totalWidth * 40 / 100
	if w < 30 {
		w = 30
	}
	return w
}

// DetailPaneWidth returns the width of the detail pane in a split layout.
func DetailPaneWidth(totalWidth int) int {
	w := totalWidth - ListPaneWidth(totalWidth) - 1
	if w < 20 {
		w = 20
	}
	return w
}

// OverlaySize returns the outer (border-inclusive) dimensions for the floating
// detail overlay, clamped to reasonable min/max values.
func OverlaySize(totalWidth, totalHeight int) (w, h int) {
	w = totalWidth * 85 / 100
	if w > 140 {
		w = 140
	}
	if w < 60 {
		w = 60
	}
	h = totalHeight * 85 / 100
	if h > 50 {
		h = 50
	}
	if h < 15 {
		h = 15
	}
	return
}

// OverlayViewportSize returns the (width, height) for the viewport inside the
// floating detail overlay, accounting for border (2) and chrome lines (header,
// footer, two newline separators → 4 more rows).
func OverlayViewportSize(totalWidth, totalHeight int) (vpW, vpH int) {
	w, h := OverlaySize(totalWidth, totalHeight)
	vpW = w - 4 // 2 border + 2 padding
	vpH = h - 6 // 2 border + header(1) + sep(1) + sep(1) + footer(1)
	if vpW < 20 {
		vpW = 20
	}
	if vpH < 5 {
		vpH = 5
	}
	return
}

// ContainsCI is a case-insensitive membership check.
func ContainsCI(list []string, val string) bool {
	for _, item := range list {
		if strings.EqualFold(item, val) {
			return true
		}
	}
	return false
}
