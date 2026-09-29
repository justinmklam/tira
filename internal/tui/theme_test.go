package tui

import (
	"fmt"
	"image/color"
	"math"
	"testing"

	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
)

// assumedBg is the terminal background each theme is designed against.
var assumedBg = map[string]color.Color{
	"default":    lipgloss.Color("#1e1e1e"),
	"tokyonight": lipgloss.Color("#1a1b26"),
	"catppuccin": lipgloss.Color("#1e1e2e"),
}

// rgb8 resolves any lipgloss colour to 8-bit RGB through the color.Color
// interface. Do NOT parse fmt.Sprint: lipgloss.Color returns a different
// concrete type per input (ansi.BasicColor, ansi.IndexedColor, color.RGBA) and
// only the hex form renders as a parseable string.
func rgb8(c color.Color) (uint8, uint8, uint8) {
	r, g, b, _ := c.RGBA()
	return uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)
}

func relLuminance(c color.Color) float64 {
	r, g, b := rgb8(c)
	lin := func(v uint8) float64 {
		s := float64(v) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

// contrastRatio is the WCAG relative-luminance contrast ratio.
func contrastRatio(a, b color.Color) float64 {
	la, lb := relLuminance(a), relLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func restoreDefaultTheme(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		if err := SetTheme("default"); err != nil {
			t.Fatalf("restoring default theme: %v", err)
		}
	})
}

func TestSetTheme_Default(t *testing.T) {
	restoreDefaultTheme(t)

	if err := SetTheme("default"); err != nil {
		t.Fatalf("SetTheme(\"default\"): %v", err)
	}

	if fmt.Sprint(ColorError) == "" {
		t.Error("ColorError should have a non-empty string representation")
	}
	if fmt.Sprint(ColorAccent) == "" {
		t.Error("ColorAccent should have a non-empty string representation")
	}
}

func TestSetTheme_Catppuccin(t *testing.T) {
	restoreDefaultTheme(t)

	if err := SetTheme("catppuccin"); err != nil {
		t.Fatalf("SetTheme(\"catppuccin\"): %v", err)
	}

	if ColorAccent == nil {
		t.Error("ColorAccent should not be nil")
	}
	if ColorSpinner == nil {
		t.Error("ColorSpinner should not be nil")
	}
}

func TestSetTheme_Unknown(t *testing.T) {
	if err := SetTheme("nonexistent"); err == nil {
		t.Fatal("SetTheme(\"nonexistent\") should return error")
	}
}

func TestSetTheme_RebuildStyles(t *testing.T) {
	restoreDefaultTheme(t)

	if err := SetTheme("catppuccin"); err != nil {
		t.Fatalf("SetTheme: %v", err)
	}

	// MutedStyle should have a foreground color set.
	got := MutedStyle.GetForeground()
	if got == nil {
		t.Error("MutedStyle foreground should not be nil")
	}
}

func TestSetTheme_EpicPalette(t *testing.T) {
	restoreDefaultTheme(t)

	if err := SetTheme("catppuccin"); err != nil {
		t.Fatalf("SetTheme: %v", err)
	}

	color := EpicColor("PROJ-1")
	if color == nil {
		t.Error("EpicColor returned nil after theme switch")
	}
}

func TestThemeNames(t *testing.T) {
	names := ThemeNames()
	if len(names) < 2 {
		t.Fatalf("expected at least 2 themes, got %d", len(names))
	}
	// Should be sorted.
	for i := 1; i < len(names); i++ {
		if names[i] < names[i-1] {
			t.Errorf("ThemeNames not sorted: %v", names)
			break
		}
	}
}

// TestThemeContrast enforces the role-class thresholds from the design: body
// text at 4.5:1, dim/redundant text and decorative borders at 3.0:1, a visible
// band/selection separation from the terminal background, and legible ink on
// every colour used as a badge fill.
func TestThemeContrast(t *testing.T) {
	restoreDefaultTheme(t)

	for _, name := range ThemeNames() {
		t.Run(name, func(t *testing.T) {
			if err := SetTheme(name); err != nil {
				t.Fatalf("SetTheme(%q): %v", name, err)
			}
			bg := assumedBg[name]
			if bg == nil {
				t.Fatalf("no assumed background for theme %q", name)
			}

			textRoles := map[string]color.Color{
				"Foreground":       ColorForeground,
				"ForegroundBright": ColorForegroundBright,
				"Highlight":        ColorHighlight,
				"Muted":            ColorMuted,
				"Accent":           ColorAccent,
				"Error":            ColorError,
				"Success":          ColorSuccess,
				"Warning":          ColorWarning,
				"Special":          ColorSpecial,
				"Caution":          ColorCaution,
				"StatusInProgress": ColorStatusInProgress,
				"StatusDone":       ColorStatusDone,
				"StatusBlocked":    ColorStatusBlocked,
				"PriorityUrgent":   ColorPriorityUrgent,
				"PriorityHigh":     ColorPriorityHigh,
				"PriorityMedium":   ColorPriorityMedium,
				"PriorityLow":      ColorPriorityLow,
			}
			for role, c := range textRoles {
				if got := contrastRatio(c, bg); got < 4.5 {
					t.Errorf("%s: contrast %.2f < 4.5 against the background", role, got)
				}
			}
			// StatusTodo is a deliberately dim, redundant cue (the status name is
			// always available elsewhere), so it is held to the decorative floor.
			if got := contrastRatio(ColorStatusTodo, bg); got < 3.0 {
				t.Errorf("StatusTodo: contrast %.2f < 3.0 against the background", got)
			}
			if got := contrastRatio(ColorSubtle, bg); got < 3.0 {
				t.Errorf("Subtle: contrast %.2f < 3.0 against the background", got)
			}

			if got := contrastRatio(ColorSurface, bg); got < 1.15 {
				t.Errorf("Surface: separation %.2f < 1.15 from the background", got)
			}

			for role, c := range map[string]color.Color{
				"Error":   ColorError,
				"Success": ColorSuccess,
				"Accent":  ColorAccent,
				"Special": ColorSpecial,
				"Warning": ColorWarning,
				"Muted":   ColorMuted,
			} {
				if got := contrastRatio(ColorOnChrome, c); got < 4.5 {
					t.Errorf("OnChrome ink on %s fill: contrast %.2f < 4.5", role, got)
				}
			}

			if len(personPalette) != 8 {
				t.Fatalf("personPalette has %d entries, want 8", len(personPalette))
			}
			for i, c := range personPalette {
				if got := contrastRatio(c, bg); got < 4.5 {
					t.Errorf("PersonPalette[%d]: contrast %.2f < 4.5 against the background", i, got)
				}
			}
		})
	}
}

// TestGlamourHeadingsHaveNoBackground pins the removal of every glamour heading
// fill: the detail pane must not paint a background, only the cursor row does.
func TestGlamourHeadingsHaveNoBackground(t *testing.T) {
	restoreDefaultTheme(t)

	headingBlocks := func(sc ansi.StyleConfig) map[string]*string {
		return map[string]*string{
			"Heading": sc.Heading.BackgroundColor,
			"H1":      sc.H1.BackgroundColor,
			"H2":      sc.H2.BackgroundColor,
			"H3":      sc.H3.BackgroundColor,
			"H4":      sc.H4.BackgroundColor,
			"H5":      sc.H5.BackgroundColor,
			"H6":      sc.H6.BackgroundColor,
		}
	}

	for _, name := range ThemeNames() {
		t.Run(name, func(t *testing.T) {
			if err := SetTheme(name); err != nil {
				t.Fatalf("SetTheme(%q): %v", name, err)
			}
			for block, bg := range headingBlocks(GlamourStyleConfig) {
				if bg != nil {
					t.Errorf("%s.%s still has a background %q", name, block, *bg)
				}
			}
		})
	}
}

// TestGlamourHeadingClearDoesNotMutateSharedConfig guards against the pointer
// aliasing trap: the shipped style configs must keep their own values.
func TestGlamourHeadingClearDoesNotMutateSharedConfig(t *testing.T) {
	restoreDefaultTheme(t)
	if err := SetTheme("tokyonight"); err != nil {
		t.Fatalf("SetTheme: %v", err)
	}
	if styles.DarkStyleConfig.H1.BackgroundColor == nil {
		t.Error("clearing the active config's heading backgrounds mutated styles.DarkStyleConfig")
	}
}

func TestAllThemesDefineAllRoles(t *testing.T) {
	for name, th := range themes {
		t.Run(name, func(t *testing.T) {
			fields := map[string]color.Color{
				"Error": th.Error, "Success": th.Success, "Warning": th.Warning,
				"Accent": th.Accent, "Special": th.Special, "Caution": th.Caution,
				"Highlight": th.Highlight, "Foreground": th.Foreground,
				"ForegroundBright": th.ForegroundBright, "Muted": th.Muted,
				"Subtle": th.Subtle, "Surface": th.Surface,
				"OnChrome":   th.OnChrome,
				"StatusTodo": th.StatusTodo, "StatusInProgress": th.StatusInProgress,
				"StatusDone": th.StatusDone, "StatusBlocked": th.StatusBlocked,
				"PriorityUrgent": th.PriorityUrgent, "PriorityHigh": th.PriorityHigh,
				"PriorityMedium": th.PriorityMedium, "PriorityLow": th.PriorityLow,
			}
			for field, c := range fields {
				if c == nil {
					t.Errorf("theme %q: %s is nil", name, field)
				}
			}
			if len(th.PersonPalette) != 8 {
				t.Fatalf("theme %q: PersonPalette has %d entries, want 8", name, len(th.PersonPalette))
			}
			seen := map[string]bool{}
			for i, c := range th.PersonPalette {
				if c == nil {
					t.Errorf("theme %q: PersonPalette[%d] is nil", name, i)
					continue
				}
				key := fmt.Sprint(c)
				if seen[key] {
					t.Errorf("theme %q: PersonPalette[%d] duplicates another entry", name, i)
				}
				seen[key] = true
				if key == fmt.Sprint(th.Muted) || key == fmt.Sprint(th.Subtle) {
					t.Errorf("theme %q: PersonPalette[%d] collides with Muted/Subtle", name, i)
				}
			}
		})
	}
	for _, name := range ThemeNames() {
		if assumedBg[name] == nil {
			t.Errorf("assumedBg is missing theme %q", name)
		}
	}
}

// TestSetThemeAssignsEveryRole catches the failure mode where a role is added to
// the struct and the maps but SetTheme forgets to assign it, which would leave
// the previous theme's colour in place.
func TestSetThemeAssignsEveryRole(t *testing.T) {
	restoreDefaultTheme(t)

	for _, name := range ThemeNames() {
		t.Run(name, func(t *testing.T) {
			if err := SetTheme(name); err != nil {
				t.Fatalf("SetTheme(%q): %v", name, err)
			}
			want := themes[name]
			checks := []struct {
				field string
				got   color.Color
				want  color.Color
			}{
				{"Error", ColorError, want.Error},
				{"Success", ColorSuccess, want.Success},
				{"Warning", ColorWarning, want.Warning},
				{"Accent", ColorAccent, want.Accent},
				{"Special", ColorSpecial, want.Special},
				{"Caution", ColorCaution, want.Caution},
				{"Highlight", ColorHighlight, want.Highlight},
				{"Foreground", ColorForeground, want.Foreground},
				{"ForegroundBright", ColorForegroundBright, want.ForegroundBright},
				{"Muted", ColorMuted, want.Muted},
				{"Subtle", ColorSubtle, want.Subtle},
				{"Surface", ColorSurface, want.Surface},
				{"OnChrome", ColorOnChrome, want.OnChrome},
				{"StatusTodo", ColorStatusTodo, want.StatusTodo},
				{"StatusInProgress", ColorStatusInProgress, want.StatusInProgress},
				{"StatusDone", ColorStatusDone, want.StatusDone},
				{"StatusBlocked", ColorStatusBlocked, want.StatusBlocked},
				{"PriorityUrgent", ColorPriorityUrgent, want.PriorityUrgent},
				{"PriorityHigh", ColorPriorityHigh, want.PriorityHigh},
				{"PriorityMedium", ColorPriorityMedium, want.PriorityMedium},
				{"PriorityLow", ColorPriorityLow, want.PriorityLow},
			}
			for _, c := range checks {
				if fmt.Sprint(c.got) != fmt.Sprint(c.want) {
					t.Errorf("%s = %v, want %v", c.field, c.got, c.want)
				}
			}
			if len(personPalette) != len(want.PersonPalette) {
				t.Fatalf("personPalette has %d entries, want %d", len(personPalette), len(want.PersonPalette))
			}
			for i := range want.PersonPalette {
				if fmt.Sprint(personPalette[i]) != fmt.Sprint(want.PersonPalette[i]) {
					t.Errorf("personPalette[%d] = %v, want %v", i, personPalette[i], want.PersonPalette[i])
				}
			}
		})
	}
}
