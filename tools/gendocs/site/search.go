package site

import (
	"html"
	"slices"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

func (b *builder) searchEntry(e *Entry, url string) searchItem {
	return searchItem{Title: e.Title, Aliases: slices.Concat(e.Keys, e.SearchNames), URL: url,
		Summary: plainMD(e.Summary), Text: searchProse(e.Body), Kind: e.Kind, Label: b.tr(kindName(e)),
		ID: e.ID, Icon: b.entryIcon(e), Version: b.docsVersion(), BuildID: b.buildID}
}

// Index explanations and headings, never scene dumps or executable examples.
// Link labels and inline identifiers remain useful; destinations and raw HTML
// are not article text and must not become accidental search keywords.
func searchProse(markdown string) string {
	source := []byte(markdown)
	doc := goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser().Parse(text.NewReader(source))
	var out strings.Builder
	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			if n.Type() == ast.TypeBlock {
				out.WriteByte(' ')
			}
			return ast.WalkContinue, nil
		}
		switch x := n.(type) {
		case *ast.CodeBlock, *ast.FencedCodeBlock, *ast.HTMLBlock, *ast.RawHTML, *ast.Image:
			return ast.WalkSkipChildren, nil
		case *ast.Text:
			out.Write(x.Segment.Value(source))
			if x.SoftLineBreak() || x.HardLineBreak() {
				out.WriteByte(' ')
			}
		case *ast.String:
			out.Write(x.Value)
		}
		return ast.WalkContinue, nil
	})
	return strings.Join(strings.Fields(html.UnescapeString(out.String())), " ")
}
