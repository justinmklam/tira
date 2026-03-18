package api

import (
	"fmt"
	"strings"
)

// ADFToMarkdown converts an Atlassian Document Format node (as parsed from JSON
// into map[string]any) into a Markdown string.
func ADFToMarkdown(node map[string]any) string {
	var sb strings.Builder
	renderNode(&sb, node, 0)
	return sb.String()
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
				sb.WriteString(fmt.Sprintf("%s%d. ", strings.Repeat("  ", listDepth), i+1))
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
