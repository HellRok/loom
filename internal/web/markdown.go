package web

import (
	"bytes"
	"html"
	"html/template"
	"regexp"
	"strconv"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// issueRefRE finds `#N` and `slug#N` in running text. Group 1 is the
// character before the ref, 2 the optional Slug and 3 the number.
var issueRefRE = regexp.MustCompile(`(^|[^A-Za-z0-9_&/#-])([a-z0-9][a-z0-9-]*)?#([0-9]+)\b`)

// renderMarkdown renders body as GitHub-flavoured Markdown. Raw HTML is
// escaped, not passed through, and `#N` / `slug#N` link to Issue pages
// (`#N` meaning an Issue in slug's Project).
func renderMarkdown(slug, body string) template.HTML {
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithASTTransformers(
			util.Prioritized(&refLinker{slug: slug}, 100),
		)),
		goldmark.WithRendererOptions(renderer.WithNodeRenderers(
			util.Prioritized(escapeRawHTML{}, 100),
		)),
	)
	var buf bytes.Buffer
	if err := md.Convert([]byte(body), &buf); err != nil {
		return template.HTML("<pre>" + html.EscapeString(body) + "</pre>") //nolint:gosec // escaped
	}
	return template.HTML(buf.String()) //nolint:gosec // goldmark output with raw HTML escaped
}

// escapeRawHTML renders raw HTML nodes as visible, escaped text.
type escapeRawHTML struct{}

func (escapeRawHTML) RegisterFuncs(r renderer.NodeRendererFuncRegisterer) {
	r.Register(ast.KindHTMLBlock, func(w util.BufWriter, src []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			b := n.(*ast.HTMLBlock)
			w.WriteString("<p>")
			lines := b.Lines()
			for i := 0; i < lines.Len(); i++ {
				seg := lines.At(i)
				w.WriteString(html.EscapeString(string(seg.Value(src))))
			}
			if b.HasClosure() {
				seg := b.ClosureLine
				w.WriteString(html.EscapeString(string(seg.Value(src))))
			}
			w.WriteString("</p>\n")
		}
		return ast.WalkSkipChildren, nil
	})
	r.Register(ast.KindRawHTML, func(w util.BufWriter, src []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			r := n.(*ast.RawHTML)
			for i := 0; i < r.Segments.Len(); i++ {
				seg := r.Segments.At(i)
				w.WriteString(html.EscapeString(string(seg.Value(src))))
			}
		}
		return ast.WalkSkipChildren, nil
	})
}

// refLinker turns issue refs in text into links.
type refLinker struct{ slug string }

func (l *refLinker) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	src := reader.Source()
	var texts []*ast.Text
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n.(type) {
		case *ast.Link, *ast.AutoLink, *ast.CodeSpan, *ast.Image:
			return ast.WalkSkipChildren, nil
		}
		if t, ok := n.(*ast.Text); ok {
			texts = append(texts, t)
		}
		return ast.WalkContinue, nil
	})
	for _, t := range texts {
		l.link(t, src)
	}
}

// link replaces t with text and link nodes if it holds issue refs.
func (l *refLinker) link(t *ast.Text, src []byte) {
	seg := t.Segment
	content := src[seg.Start:seg.Stop]
	matches := issueRefRE.FindAllSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return
	}
	parent := t.Parent()
	pos := 0
	// emit inserts a node before t, then the original is trimmed or removed.
	emit := func(n ast.Node) { parent.InsertBefore(parent, t, n) }
	for _, m := range matches {
		refStart := m[3] // end of group 1: where the ref begins
		if m[4] >= 0 {
			refStart = m[4]
		}
		if refStart > pos {
			emit(ast.NewTextSegment(text.NewSegment(seg.Start+pos, seg.Start+refStart)))
		}
		slug := l.slug
		if m[4] >= 0 {
			slug = string(content[m[4]:m[5]])
		}
		digits := string(content[m[6]:m[7]])
		if _, err := strconv.ParseInt(digits, 10, 64); err != nil {
			// Too big to be an Issue number: leave it as text.
			continue
		}
		link := ast.NewLink()
		link.Destination = []byte("/" + slug + "/issues/" + digits)
		link.AppendChild(link, ast.NewTextSegment(text.NewSegment(seg.Start+refStart, seg.Start+m[7])))
		emit(link)
		pos = m[1]
	}
	if pos < len(content) || t.SoftLineBreak() || t.HardLineBreak() {
		rest := ast.NewTextSegment(text.NewSegment(seg.Start+pos, seg.Stop))
		rest.SetSoftLineBreak(t.SoftLineBreak())
		rest.SetHardLineBreak(t.HardLineBreak())
		emit(rest)
	}
	parent.RemoveChild(parent, t)
}
