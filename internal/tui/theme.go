package tui

import (
	"fmt"
	"sort"

	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
)

// Theme defines a complete color palette for the TUI.
type Theme struct {
	Red      lipgloss.Color
	Green    lipgloss.Color
	Yellow   lipgloss.Color
	Blue     lipgloss.Color
	Magenta  lipgloss.Color
	Orange   lipgloss.Color
	White    lipgloss.Color
	Fg       lipgloss.Color
	FgBright lipgloss.Color
	Dim      lipgloss.Color
	Dimmer   lipgloss.Color
	Bg       lipgloss.Color

	EpicPalette []lipgloss.Color
}

// GlamourStyleConfig is the glamour style used for markdown rendering.
// Updated by SetTheme to match the active color theme.
var GlamourStyleConfig = styles.DarkStyleConfig

var themes = map[string]Theme{
	"default": {
		Red:      lipgloss.Color("9"),
		Green:    lipgloss.Color("10"),
		Yellow:   lipgloss.Color("11"),
		Blue:     lipgloss.Color("12"),
		Magenta:  lipgloss.Color("13"),
		Orange:   lipgloss.Color("208"),
		White:    lipgloss.Color("15"),
		Fg:       lipgloss.Color("252"),
		FgBright: lipgloss.Color("255"),
		Dim:      lipgloss.Color("244"),
		Dimmer:   lipgloss.Color("240"),
		Bg:       lipgloss.Color("237"),
		EpicPalette: []lipgloss.Color{
			"39", "208", "141", "43", "214", "99", "203", "118", "45", "220",
		},
	},
	"catppuccin": {
		Red:      lipgloss.Color("#f38ba8"), // red
		Green:    lipgloss.Color("#a6e3a1"), // green
		Yellow:   lipgloss.Color("#f9e2af"), // yellow
		Blue:     lipgloss.Color("#cba6f7"), // blue
		Magenta:  lipgloss.Color("#f5c2e7"), // pink
		Orange:   lipgloss.Color("#fab387"), // peach
		White:    lipgloss.Color("#bac2de"), // subtext1
		Fg:       lipgloss.Color("#cdd6f4"), // text
		FgBright: lipgloss.Color("#cdd6f4"), // text
		Dim:      lipgloss.Color("#6c7086"), // overlay0
		Dimmer:   lipgloss.Color("#585b70"), // surface2
		Bg:       lipgloss.Color("#313244"), // surface0
		EpicPalette: []lipgloss.Color{
			"#89b4fa", "#fab387", "#a6e3a1", "#94e2d5", "#f9e2af",
			"#cba6f7", "#f38ba8", "#f5c2e7", "#74c7ec", "#f5e0dc",
		},
	},
}

// glamourStyles maps theme names to glamour style configs for markdown rendering.
var glamourStyles = map[string]ansi.StyleConfig{
	"default":    styles.DarkStyleConfig,
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

	ColorRed = t.Red
	ColorGreen = t.Green
	ColorYellow = t.Yellow
	ColorBlue = t.Blue
	ColorMagenta = t.Magenta
	ColorOrange = t.Orange
	ColorWhite = t.White
	ColorFg = t.Fg
	ColorFgBright = t.FgBright
	ColorDim = t.Dim
	ColorDimmer = t.Dimmer
	ColorBg = t.Bg
	SpinnerColor = t.Blue

	if len(t.EpicPalette) > 0 {
		epicPalette = t.EpicPalette
	}

	// Rebuild pre-built styles with new colors.
	DimStyle = lipgloss.NewStyle().Foreground(ColorDim)
	BoldBlue = lipgloss.NewStyle().Bold(true).Foreground(ColorBlue)
	SelectedBg = lipgloss.NewStyle().Background(ColorBg)

	if gs, ok := glamourStyles[name]; ok {
		GlamourStyleConfig = gs
	}

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
