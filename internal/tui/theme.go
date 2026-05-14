package tui

import (
	"fmt"
	"image/color"
	"sort"

	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
)

// Theme defines a complete color palette for the TUI.
type Theme struct {
	Error            color.Color
	Success          color.Color
	Warning          color.Color
	Accent           color.Color
	AccentStr        string
	Special          color.Color
	Caution          color.Color
	Highlight        color.Color
	Foreground       color.Color
	ForegroundBright color.Color
	Muted            color.Color
	Subtle           color.Color
	Surface          color.Color

	EpicPalette []color.Color
}

// GlamourStyleConfig is the glamour style used for markdown rendering.
// Updated by SetTheme to match the active color theme.
var GlamourStyleConfig = styles.DarkStyleConfig

var themes = map[string]Theme{
	"default": {
		Error:            lipgloss.Color("9"),
		Success:          lipgloss.Color("10"),
		Warning:          lipgloss.Color("11"),
		Accent:           lipgloss.Color("12"),
		AccentStr:        "12",
		Special:          lipgloss.Color("13"),
		Caution:          lipgloss.Color("208"),
		Highlight:        lipgloss.Color("15"),
		Foreground:       lipgloss.Color("252"),
		ForegroundBright: lipgloss.Color("255"),
		Muted:            lipgloss.Color("244"),
		Subtle:           lipgloss.Color("240"),
		Surface:          lipgloss.Color("237"),
		EpicPalette: []color.Color{
			lipgloss.Color("39"), lipgloss.Color("208"), lipgloss.Color("141"), lipgloss.Color("43"), lipgloss.Color("214"), lipgloss.Color("99"), lipgloss.Color("203"), lipgloss.Color("118"), lipgloss.Color("45"), lipgloss.Color("220"),
		},
	},
	"tokyonight": {
		Error:            lipgloss.Color("#f7768e"), // red
		Success:          lipgloss.Color("#9ece6a"), // green
		Warning:          lipgloss.Color("#e0af68"), // yellow
		Accent:           lipgloss.Color("#f7768e"), // red
		AccentStr:        "#f7768e",
		Special:          lipgloss.Color("#bb9af7"), // magenta
		Caution:          lipgloss.Color("#ff9e64"), // orange
		Highlight:        lipgloss.Color("#a9b1d6"), // white
		Foreground:       lipgloss.Color("#c0caf5"), // foreground
		ForegroundBright: lipgloss.Color("#c0caf5"), // bright white
		Muted:            lipgloss.Color("#414868"), // bright black
		Subtle:           lipgloss.Color("#283457"), // comment
		Surface:          lipgloss.Color("#283457"), // selection
		EpicPalette: []color.Color{
			lipgloss.Color("#7aa2f7"), lipgloss.Color("#ff9e64"), lipgloss.Color("#9ece6a"), lipgloss.Color("#7dcfff"), lipgloss.Color("#e0af68"),
			lipgloss.Color("#bb9af7"), lipgloss.Color("#f7768e"), lipgloss.Color("#73daca"), lipgloss.Color("#2ac3de"), lipgloss.Color("#ff007c"),
		},
	},
	"catppuccin": {
		Error:            lipgloss.Color("#f38ba8"), // red
		Success:          lipgloss.Color("#a6e3a1"), // green
		Warning:          lipgloss.Color("#f9e2af"), // yellow
		Accent:           lipgloss.Color("#cba6f7"), // mauve
		AccentStr:        "#cba6f7",
		Special:          lipgloss.Color("#f5c2e7"), // pink
		Caution:          lipgloss.Color("#fab387"), // peach
		Highlight:        lipgloss.Color("#bac2de"), // subtext1
		Foreground:       lipgloss.Color("#cdd6f4"), // text
		ForegroundBright: lipgloss.Color("#cdd6f4"), // text
		Muted:            lipgloss.Color("#6c7086"), // overlay0
		Subtle:           lipgloss.Color("#585b70"), // surface2
		Surface:          lipgloss.Color("#313244"), // surface0
		EpicPalette: []color.Color{
			lipgloss.Color("#89b4fa"), lipgloss.Color("#fab387"), lipgloss.Color("#a6e3a1"), lipgloss.Color("#94e2d5"), lipgloss.Color("#f9e2af"),
			lipgloss.Color("#cba6f7"), lipgloss.Color("#f38ba8"), lipgloss.Color("#f5c2e7"), lipgloss.Color("#74c7ec"), lipgloss.Color("#f5e0dc"),
		},
	},
}

// glamourStyles maps theme names to glamour style configs for markdown rendering.
var glamourStyles = map[string]ansi.StyleConfig{
	"default":    styles.DarkStyleConfig,
	"tokyonight": styles.TokyoNightStyleConfig,
	"catppuccin": catppuccinGlamourStyle,
}

// SetTheme applies a named theme by overwriting the package-level color
// variables and rebuilding pre-built styles. Call once at startup before
// any TUI rendering begins.
func SetTheme(name string) error {
	t, ok := themes[name]
	if !ok {
		return fmt.Errorf("unknown theme %q (available: %v)", name, ThemeNames())
	}

	ColorError = t.Error
	ColorSuccess = t.Success
	ColorWarning = t.Warning
	ColorAccent = t.Accent
	ColorSpecial = t.Special
	ColorCaution = t.Caution
	ColorHighlight = t.Highlight
	ColorForeground = t.Foreground
	ColorForegroundBright = t.ForegroundBright
	ColorMuted = t.Muted
	ColorSubtle = t.Subtle
	ColorSurface = t.Surface
	ColorSpinner = t.Accent

	if len(t.EpicPalette) > 0 {
		epicPalette = t.EpicPalette
	}

	// Rebuild pre-built styles with new colors.
	MutedStyle = lipgloss.NewStyle().Foreground(ColorMuted)
	BoldAccent = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
	SurfaceBg = lipgloss.NewStyle().Background(ColorSurface)

	if gs, ok := glamourStyles[name]; ok {
		GlamourStyleConfig = gs
	}

	// Override glamour heading color to match the active theme's Accent.
	GlamourStyleConfig.Heading.Color = &t.AccentStr

	return nil
}

// ThemeNames returns the sorted list of available theme names.
func ThemeNames() []string {
	names := make([]string, 0, len(themes))
	for name := range themes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func stringPtr(s string) *string { return &s }
func boolPtr(b bool) *bool       { return &b }
func uintPtr(u uint) *uint       { return &u }

var catppuccinGlamourStyle = ansi.StyleConfig{
	Document: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			BlockPrefix: "\n",
			BlockSuffix: "\n",
			Color:       stringPtr("#cdd6f4"),
		},
		Margin: uintPtr(2),
	},
	BlockQuote: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{},
		Indent:         uintPtr(1),
		IndentToken:    stringPtr("│ "),
	},
	List: ansi.StyleList{
		StyleBlock: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color: stringPtr("#cdd6f4"),
			},
		},
		LevelIndent: 2,
	},
	Heading: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			BlockSuffix: "\n",
			Color:       stringPtr("#cba6f7"),
			Bold:        boolPtr(true),
		},
	},
	H1: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			Prefix: "# ",
			Bold:   boolPtr(true),
		},
	},
	H2: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			Prefix: "## ",
		},
	},
	H3: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			Prefix: "### ",
		},
	},
	H4: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			Prefix: "#### ",
		},
	},
	H5: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			Prefix: "##### ",
		},
	},
	H6: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			Prefix: "###### ",
		},
	},
	Strikethrough: ansi.StylePrimitive{
		CrossedOut: boolPtr(true),
	},
	Emph: ansi.StylePrimitive{
		Italic: boolPtr(true),
	},
	Strong: ansi.StylePrimitive{
		Bold: boolPtr(true),
	},
	HorizontalRule: ansi.StylePrimitive{
		Color:  stringPtr("#585b70"),
		Format: "\n--------\n",
	},
	Item: ansi.StylePrimitive{
		BlockPrefix: "• ",
	},
	Enumeration: ansi.StylePrimitive{
		BlockPrefix: ". ",
		Color:       stringPtr("#94e2d5"),
	},
	Task: ansi.StyleTask{
		StylePrimitive: ansi.StylePrimitive{},
		Ticked:         "[✓] ",
		Unticked:       "[ ] ",
	},
	Link: ansi.StylePrimitive{
		Color:     stringPtr("#89b4fa"),
		Underline: boolPtr(true),
	},
	LinkText: ansi.StylePrimitive{
		Color: stringPtr("#94e2d5"),
	},
	Image: ansi.StylePrimitive{
		Color:     stringPtr("#89b4fa"),
		Underline: boolPtr(true),
	},
	ImageText: ansi.StylePrimitive{
		Color:  stringPtr("#94e2d5"),
		Format: "Image: {{.text}} →",
	},
	Code: ansi.StyleBlock{
		StylePrimitive: ansi.StylePrimitive{
			Color: stringPtr("#fab387"),
		},
	},
	CodeBlock: ansi.StyleCodeBlock{
		StyleBlock: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{
				Color: stringPtr("#cdd6f4"),
			},
			Margin: uintPtr(2),
		},
		Chroma: &ansi.Chroma{
			Text: ansi.StylePrimitive{
				Color: stringPtr("#cdd6f4"),
			},
			Error: ansi.StylePrimitive{
				Color:           stringPtr("#cdd6f4"),
				BackgroundColor: stringPtr("#f38ba8"),
			},
			Comment: ansi.StylePrimitive{
				Color: stringPtr("#6c7086"),
			},
			CommentPreproc: ansi.StylePrimitive{
				Color: stringPtr("#f5c2e7"),
			},
			Keyword: ansi.StylePrimitive{
				Color: stringPtr("#cba6f7"),
			},
			KeywordReserved: ansi.StylePrimitive{
				Color: stringPtr("#cba6f7"),
			},
			KeywordNamespace: ansi.StylePrimitive{
				Color: stringPtr("#cba6f7"),
			},
			KeywordType: ansi.StylePrimitive{
				Color: stringPtr("#f9e2af"),
			},
			Operator: ansi.StylePrimitive{
				Color: stringPtr("#94e2d5"),
			},
			Punctuation: ansi.StylePrimitive{
				Color: stringPtr("#6c7086"),
			},
			Name: ansi.StylePrimitive{
				Color: stringPtr("#89b4fa"),
			},
			NameConstant: ansi.StylePrimitive{
				Color: stringPtr("#fab387"),
			},
			NameBuiltin: ansi.StylePrimitive{
				Color: stringPtr("#f38ba8"),
			},
			NameTag: ansi.StylePrimitive{
				Color: stringPtr("#cba6f7"),
			},
			NameAttribute: ansi.StylePrimitive{
				Color: stringPtr("#a6e3a1"),
			},
			NameClass: ansi.StylePrimitive{
				Color: stringPtr("#f9e2af"),
			},
			NameDecorator: ansi.StylePrimitive{
				Color: stringPtr("#a6e3a1"),
			},
			NameFunction: ansi.StylePrimitive{
				Color: stringPtr("#89b4fa"),
			},
			LiteralNumber: ansi.StylePrimitive{
				Color: stringPtr("#fab387"),
			},
			LiteralString: ansi.StylePrimitive{
				Color: stringPtr("#a6e3a1"),
			},
			LiteralStringEscape: ansi.StylePrimitive{
				Color: stringPtr("#f5c2e7"),
			},
			GenericDeleted: ansi.StylePrimitive{
				Color: stringPtr("#f38ba8"),
			},
			GenericEmph: ansi.StylePrimitive{
				Italic: boolPtr(true),
			},
			GenericInserted: ansi.StylePrimitive{
				Color: stringPtr("#a6e3a1"),
			},
			GenericStrong: ansi.StylePrimitive{
				Bold: boolPtr(true),
			},
			GenericSubheading: ansi.StylePrimitive{
				Color: stringPtr("#cba6f7"),
			},
			Background: ansi.StylePrimitive{
				BackgroundColor: stringPtr("#1e1e2e"),
			},
		},
	},
	Table: ansi.StyleTable{
		StyleBlock: ansi.StyleBlock{
			StylePrimitive: ansi.StylePrimitive{},
		},
	},
	DefinitionDescription: ansi.StylePrimitive{
		BlockPrefix: "\n🠶 ",
	},
}
