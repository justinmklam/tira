package api

import (
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// ADFToMarkdown converts an Atlassian Document Format node (as parsed from JSON
// into map[string]any) into a Markdown string.
func ADFToMarkdown(node map[string]any) string {
	var sb strings.Builder
	renderNode(&sb, node, 0)
	return sb.String()
}

// markdownToADF converts Markdown text to Atlassian Document Format (ADF).
func markdownToADF(input string) map[string]any {
	if input == "" {
		return map[string]any{
			"version": 1,
			"type":    "doc",
			"content": []any{},
		}
	}

	md := goldmark.New()
	source := []byte(input)
	reader := text.NewReader(source)
	doc := md.Parser().Parse(reader)

	converter := &adfConverter{source: source}
	converter.walkNode(doc)

	return map[string]any{
		"version": 1,
		"type":    "doc",
		"content": converter.content,
	}
}

// adfConverter converts Markdown AST to ADF
type adfConverter struct {
	source  []byte
	content []any
}

func (c *adfConverter) walkNode(node ast.Node) {
	switch n := node.(type) {
	case *ast.Document:
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			c.walkNode(child)
		}

	case *ast.Paragraph:
		para := c.buildParagraph(node)
		if para != nil {
			c.content = append(c.content, para)
		}

	case *ast.Heading:
		heading := c.buildHeading(node, n.Level)
		if heading != nil {
			c.content = append(c.content, heading)
		}

	case *ast.List:
		list := c.buildList(node, n)
		if list != nil {
			c.content = append(c.content, list)
		}

	case *ast.CodeBlock:
		code := c.buildCodeBlock(node, "")
		if code != nil {
			c.content = append(c.content, code)
		}

	case *ast.FencedCodeBlock:
		language := ""
		if n.Info != nil {
			language = string(n.Language(c.source))
		}
		code := c.buildCodeBlock(node, language)
		if code != nil {
			c.content = append(c.content, code)
		}

	case *ast.Blockquote:
		quote := c.buildBlockquote(node)
		if quote != nil {
			c.content = append(c.content, quote)
		}

	case *ast.ThematicBreak:
		c.content = append(c.content, map[string]any{
			"type": "rule",
		})

	case *ast.HTMLBlock:
		// Skip HTML blocks

	case *ast.Text:
		// Handled by parent nodes

	default:
		// Handle other node types
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			c.walkNode(child)
		}
	}
}

func (c *adfConverter) buildParagraph(node ast.Node) map[string]any {
	textContent := c.buildInlineContent(node)
	if len(textContent) == 0 {
		return nil
	}
	return map[string]any{
		"type": "paragraph",
		"content": []any{
			map[string]any{
				"type": "text",
				"text": textContent,
			},
		},
	}
}

func (c *adfConverter) buildHeading(node ast.Node, level int) map[string]any {
	textContent := c.buildInlineContent(node)
	if textContent == "" {
		return nil
	}
	return map[string]any{
		"type": "heading",
		"attrs": map[string]any{
			"level": level,
		},
		"content": []any{
			map[string]any{
				"type": "text",
				"text": textContent,
			},
		},
	}
}

func (c *adfConverter) buildList(node ast.Node, listNode *ast.List) map[string]any {
	listType := "bulletList"
	if listNode.IsOrdered() {
		listType = "orderedList"
	}

	items := []any{}
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if listItem, ok := child.(*ast.ListItem); ok {
			itemContent := c.buildInlineContent(listItem)
			if itemContent != "" {
				items = append(items, map[string]any{
					"type": "listItem",
					"content": []any{
						map[string]any{
							"type": "paragraph",
							"content": []any{
								map[string]any{
									"type": "text",
									"text": itemContent,
								},
							},
						},
					},
				})
			}
		}
	}

	if len(items) == 0 {
		return nil
	}

	return map[string]any{
		"type":    listType,
		"content": items,
	}
}

func (c *adfConverter) buildCodeBlock(node ast.Node, language string) map[string]any {
	var codeText string

	if fenced, ok := node.(*ast.FencedCodeBlock); ok {
		// Use Lines to get the code content
		var lines []string
		for i := 0; i < fenced.Lines().Len(); i++ {
			segment := fenced.Lines().At(i)
			lines = append(lines, string(segment.Value(c.source)))
		}
		codeText = strings.Join(lines, "\n")
	} else {
		var codeLines []string
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			if textNode, ok := child.(*ast.Text); ok {
				codeLines = append(codeLines, string(textNode.Segment.Value(c.source)))
			}
		}
		codeText = strings.Join(codeLines, "\n")
	}

	if codeText == "" {
		return nil
	}

	content := []any{
		map[string]any{
			"type": "text",
			"text": codeText,
		},
	}

	adf := map[string]any{
		"type": "codeBlock",
		"attrs": map[string]any{
			"language": language,
		},
		"content": content,
	}

	return adf
}

func (c *adfConverter) buildBlockquote(node ast.Node) map[string]any {
	textContent := c.buildInlineContent(node)
	if textContent == "" {
		return nil
	}
	return map[string]any{
		"type": "blockquote",
		"content": []any{
			map[string]any{
				"type": "paragraph",
				"content": []any{
					map[string]any{
						"type": "text",
						"text": textContent,
					},
				},
			},
		},
	}
}

func (c *adfConverter) buildInlineContent(node ast.Node) string {
	var sb strings.Builder
	c.collectInlineText(&sb, node)
	return sb.String()
}

func (c *adfConverter) collectInlineText(sb *strings.Builder, node ast.Node) {
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Text:
			sb.Write(n.Segment.Value(c.source))
		case *ast.Emphasis:
			c.collectInlineText(sb, child)
		case *ast.CodeSpan:
			for gc := child.FirstChild(); gc != nil; gc = gc.NextSibling() {
				if textNode, ok := gc.(*ast.Text); ok {
					sb.Write(textNode.Segment.Value(c.source))
				}
			}
		case *ast.Link:
			// For links, just include the text content
			c.collectInlineText(sb, child)
		case *ast.AutoLink:
			sb.Write(n.URL(c.source))
		default:
			c.collectInlineText(sb, child)
		}
	}
}

func renderNode(sb *strings.Builder, node map[string]any, listDepth int) {
	nodeType, _ := node["type"].(string)
	content, _ := node["content"].([]any)
	attrs, _ := node["attrs"].(map[string]any)

	switch nodeType {
	case "doc":
		renderChildren(sb, content, listDepth)

	case "paragraph":
		renderChildren(sb, content, listDepth)
		sb.WriteString("\n\n")

	case "heading":
		level := 1
		if l, ok := attrs["level"].(float64); ok {
			level = int(l)
		}
		sb.WriteString(strings.Repeat("#", level) + " ")
		renderChildren(sb, content, listDepth)
		sb.WriteString("\n\n")

	case "text":
		text, _ := node["text"].(string)
		marks, _ := node["marks"].([]any)
		text = applyMarks(text, marks)
		sb.WriteString(text)

	case "hardBreak":
		sb.WriteString("\n")

	case "rule":
		sb.WriteString("\n---\n\n")

	case "blockquote":
		var inner strings.Builder
		renderChildren(&inner, content, listDepth)
		for _, line := range strings.Split(strings.TrimRight(inner.String(), "\n"), "\n") {
			sb.WriteString("> " + line + "\n")
		}
		sb.WriteString("\n")

	case "bulletList":
		for _, item := range content {
			if m, ok := item.(map[string]any); ok {
				sb.WriteString(strings.Repeat("  ", listDepth) + "- ")
				renderListItem(sb, m, listDepth+1)
			}
		}
		if listDepth == 0 {
			sb.WriteString("\n")
		}

	case "orderedList":
		for i, item := range content {
			if m, ok := item.(map[string]any); ok {
				fmt.Fprintf(sb, "%s%d. ", strings.Repeat("  ", listDepth), i+1)
				renderListItem(sb, m, listDepth+1)
			}
		}
		if listDepth == 0 {
			sb.WriteString("\n")
		}

	case "codeBlock":
		lang := ""
		if l, ok := attrs["language"].(string); ok {
			lang = l
		}
		var code strings.Builder
		renderChildren(&code, content, listDepth)
		sb.WriteString("```" + lang + "\n")
		sb.WriteString(strings.TrimRight(code.String(), "\n"))
		sb.WriteString("\n```\n\n")

	case "inlineCard", "blockCard":
		if url, ok := attrs["url"].(string); ok {
			sb.WriteString("<" + url + ">")
		}

	case "mention":
		if text, ok := attrs["text"].(string); ok {
			sb.WriteString(text)
		} else if id, ok := attrs["id"].(string); ok {
			sb.WriteString("@" + id)
		}

	case "emoji":
		if text, ok := attrs["text"].(string); ok {
			sb.WriteString(text)
		} else if shortName, ok := attrs["shortName"].(string); ok {
			sb.WriteString(shortName)
		}

	case "panel":
		panelType := "info"
		if pt, ok := attrs["panelType"].(string); ok {
			panelType = pt
		}
		sb.WriteString("> **[" + strings.ToUpper(panelType) + "]**\n")
		var inner strings.Builder
		renderChildren(&inner, content, listDepth)
		for _, line := range strings.Split(strings.TrimRight(inner.String(), "\n"), "\n") {
			sb.WriteString("> " + line + "\n")
		}
		sb.WriteString("\n")

	case "mediaSingle", "media":
		// skip attachments/images

	case "table":
		renderTable(sb, content)

	default:
		// unknown node — render children if any
		renderChildren(sb, content, listDepth)
	}
}

// renderListItem renders the inline content on the same line, then any nested
// lists or block elements on subsequent lines.
func renderListItem(sb *strings.Builder, item map[string]any, listDepth int) {
	content, _ := item["content"].([]any)
	// 4-space continuation indent keeps block elements (code blocks, etc.)
	// inside the list item under CommonMark rules.
	indent := strings.Repeat("    ", listDepth)
	first := true
	for _, child := range content {
		m, ok := child.(map[string]any)
		if !ok {
			continue
		}
		childType, _ := m["type"].(string)
		switch childType {
		case "bulletList", "orderedList":
			if first {
				sb.WriteString("\n")
			}
			renderNode(sb, m, listDepth)
		case "codeBlock":
			// Blank line + indented fence to stay inside the list item.
			sb.WriteString("\n")
			var inner strings.Builder
			renderNode(&inner, m, listDepth)
			for _, line := range strings.Split(strings.TrimRight(inner.String(), "\n"), "\n") {
				if line == "" {
					sb.WriteString("\n")
				} else {
					sb.WriteString(indent + line + "\n")
				}
			}
			sb.WriteString("\n")
		default:
			var inner strings.Builder
			renderNode(&inner, m, listDepth)
			line := strings.TrimRight(inner.String(), "\n")
			if first {
				sb.WriteString(line + "\n")
			} else {
				sb.WriteString(indent + line + "\n")
			}
		}
		first = false
	}
}

func renderChildren(sb *strings.Builder, content []any, listDepth int) {
	for _, child := range content {
		if m, ok := child.(map[string]any); ok {
			renderNode(sb, m, listDepth)
		}
	}
}

func applyMarks(text string, marks []any) string {
	for _, raw := range marks {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		markType, _ := m["type"].(string)
		attrs, _ := m["attrs"].(map[string]any)
		switch markType {
		case "strong":
			text = "**" + text + "**"
		case "em":
			text = "_" + text + "_"
		case "code":
			text = "`" + text + "`"
		case "strike":
			text = "~~" + text + "~~"
		case "link":
			href, _ := attrs["href"].(string)
			if href != "" {
				text = "[" + text + "](" + href + ")"
			}
		case "underline", "textColor", "subsup":
			// no markdown equivalent — leave text as-is
		}
	}
	return text
}

func renderTable(sb *strings.Builder, rows []any) {
	var header []string
	firstRow := true
	for _, rawRow := range rows {
		row, ok := rawRow.(map[string]any)
		if !ok {
			continue
		}
		cells, _ := row["content"].([]any)
		var cols []string
		for _, rawCell := range cells {
			cell, ok := rawCell.(map[string]any)
			if !ok {
				continue
			}
			var inner strings.Builder
			renderChildren(&inner, cell["content"].([]any), 0)
			cols = append(cols, strings.TrimSpace(strings.ReplaceAll(inner.String(), "\n", " ")))
		}
		sb.WriteString("| " + strings.Join(cols, " | ") + " |\n")
		if firstRow {
			header = make([]string, len(cols))
			for i := range cols {
				header[i] = "---"
			}
			sb.WriteString("| " + strings.Join(header, " | ") + " |\n")
			firstRow = false
		}
	}
	sb.WriteString("\n")
}
