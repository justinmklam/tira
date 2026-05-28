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

func TestMarkdownToADF_InlineFormatting(t *testing.T) {
	t.Run("link produces text node with link mark", func(t *testing.T) {
		adf := markdownToADF("Check [example](https://example.com) here")
		content, _ := adf["content"].([]any)
		if len(content) == 0 {
			t.Fatal("no content nodes")
		}
		para, _ := content[0].(map[string]any)
		nodes, _ := para["content"].([]any)

		var linkNode map[string]any
		for _, n := range nodes {
			node, _ := n.(map[string]any)
			marks, _ := node["marks"].([]any)
			for _, m := range marks {
				mark, _ := m.(map[string]any)
				if mark["type"] == "link" {
					linkNode = node
				}
			}
		}
		if linkNode == nil {
			t.Fatal("no text node with link mark found")
		}
		if linkNode["text"] != "example" {
			t.Errorf("link text = %q, want %q", linkNode["text"], "example")
		}
		marks, _ := linkNode["marks"].([]any)
		linkMark, _ := marks[0].(map[string]any)
		attrs, _ := linkMark["attrs"].(map[string]any)
		if attrs["href"] != "https://example.com" {
			t.Errorf("link href = %q, want %q", attrs["href"], "https://example.com")
		}
	})

	t.Run("bold produces text node with strong mark", func(t *testing.T) {
		adf := markdownToADF("**bold text**")
		content, _ := adf["content"].([]any)
		para, _ := content[0].(map[string]any)
		nodes, _ := para["content"].([]any)
		if len(nodes) == 0 {
			t.Fatal("no inline nodes")
		}
		node, _ := nodes[0].(map[string]any)
		if node["text"] != "bold text" {
			t.Errorf("text = %q, want %q", node["text"], "bold text")
		}
		marks, _ := node["marks"].([]any)
		if len(marks) == 0 {
			t.Fatal("no marks on bold node")
		}
		mark, _ := marks[0].(map[string]any)
		if mark["type"] != "strong" {
			t.Errorf("mark type = %q, want %q", mark["type"], "strong")
		}
	})

	t.Run("italic produces text node with em mark", func(t *testing.T) {
		adf := markdownToADF("_italic text_")
		content, _ := adf["content"].([]any)
		para, _ := content[0].(map[string]any)
		nodes, _ := para["content"].([]any)
		if len(nodes) == 0 {
			t.Fatal("no inline nodes")
		}
		node, _ := nodes[0].(map[string]any)
		if node["text"] != "italic text" {
			t.Errorf("text = %q, want %q", node["text"], "italic text")
		}
		marks, _ := node["marks"].([]any)
		if len(marks) == 0 {
			t.Fatal("no marks on italic node")
		}
		mark, _ := marks[0].(map[string]any)
		if mark["type"] != "em" {
			t.Errorf("mark type = %q, want %q", mark["type"], "em")
		}
	})

	t.Run("inline code produces text node with code mark", func(t *testing.T) {
		adf := markdownToADF("Use `fmt.Println` to print")
		content, _ := adf["content"].([]any)
		para, _ := content[0].(map[string]any)
		nodes, _ := para["content"].([]any)

		var codeNode map[string]any
		for _, n := range nodes {
			node, _ := n.(map[string]any)
			marks, _ := node["marks"].([]any)
			for _, m := range marks {
				mark, _ := m.(map[string]any)
				if mark["type"] == "code" {
					codeNode = node
				}
			}
		}
		if codeNode == nil {
			t.Fatal("no text node with code mark found")
		}
		if codeNode["text"] != "fmt.Println" {
			t.Errorf("code text = %q, want %q", codeNode["text"], "fmt.Println")
		}
	})

	t.Run("link roundtrip preserves text and URL", func(t *testing.T) {
		input := "Check [example](https://example.com) here"
		adf := markdownToADF(input)
		got := strings.TrimSpace(ADFToMarkdown(adf))
		if !strings.Contains(got, "example") {
			t.Errorf("roundtrip lost link text: %q", got)
		}
		if !strings.Contains(got, "https://example.com") {
			t.Errorf("roundtrip lost link URL: %q", got)
		}
	})
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
