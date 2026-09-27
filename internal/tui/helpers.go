package tui

import (
	"fmt"
	"image/color"
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

// BoardTabs are the view names shown in the board tab strip, in cycle order.
var BoardTabs = []string{"Backlog", "Kanban", "Epics"}

// flattenRow is SanitizeRow without whitespace collapsing, so an
// already-sized value keeps its padding. Newlines and carriage returns become
// spaces and control characters are dropped, which is what keeps a badge on one
// line.
func flattenRow(s string) string {
	if s == "" {
		return ""
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			return ' '
		case r < 0x20 || r == 0x7f:
			return -1
		}
		return r
	}, s)
}

// TitleBar renders a full-width row with bold, brightest text at the left and a
// muted right-hand detail (pass "" for none). The result is exactly width cells;
// the detail is dropped rather than truncated when it cannot fit. It carries no
// background fill — only the cursor row does.
func TitleBar(text, right string, width int) string {
	if width <= 0 {
		return ""
	}
	left := SanitizeRow(text)
	right = SanitizeRow(right)
	leftStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorForegroundBright)

	leftCells := width
	includeRight := right != "" && DisplayWidth(left)+2+DisplayWidth(right) <= width
	if includeRight {
		leftCells = width - DisplayWidth(right) - 2
	}

	var b strings.Builder
	b.WriteString(leftStyle.Render(FixedWidth(left, leftCells)))
	if includeRight {
		b.WriteString("  ")
		b.WriteString(MutedStyle.Render(right))
	}
	return b.String()
}

// ModalTitle renders the in-modal title band, sized to the modal's inner width.
func ModalTitle(text string, innerW int) string {
	return TitleBar(text, "", innerW)
}

// TabStrip renders BoardTabs over the whole width with the active tab filled
// and bracketed; right is a view-specific detail such as "DEMO Sprint 1". The
// result is exactly width cells. When the tabs and the detail cannot both fit,
// the detail is dropped entirely rather than truncating a tab.
func TabStrip(active int, right string, width int) string {
	if right == "" {
		return TabStripStyled(active, "", width)
	}
	return TabStripStyled(active, MutedStyle.Render(SanitizeRow(right)), width)
}

// TabStripStyled is TabStrip for a right-hand detail that is already sanitised
// and styled, such as a row of coloured transient badges. Callers must pass
// text that has been through SanitizeRow; the width is measured after styling.
func TabStripStyled(active int, right string, width int) string {
	if width <= 0 {
		return ""
	}

	var lb strings.Builder
	lb.WriteString(" ")
	for i, name := range BoardTabs {
		if i > 0 {
			lb.WriteString("  ")
		}
		if i == active {
			lb.WriteString(BoldAccent.Render("[" + name + "]"))
		} else {
			lb.WriteString(MutedStyle.Render(name))
		}
	}
	left := lb.String()
	lw := DisplayWidth(left)
	if lw >= width {
		return TruncateWidth(left, width)
	}

	mid := width - lw
	if right != "" && mid >= 2+DisplayWidth(right) {
		mid -= 2 + DisplayWidth(right)
		return left +
			strings.Repeat(" ", mid+2) +
			right
	}
	return left + strings.Repeat(" ", mid)
}

// SectionHeader renders a bold, caller-coloured label padded to width. The
// caller owns the colour so a column header can be status-tinted. An over-long
// label is truncated with an ellipsis, never wrapped, and no background fill is
// applied.
func SectionHeader(text string, fg color.Color, width int) string {
	return sectionHeader(text, fg, width, true)
}

// SectionHeaderRegular is SectionHeader without the bold weight. It is used
// where weight, not hue, distinguishes the active element (a kanban column).
func SectionHeaderRegular(text string, fg color.Color, width int) string {
	return sectionHeader(text, fg, width, false)
}

func sectionHeader(text string, fg color.Color, width int, bold bool) string {
	if width <= 0 {
		return ""
	}
	// flattenRow rather than SanitizeRow: a column header is pre-aligned, and
	// collapsing its runs of spaces would destroy the alignment.
	label := FixedWidth(flattenRow(text), width)
	return lipgloss.NewStyle().Bold(bold).Foreground(fg).Render(label)
}

// EmptyState renders centred, dim, italic placeholder text inside width cells.
func EmptyState(text string, width int) string {
	if width <= 0 {
		return ""
	}
	text = SanitizeRow(text)
	if DisplayWidth(text) > width {
		text = FixedWidth(text, width)
	}
	pad := width - DisplayWidth(text)
	left := pad / 2
	right := pad - left
	return MutedStyle.Italic(true).Render(strings.Repeat(" ", left) + text + strings.Repeat(" ", right))
}

// Badge renders text as a single-line pill with one space of padding on each
// side, in the given foreground and background colours. It does not pad to a
// column: callers pass an already-sized, sanitized string.
func Badge(text string, fg, bg color.Color) string {
	return lipgloss.NewStyle().Foreground(fg).Background(bg).Render(" " + flattenRow(text) + " ")
}

// FooterHints renders "key: description" hints with bolded keys and muted
// descriptions. Whole hints are dropped from the tail (lowest priority last)
// rather than truncating one mid-token, and an ellipsis marks the drop.
func FooterHints(hints []string, width int) string {
	if width <= 0 || len(hints) == 0 {
		return ""
	}
	for n := len(hints); n >= 1; n-- {
		s := renderHintList(hints[:n])
		w := DisplayWidth(s)
		if w > width {
			continue
		}
		if n == len(hints) {
			return s
		}
		if w+1 <= width {
			return s + MutedStyle.Render("…")
		}
	}
	return TruncateWidth(renderHintList(hints[:1]), width)
}

func renderHintList(hints []string) string {
	keyStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorForeground)
	parts := make([]string, len(hints))
	for i, h := range hints {
		h = SanitizeRow(h)
		key, desc := h, ""
		if idx := strings.IndexByte(h, ' '); idx >= 0 {
			key, desc = h[:idx], h[idx+1:]
		}
		if desc == "" {
			parts[i] = keyStyle.Render(key)
		} else {
			parts[i] = keyStyle.Render(key) + " " + MutedStyle.Render(desc)
		}
	}
	return strings.Join(parts, "   ")
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

// Frame renders body inside a rounded border of exactly outerW columns and h
// rows (border included). A non-empty title is embedded in the top border after
// "─ ". Body lines are clamped to outerW-2, so an over-wide line is truncated
// with … rather than wrapped. bc colours the border, tc the bold title. h <= 0
// sizes the frame to the body, and the frame emits exactly h rows: body lines
// past h-2 are dropped and short bodies are padded with empty framed rows.
func Frame(title, body string, outerW, h int, bc, tc color.Color) string {
	if outerW < 4 {
		return ""
	}
	innerW := outerW - 2
	bodyLines := strings.Split(body, "\n")
	if h <= 0 {
		h = len(bodyLines) + 2
	}
	if h < 2 {
		h = 2
	}

	b := lipgloss.RoundedBorder()
	border := lipgloss.NewStyle().Foreground(bc)

	// Top border: an optional title is embedded after "╭─ ". A title too wide for
	// the frame is truncated to the cells available, so the row never wraps.
	var top strings.Builder
	top.WriteString(border.Render(b.TopLeft))
	titleW := DisplayWidth(title)
	if maxTitleW := outerW - 5; titleW > maxTitleW {
		titleW = maxTitleW
	}
	if titleW > 0 {
		titleStyle := lipgloss.NewStyle().Bold(true).Foreground(tc)
		top.WriteString(border.Render(b.Top + " "))
		top.WriteString(titleStyle.Render(FixedWidth(title, titleW)))
		top.WriteString(border.Render(" "))
		top.WriteString(border.Render(strings.Repeat(b.Top, outerW-5-titleW)))
	} else {
		top.WriteString(border.Render(strings.Repeat(b.Top, innerW)))
	}
	top.WriteString(border.Render(b.TopRight))

	rows := make([]string, 0, h)
	rows = append(rows, top.String())
	for i := 0; i < h-2; i++ {
		line := strings.Repeat(" ", innerW)
		if i < len(bodyLines) {
			line = FixedWidth(bodyLines[i], innerW)
		}
		rows = append(rows, border.Render(b.Left)+line+border.Render(b.Right))
	}
	rows = append(rows,
		border.Render(b.BottomLeft)+border.Render(strings.Repeat(b.Bottom, innerW))+border.Render(b.BottomRight))

	return strings.Join(rows, "\n")
}

// SplitView renders an untitled list pane and a rounded "Details" pane side by
// side with a one-column gutter. h is each pane's outer height in rows, border
// included. totalW is the full terminal width, and the result is exactly totalW
// columns wide: ListPaneWidth returns the list pane's content width while
// DetailPaneWidth already subtracts both frames and the gutter.
func SplitView(listBody, detailBody string, totalW, h int) string {
	list := Frame("", listBody, ListPaneWidth(totalW)+2, h, ColorSubtle, ColorForegroundBright)
	detail := Frame("Details", detailBody, DetailPaneWidth(totalW)+2, h, ColorSubtle, ColorForegroundBright)
	return lipgloss.JoinHorizontal(lipgloss.Top, list, " ", detail)
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

// DetailPaneWidth returns the width of the detail pane's content in a split
// layout. It subtracts the list pane's two border columns, the one-column
// gutter, and the detail pane's own two border columns from totalWidth.
func DetailPaneWidth(totalWidth int) int {
	w := totalWidth - (ListPaneWidth(totalWidth) + 2) - 1 - 2
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
