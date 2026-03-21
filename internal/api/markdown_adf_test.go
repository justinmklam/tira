package api

import (
	"strings"
	"testing"
)

func TestMarkdownToADF_Structure(t *testing.T) {
	input := "Hello world"
	got := markdownToADF(input)

	// Check top-level structure
	if got["version"] != 1 {
		t.Errorf("version = %v, want 1", got["version"])
	}
	if got["type"] != "doc" {
		t.Errorf("type = %q, want %q", got["type"], "doc")
	}

	content, ok := got["content"].([]any)
	if !ok {
		t.Fatal("content is not a slice")
	}
	if len(content) != 1 {
		t.Fatalf("content has %d elements, want 1", len(content))
	}

	// Check paragraph
	para, ok := content[0].(map[string]any)
	if !ok {
		t.Fatal("first content element is not a map")
	}
	if para["type"] != "paragraph" {
		t.Errorf("paragraph type = %q, want %q", para["type"], "paragraph")
	}

	paraContent, ok := para["content"].([]any)
	if !ok {
		t.Fatal("paragraph content is not a slice")
	}
	if len(paraContent) != 1 {
		t.Fatalf("paragraph content has %d elements, want 1", len(paraContent))
	}

	// Check text node
	textNode, ok := paraContent[0].(map[string]any)
	if !ok {
		t.Fatal("text node is not a map")
	}
	if textNode["type"] != "text" {
		t.Errorf("text type = %q, want %q", textNode["type"], "text")
	}
	if textNode["text"] != input {
		t.Errorf("text = %q, want %q", textNode["text"], input)
	}
}

func TestMarkdownToADF_Roundtrip(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"simple", "Hello world"},
		{"with newlines", "Line 1\nLine 2"},
		{"with markdown", "**bold** and _italic_"},
		{"code", "Use `fmt.Println`"},
		{"link", "Check [example](https://example.com)"},
		{"multiline", "First paragraph\n\nSecond paragraph"},
		{"special chars", "Special: <>&\"'"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adf := markdownToADF(tt.input)
			got := ADFToMarkdown(adf)

			// ADFToMarkdown adds trailing newlines for paragraphs, so trim
			got = strings.TrimSpace(got)
			input := strings.TrimSpace(tt.input)

			// For simple text without markdown formatting, should preserve exactly
			if tt.name == "simple" || tt.name == "special chars" || tt.name == "empty" {
				if got != input {
					t.Errorf("roundtrip = %q, want %q", got, input)
				}
				return
			}

			// For other cases, just verify content isn't completely lost
			if input != "" && got == "" {
				t.Errorf("roundtrip lost all content: got empty from input %q", tt.input)
			}
		})
	}
}

func TestMarkdownToADF_EmptyString(t *testing.T) {
	got := markdownToADF("")

	content, ok := got["content"].([]any)
	if !ok {
		t.Fatal("content is not a slice")
	}

	if len(content) != 1 {
		t.Errorf("empty input should still produce paragraph structure, got %d elements", len(content))
	}

	// Convert back to markdown - should be empty or whitespace
	back := ADFToMarkdown(got)
	if strings.TrimSpace(back) != "" {
		t.Errorf("empty input roundtrip = %q, want empty", back)
	}
}

func TestMarkdownToADF_LongText(t *testing.T) {
	input := strings.Repeat("This is a long line. ", 100)
	got := markdownToADF(input)

	back := ADFToMarkdown(got)
	back = strings.TrimSpace(back)

	if !strings.Contains(back, "This is a long line") {
		t.Error("long text was not preserved in roundtrip")
	}
}

func TestMarkdownToADF_Unicode(t *testing.T) {
	input := "Hello 世界 🌍 Привет"
	got := markdownToADF(input)
	back := ADFToMarkdown(got)
	back = strings.TrimSpace(back)

	if back != input {
		t.Errorf("unicode roundtrip = %q, want %q", back, input)
	}
}
