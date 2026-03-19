package api

import (
	"strings"
	"testing"
)

func TestADFToMarkdown_EmptyDoc(t *testing.T) {
	doc := map[string]any{"type": "doc", "content": []any{}}
	got := ADFToMarkdown(doc)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestADFToMarkdown_Paragraph(t *testing.T) {
	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{"type": "text", "text": "Hello world"},
				},
			},
		},
	}
	got := ADFToMarkdown(doc)
	if !strings.Contains(got, "Hello world") {
		t.Errorf("expected 'Hello world', got %q", got)
	}
}

func TestADFToMarkdown_Heading(t *testing.T) {
	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type":  "heading",
				"attrs": map[string]any{"level": float64(2)},
				"content": []any{
					map[string]any{"type": "text", "text": "My Heading"},
				},
			},
		},
	}
	got := ADFToMarkdown(doc)
	if !strings.Contains(got, "## My Heading") {
		t.Errorf("expected '## My Heading', got %q", got)
	}
}

func TestADFToMarkdown_BoldAndItalic(t *testing.T) {
	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{
						"type": "text",
						"text": "bold",
						"marks": []any{
							map[string]any{"type": "strong"},
						},
					},
					map[string]any{"type": "text", "text": " and "},
					map[string]any{
						"type": "text",
						"text": "italic",
						"marks": []any{
							map[string]any{"type": "em"},
						},
					},
				},
			},
		},
	}
	got := ADFToMarkdown(doc)
	if !strings.Contains(got, "**bold**") {
		t.Errorf("expected **bold**, got %q", got)
	}
	if !strings.Contains(got, "_italic_") {
		t.Errorf("expected _italic_, got %q", got)
	}
}

func TestADFToMarkdown_CodeMark(t *testing.T) {
	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{
						"type":  "text",
						"text":  "foo",
						"marks": []any{map[string]any{"type": "code"}},
					},
				},
			},
		},
	}
	got := ADFToMarkdown(doc)
	if !strings.Contains(got, "`foo`") {
		t.Errorf("expected `foo`, got %q", got)
	}
}

func TestADFToMarkdown_StrikeAndLink(t *testing.T) {
	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{
						"type":  "text",
						"text":  "removed",
						"marks": []any{map[string]any{"type": "strike"}},
					},
					map[string]any{"type": "text", "text": " "},
					map[string]any{
						"type": "text",
						"text": "click",
						"marks": []any{map[string]any{
							"type":  "link",
							"attrs": map[string]any{"href": "https://example.com"},
						}},
					},
				},
			},
		},
	}
	got := ADFToMarkdown(doc)
	if !strings.Contains(got, "~~removed~~") {
		t.Errorf("expected ~~removed~~, got %q", got)
	}
	if !strings.Contains(got, "[click](https://example.com)") {
		t.Errorf("expected link markdown, got %q", got)
	}
}

func TestADFToMarkdown_BulletList(t *testing.T) {
	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type": "bulletList",
				"content": []any{
					map[string]any{
						"type": "listItem",
						"content": []any{
							map[string]any{
								"type": "paragraph",
								"content": []any{
									map[string]any{"type": "text", "text": "Item one"},
								},
							},
						},
					},
					map[string]any{
						"type": "listItem",
						"content": []any{
							map[string]any{
								"type": "paragraph",
								"content": []any{
									map[string]any{"type": "text", "text": "Item two"},
								},
							},
						},
					},
				},
			},
		},
	}
	got := ADFToMarkdown(doc)
	if !strings.Contains(got, "- Item one") {
		t.Errorf("expected '- Item one', got %q", got)
	}
	if !strings.Contains(got, "- Item two") {
		t.Errorf("expected '- Item two', got %q", got)
	}
}

func TestADFToMarkdown_OrderedList(t *testing.T) {
	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type": "orderedList",
				"content": []any{
					map[string]any{
						"type": "listItem",
						"content": []any{
							map[string]any{
								"type": "paragraph",
								"content": []any{
									map[string]any{"type": "text", "text": "First"},
								},
							},
						},
					},
					map[string]any{
						"type": "listItem",
						"content": []any{
							map[string]any{
								"type": "paragraph",
								"content": []any{
									map[string]any{"type": "text", "text": "Second"},
								},
							},
						},
					},
				},
			},
		},
	}
	got := ADFToMarkdown(doc)
	if !strings.Contains(got, "1. First") {
		t.Errorf("expected '1. First', got %q", got)
	}
	if !strings.Contains(got, "2. Second") {
		t.Errorf("expected '2. Second', got %q", got)
	}
}

func TestADFToMarkdown_CodeBlock(t *testing.T) {
	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type":  "codeBlock",
				"attrs": map[string]any{"language": "go"},
				"content": []any{
					map[string]any{"type": "text", "text": "fmt.Println(\"hi\")"},
				},
			},
		},
	}
	got := ADFToMarkdown(doc)
	if !strings.Contains(got, "```go") {
		t.Errorf("expected ```go fence, got %q", got)
	}
	if !strings.Contains(got, `fmt.Println("hi")`) {
		t.Errorf("expected code content, got %q", got)
	}
	if !strings.Contains(got, "```\n") {
		t.Errorf("expected closing fence, got %q", got)
	}
}

func TestADFToMarkdown_Blockquote(t *testing.T) {
	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type": "blockquote",
				"content": []any{
					map[string]any{
						"type": "paragraph",
						"content": []any{
							map[string]any{"type": "text", "text": "quoted text"},
						},
					},
				},
			},
		},
	}
	got := ADFToMarkdown(doc)
	if !strings.Contains(got, "> quoted text") {
		t.Errorf("expected blockquote, got %q", got)
	}
}

func TestADFToMarkdown_HardBreak(t *testing.T) {
	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{"type": "text", "text": "line1"},
					map[string]any{"type": "hardBreak"},
					map[string]any{"type": "text", "text": "line2"},
				},
			},
		},
	}
	got := ADFToMarkdown(doc)
	if !strings.Contains(got, "line1\nline2") {
		t.Errorf("expected hard break, got %q", got)
	}
}

func TestADFToMarkdown_Rule(t *testing.T) {
	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{"type": "rule"},
		},
	}
	got := ADFToMarkdown(doc)
	if !strings.Contains(got, "---") {
		t.Errorf("expected rule, got %q", got)
	}
}

func TestADFToMarkdown_Mention(t *testing.T) {
	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{
						"type":  "mention",
						"attrs": map[string]any{"text": "@Jane"},
					},
				},
			},
		},
	}
	got := ADFToMarkdown(doc)
	if !strings.Contains(got, "@Jane") {
		t.Errorf("expected mention, got %q", got)
	}
}

func TestADFToMarkdown_Table(t *testing.T) {
	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type": "table",
				"content": []any{
					map[string]any{
						"type": "tableRow",
						"content": []any{
							map[string]any{
								"type": "tableHeader",
								"content": []any{
									map[string]any{
										"type": "paragraph",
										"content": []any{
											map[string]any{"type": "text", "text": "Col1"},
										},
									},
								},
							},
							map[string]any{
								"type": "tableHeader",
								"content": []any{
									map[string]any{
										"type": "paragraph",
										"content": []any{
											map[string]any{"type": "text", "text": "Col2"},
										},
									},
								},
							},
						},
					},
					map[string]any{
						"type": "tableRow",
						"content": []any{
							map[string]any{
								"type": "tableCell",
								"content": []any{
									map[string]any{
										"type": "paragraph",
										"content": []any{
											map[string]any{"type": "text", "text": "A"},
										},
									},
								},
							},
							map[string]any{
								"type": "tableCell",
								"content": []any{
									map[string]any{
										"type": "paragraph",
										"content": []any{
											map[string]any{"type": "text", "text": "B"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	got := ADFToMarkdown(doc)
	if !strings.Contains(got, "| Col1 | Col2 |") {
		t.Errorf("expected table header, got %q", got)
	}
	if !strings.Contains(got, "| --- | --- |") {
		t.Errorf("expected table separator, got %q", got)
	}
	if !strings.Contains(got, "| A | B |") {
		t.Errorf("expected table row, got %q", got)
	}
}

func TestADFToMarkdown_Panel(t *testing.T) {
	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type":  "panel",
				"attrs": map[string]any{"panelType": "warning"},
				"content": []any{
					map[string]any{
						"type": "paragraph",
						"content": []any{
							map[string]any{"type": "text", "text": "Be careful"},
						},
					},
				},
			},
		},
	}
	got := ADFToMarkdown(doc)
	if !strings.Contains(got, "**[WARNING]**") {
		t.Errorf("expected panel type, got %q", got)
	}
	if !strings.Contains(got, "Be careful") {
		t.Errorf("expected panel content, got %q", got)
	}
}

func TestADFToMarkdown_InlineCard(t *testing.T) {
	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{
						"type":  "inlineCard",
						"attrs": map[string]any{"url": "https://example.com"},
					},
				},
			},
		},
	}
	got := ADFToMarkdown(doc)
	if !strings.Contains(got, "<https://example.com>") {
		t.Errorf("expected auto-link, got %q", got)
	}
}

func TestADFToMarkdown_UnknownNode(t *testing.T) {
	doc := map[string]any{
		"type": "doc",
		"content": []any{
			map[string]any{
				"type": "unknownType",
				"content": []any{
					map[string]any{"type": "text", "text": "fallback"},
				},
			},
		},
	}
	got := ADFToMarkdown(doc)
	if !strings.Contains(got, "fallback") {
		t.Errorf("expected children rendered for unknown node, got %q", got)
	}
}
