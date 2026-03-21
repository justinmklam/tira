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

	// Empty input should produce empty content array
	if len(content) != 0 {
		t.Errorf("empty input should produce empty content array, got %d elements", len(content))
	}

	// Convert back to markdown - should be empty
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

func TestMarkdownToADF_Headings(t *testing.T) {
	input := "# Heading 1\n\n## Heading 2\n\n### Heading 3\n\nSome text."
	got := markdownToADF(input)

	content, ok := got["content"].([]any)
	if !ok {
		t.Fatal("content is not a slice")
	}

	// Should have 4 elements: 3 headings + 1 paragraph
	if len(content) != 4 {
		t.Fatalf("content has %d elements, want 4", len(content))
	}

	// Check first heading
	h1, ok := content[0].(map[string]any)
	if !ok {
		t.Fatal("first element is not a map")
	}
	if h1["type"] != "heading" {
		t.Errorf("first element type = %q, want %q", h1["type"], "heading")
	}
	if attrs, ok := h1["attrs"].(map[string]any); ok {
		if level, ok := attrs["level"].(int); !ok || level != 1 {
			t.Errorf("heading level = %v, want 1", attrs["level"])
		}
	}

	// Check second heading
	h2, ok := content[1].(map[string]any)
	if !ok {
		t.Fatal("second element is not a map")
	}
	if h2["type"] != "heading" {
		t.Errorf("second element type = %q, want %q", h2["type"], "heading")
	}
	if attrs, ok := h2["attrs"].(map[string]any); ok {
		if level, ok := attrs["level"].(int); !ok || level != 2 {
			t.Errorf("heading level = %v, want 2", attrs["level"])
		}
	}

	// Check third heading
	h3, ok := content[2].(map[string]any)
	if !ok {
		t.Fatal("third element is not a map")
	}
	if h3["type"] != "heading" {
		t.Errorf("third element type = %q, want %q", h3["type"], "heading")
	}
	if attrs, ok := h3["attrs"].(map[string]any); ok {
		if level, ok := attrs["level"].(int); !ok || level != 3 {
			t.Errorf("heading level = %v, want 3", attrs["level"])
		}
	}

	// Check paragraph
	para, ok := content[3].(map[string]any)
	if !ok {
		t.Fatal("fourth element is not a map")
	}
	if para["type"] != "paragraph" {
		t.Errorf("fourth element type = %q, want %q", para["type"], "paragraph")
	}
}

func TestMarkdownToADF_Lists(t *testing.T) {
	input := "- Item 1\n- Item 2\n- Item 3"
	got := markdownToADF(input)

	content, ok := got["content"].([]any)
	if !ok {
		t.Fatal("content is not a slice")
	}

	if len(content) != 1 {
		t.Fatalf("content has %d elements, want 1", len(content))
	}

	list, ok := content[0].(map[string]any)
	if !ok {
		t.Fatal("first element is not a map")
	}
	if list["type"] != "bulletList" {
		t.Errorf("list type = %q, want %q", list["type"], "bulletList")
	}

	items, ok := list["content"].([]any)
	if !ok {
		t.Fatal("list content is not a slice")
	}
	if len(items) != 3 {
		t.Errorf("list has %d items, want 3", len(items))
	}
}

func TestMarkdownToADF_CodeBlock(t *testing.T) {
	input := "```go\nfmt.Println(\"hello\")\n```"
	got := markdownToADF(input)

	content, ok := got["content"].([]any)
	if !ok {
		t.Fatal("content is not a slice")
	}

	if len(content) != 1 {
		t.Fatalf("content has %d elements, want 1", len(content))
	}

	codeBlock, ok := content[0].(map[string]any)
	if !ok {
		t.Fatal("first element is not a map")
	}
	if codeBlock["type"] != "codeBlock" {
		t.Errorf("codeBlock type = %q, want %q", codeBlock["type"], "codeBlock")
	}

	if attrs, ok := codeBlock["attrs"].(map[string]any); ok {
		if lang, ok := attrs["language"].(string); !ok || lang != "go" {
			t.Errorf("codeBlock language = %v, want %q", attrs["language"], "go")
		}
	}
}
