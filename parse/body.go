package parse

import (
	"bytes"
	"regexp"
	"slices"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

type Block struct {
	ID      string `json:"id,omitempty"`
	Marker  string `json:"marker,omitempty"`
	Text    string `json:"text"`
	Checked bool   `json:"checked"`
	Gap     bool   `json:"gap"`
	Line    int    `json:"line"`
}

type Link struct {
	Rel     string `json:"rel"`
	Target  string `json:"target"`
	Line    int    `json:"line"`
	Display string `json:"display,omitempty"`
	Block   string `json:"block,omitempty"`
}

var (
	blockID  = regexp.MustCompile(`<!--\s*([A-Za-z]+):([0-9A-Za-z]+)\s*-->`)
	checkbox = regexp.MustCompile(`^\[[ xX]\]\s*`)
	wikilink = regexp.MustCompile(`(?:([A-Za-z][A-Za-z0-9-]*)::[ \t]*)?\[\[([^\[\]|#\n]+?)(?:#([^\[\]|\n]*))?(?:\|([^\[\]\n]*))?\]\]`)
)

var md = goldmark.New(goldmark.WithExtensions(extension.TaskList))

type bodyResult struct {
	blocks  []Block
	links   []Link
	heading string
}

func parseBody(body []byte, firstLine int) bodyResult {
	reader := text.NewReader(body)
	doc := md.Parser().Parse(reader)
	lines := lineIndex(body)
	lineAt := func(offset int) int {
		i, _ := slices.BinarySearch(lines, offset+1)
		return firstLine + i - 1
	}
	res := bodyResult{}
	masked := append([]byte(nil), body...)
	blank := func(start, stop int) {
		for i := start; i < stop && i < len(masked); i++ {
			if masked[i] != '\n' {
				masked[i] = ' '
			}
		}
	}

	gapLevel := 0
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := n.(type) {
		case *ast.Heading:
			if res.heading == "" {
				res.heading = strings.TrimSpace(blockText(t, body))
			}
			if gapLevel > 0 && t.Level <= gapLevel {
				gapLevel = 0
			}
			if gapLevel == 0 && strings.EqualFold(strings.TrimSpace(blockText(t, body)), "gaps") {
				gapLevel = t.Level
			}
		case *ast.CodeSpan:
			for c := t.FirstChild(); c != nil; c = c.NextSibling() {
				if seg, ok := c.(*ast.Text); ok {
					blank(seg.Segment.Start, seg.Segment.Stop)
				}
			}
		case *ast.FencedCodeBlock, *ast.CodeBlock, *ast.HTMLBlock:
			ls := n.Lines()
			for i := 0; i < ls.Len(); i++ {
				seg := ls.At(i)
				blank(seg.Start, seg.Stop)
			}
		case *ast.ListItem:
			if b, ok := taskBlock(t, body, lineAt); ok {
				b.Gap = gapLevel > 0 || b.Marker == "g"
				res.blocks = append(res.blocks, b)
			}
		}
		return ast.WalkContinue, nil
	})

	res.links = scanWikilinks(string(masked), lineAt)
	return res
}

func lineIndex(body []byte) []int {
	idx := []int{0}
	for i, c := range body {
		if c == '\n' {
			idx = append(idx, i+1)
		}
	}
	return idx
}

func blockText(n ast.Node, body []byte) string {
	var buf bytes.Buffer
	ls := n.Lines()
	for i := 0; i < ls.Len(); i++ {
		seg := ls.At(i)
		if i > 0 {
			buf.WriteByte(' ')
		}
		buf.Write(bytes.TrimSpace(body[seg.Start:seg.Stop]))
	}
	return buf.String()
}

func taskBlock(item *ast.ListItem, body []byte, lineAt func(int) int) (Block, bool) {
	first := item.FirstChild()
	if first == nil {
		return Block{}, false
	}
	box, ok := first.FirstChild().(*extast.TaskCheckBox)
	if !ok {
		return Block{}, false
	}
	raw := blockText(first, body)
	raw = checkbox.ReplaceAllString(raw, "")
	b := Block{Checked: box.IsChecked}
	if m := blockID.FindStringSubmatch(raw); m != nil {
		b.Marker, b.ID = m[1], m[2]
		raw = blockID.ReplaceAllString(raw, "")
	}
	b.Text = strings.Join(strings.Fields(raw), " ")
	if ls := first.Lines(); ls.Len() > 0 {
		b.Line = lineAt(ls.At(0).Start)
	}
	return b, true
}

func scanWikilinks(masked string, lineAt func(int) int) []Link {
	var out []Link
	seen := map[Link]bool{}
	for _, m := range wikilink.FindAllStringSubmatchIndex(masked, -1) {
		sub := func(i int) string {
			if m[2*i] < 0 {
				return ""
			}
			return strings.TrimSpace(masked[m[2*i]:m[2*i+1]])
		}
		l := Link{Rel: strings.ToLower(sub(1)), Target: sub(2), Block: sub(3), Display: sub(4)}
		if l.Target == "" {
			continue
		}
		if l.Rel == "" {
			l.Rel = RelMentions
		}
		key := Link{Rel: l.Rel, Target: l.Target}
		if seen[key] {
			continue
		}
		seen[key] = true
		l.Line = lineAt(m[0])
		out = append(out, l)
	}
	return out
}
