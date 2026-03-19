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

// ContainsCI is a case-insensitive membership check.
func ContainsCI(list []string, val string) bool {
	for _, item := range list {
		if strings.EqualFold(item, val) {
			return true
		}
	}
	return false
}
