package messageformat

import (
	"bytes"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extensionast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

var markdownParser = goldmark.New(goldmark.WithExtensions(extension.Table, extension.Strikethrough))

func MarkdownPlainText(content string) (string, error) {
	source := []byte(content)
	document := markdownParser.Parser().Parse(text.NewReader(source))
	var buffer bytes.Buffer

	err := ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			appendNodeText(&buffer, node, source)
			return ast.WalkContinue, nil
		}
		if node.Kind() == extensionast.KindTableCell {
			appendCellBreak(&buffer)
		}
		if isLineBoundary(node.Kind()) {
			appendLineBreak(&buffer)
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return "", err
	}
	return normalizeText(buffer.String()), nil
}

func appendNodeText(buffer *bytes.Buffer, node ast.Node, source []byte) {
	switch typedNode := node.(type) {
	case *ast.Text:
		buffer.Write(typedNode.Value(source))
		if typedNode.SoftLineBreak() || typedNode.HardLineBreak() {
			appendLineBreak(buffer)
		}
	case *ast.String:
		buffer.Write(typedNode.Value)
	case *ast.CodeSpan:
		buffer.Write(typedNode.Text(source))
	case *ast.AutoLink:
		buffer.Write(typedNode.Label(source))
	case *ast.CodeBlock:
		appendLineBreak(buffer)
		buffer.Write(typedNode.Text(source))
	case *ast.FencedCodeBlock:
		appendLineBreak(buffer)
		buffer.Write(typedNode.Text(source))
	}
}

func isLineBoundary(kind ast.NodeKind) bool {
	switch kind {
	case ast.KindBlockquote, ast.KindCodeBlock, ast.KindFencedCodeBlock, ast.KindHeading,
		ast.KindListItem, ast.KindParagraph, ast.KindThematicBreak, ast.KindTextBlock,
		extensionast.KindTable, extensionast.KindTableHeader, extensionast.KindTableRow:
		return true
	default:
		return false
	}
}

func appendCellBreak(buffer *bytes.Buffer) {
	if buffer.Len() == 0 {
		return
	}
	current := buffer.String()
	if strings.HasSuffix(current, "\n") || strings.HasSuffix(current, " ") {
		return
	}
	buffer.WriteByte(' ')
}

func appendLineBreak(buffer *bytes.Buffer) {
	if buffer.Len() == 0 || strings.HasSuffix(buffer.String(), "\n") {
		return
	}
	buffer.WriteByte('\n')
}

func normalizeText(content string) string {
	rawLines := strings.Split(content, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return strings.Join(lines, "\n")
}
