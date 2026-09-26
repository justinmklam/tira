package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// DisplayWidth returns the number of terminal cells s occupies when rendered.
// Unlike a rune count it treats wide characters (CJK, emoji) as two cells and
// ignores ANSI escape sequences, which is what decides whether a line wraps.
func DisplayWidth(s string) int { return ansi.StringWidth(s) }

// SanitizeRow flattens arbitrary text into a single displayable line. Newlines,
// carriage returns, and tabs become spaces, escape and other control characters
// are dropped, and runs of whitespace collapse. Data supplied by a caller (an
// issue summary, a display name) therefore cannot break out of a fixed-width
// row or inject its own styling.
func SanitizeRow(s string) string {
	if s == "" {
		return ""
	}
	flat := strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			return ' '
		case r < 0x20 || r == 0x7f: // control characters, including ESC
			return -1
		}
		return r
	}, s)
	return strings.Join(strings.Fields(flat), " ")
}

// FixedWidth returns s padded or truncated to exactly n display cells, so a
// column never overflows its slot no matter how wide the characters are. ANSI
// escape sequences are preserved. A non-positive n yields the empty string.
func FixedWidth(s string, n int) string {
	if n <= 0 {
		return ""
	}
	w := DisplayWidth(s)
	switch {
	case w == n:
		return s
	case w > n:
		if n == 1 {
			return ansi.Truncate(s, 1, "")
		}
		// A wide character that would cross the boundary is dropped rather than
		// split, which can leave the result short of n cells: pad it back out.
		truncated := ansi.Truncate(s, n, "…")
		if pad := n - DisplayWidth(truncated); pad > 0 {
			truncated += strings.Repeat(" ", pad)
		}
		return truncated
	default:
		return s + strings.Repeat(" ", n-w)
	}
}

// TruncateWidth returns s clipped to at most n terminal display cells. It is
// ANSI-aware, so styling that straddles the cut is closed rather than broken,
// and a string that already fits is returned unchanged (no padding). A
// non-positive n yields the empty string.
func TruncateWidth(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if DisplayWidth(s) <= n {
		return s
	}
	return ansi.Truncate(s, n, "")
}

// FitInput renders input within the given number of terminal cells, sizing its
// scrolling viewport so a long value stays on a single line with the cursor in
// view. The prompt and the cursor cell are counted against the budget. The input
// is taken by value, so the caller's model keeps its own width.
func FitInput(input textinput.Model, cells int) string {
	width := cells - DisplayWidth(input.Prompt) - 1
	if width < 1 {
		width = 1
	}
	input.SetWidth(width)
	// The viewport offsets are only recomputed when the value or cursor
	// changes, so nudge the cursor to reflow for the width set above.
	input.SetCursor(input.Position())
	return input.View()
}

// FormatStoryPoints returns a compact display value for story points.
func FormatStoryPoints(points float64) string {
	if !(points > 0) {
		return "—"
	}
	if points == float64(int(points)) {
		return fmt.Sprintf("%d", int(points))
	}
	return fmt.Sprintf("%.1f", points)
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
	div := lipgloss.NewStyle().Foreground(ColorSubtle).Render("│")
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
// The list pane takes 55% of the total width.
func ListPaneWidth(totalWidth int) int {
	w := totalWidth * 55 / 100
	if w < 30 {
		w = 30
	}
	return w
}

// DetailPaneWidth returns the width of the detail pane in a split layout.
// The detail pane takes the remaining width (approximately 35%).
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
	h = totalHeight * 95 / 100
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

// PickerFooter is the navigation hint shown at the bottom of a picker modal.
const PickerFooter = "  ↑/↓ ctrl+p/n: navigate   enter: select   esc: cancel"

// pickerFooterNarrow replaces PickerFooter when the modal is too narrow for it.
const pickerFooterNarrow = "  ↑/↓ enter: select   esc: cancel"

// Smallest terminal a picker modal can be drawn into without overflowing it.
const (
	pickerMinTermW = 40
	pickerMinTermH = 10
)

// PickerOverlaySize returns the outer modal width (border included), the usable
// inner width, and the number of list rows that fit, for a terminal of the
// given size. modalW and innerW are the values to hand to lipgloss Width: they
// already account for the border, so a body line of innerW cells fits exactly.
// Both results are clamped so the modal can never be larger than the terminal.
func PickerOverlaySize(totalW, totalH int) (modalW, innerW, listH int) {
	w, h := totalW, totalH
	if w == 0 {
		w = 120
	}
	if h == 0 {
		h = 40
	}

	modalW = w * 2 / 3
	if modalW > 90 {
		modalW = 90
	}
	if modalW < 52 {
		modalW = 52
	}
	if modalW > w {
		modalW = w
	}
	innerW = modalW - 2

	// Chrome is five rows: two border rows, header, separator, footer.
	listH = h/2 - 6
	if maxRows := h - 5; listH > maxRows {
		listH = maxRows
	}
	if listH < 1 {
		listH = 1
	}
	return modalW, innerW, listH
}

// RenderPickerModal renders a centered picker modal with one shared frame: a
// bold title header, content, a separator, and a muted footer.
//
// content is called with the usable inner width and the number of list rows,
// and must return at most listH+2 lines (a picker returns its input line, a
// separator, and up to listH rows). Every line is clamped to innerW display
// cells, so a long value can never wrap and break the frame. Footers wider than
// the modal are swapped for a compact hint, then truncated as a last resort.
func RenderPickerModal(title string, content func(innerW, listH int) string, footer string, totalW, totalH int) string {
	w, h := totalW, totalH
	if w == 0 {
		w = 120
	}
	if h == 0 {
		h = 40
	}

	if w < pickerMinTermW || h < pickerMinTermH {
		const msg = "Terminal too small"
		if w < DisplayWidth(msg)+2 || h < 3 {
			return ""
		}
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, MutedStyle.Render(msg))
	}

	modalW, innerW, listH := PickerOverlaySize(w, h)

	header := BoldAccent.Padding(0, 1).Width(innerW).
		Render(FixedWidth(SanitizeRow(title), innerW-2))

	contentStr := ""
	if content != nil {
		contentStr = content(innerW, listH)
	}
	bodyLines := clampLines(strings.Split(contentStr, "\n"), innerW, listH+2)

	if DisplayWidth(footer) > innerW {
		footer = pickerFooterNarrow
	}

	body := header + "\n" +
		strings.Join(bodyLines, "\n") + "\n" +
		MutedStyle.Render(FixedWidth(footer, innerW))

	modal := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorAccent).
		Width(modalW).
		Render(body)

	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, modal)
}

// clampLines forces each line to exactly width display cells and limits the
// block to maxLines lines. Styled lines keep their ANSI sequences.
func clampLines(lines []string, width, maxLines int) []string {
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = FixedWidth(line, width)
	}
	return out
}

// RenderPickerOverlay renders a centered picker modal with consistent styling.
// The picker model's View method is called to render the list content.
func RenderPickerOverlay(pickerView func(innerW, listH int) string, title string, totalW, totalH int) string {
	return RenderPickerModal(title, pickerView, PickerFooter, totalW, totalH)
}
