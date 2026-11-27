package parse

import (
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
)

const DefaultKind = "note"

type Node struct {
	ID          string         `json:"id"`
	Kind        string         `json:"kind"`
	Title       string         `json:"title"`
	Parent      string         `json:"parent"`
	Body        string         `json:"body"`
	Fields      map[string]any `json:"fields"`
	FieldOrder  []string       `json:"field_order"`
	Frontmatter bool           `json:"frontmatter"`
	Blocks      []Block        `json:"blocks"`
	Links       []Link         `json:"links"`
}

func ParseNode(name string, src []byte) (*Node, []Diagnostic) {
	src = normalize(src)
	diags := []Diagnostic{}
	n := &Node{Fields: map[string]any{}, FieldOrder: []string{}}

	sp := splitFrontmatter(src)
	body := sp.body
	text := sp.body
	var fmLinks []Link
	switch {
	case sp.unterminat:
		diags = append(diags, diag(CodeUnterminatedFrontmtr, "frontmatter is not closed by a --- line", 1, ""))
	case sp.present:
		fm, line, err := parseFrontmatter(sp.yaml)
		if err != nil {
			diags = append(diags, diag(CodeMalformedFrontmatter, "invalid YAML in frontmatter: "+err.Error(), line, ""))
			body = src
			break
		}
		n.Frontmatter = true
		fmLinks = frontmatterLinks(fm)
		duplicateKeyDiags(fm, &diags)
		n.Fields, n.FieldOrder = fm.fields, fm.keys
		n.ID = stringField(fm, "id", &diags)
		n.Kind = stringField(fm, "kind", &diags)
		n.Title = stringField(fm, "title", &diags)
		n.Parent = stringField(fm, "parent", &diags)
		if _, ok := fm.fields["status"]; ok {
			diags = append(diags, diag(CodeStatusInFrontmatter, "status belongs in events and is ignored in frontmatter", 0, "status"))
		}
	}

	res := parseBody(text, sp.bodyLine)
	n.Body = strings.TrimSpace(string(body))
	n.Blocks, n.Links = res.blocks, mergeLinks(fmLinks, res.links)
	if n.Blocks == nil {
		n.Blocks = []Block{}
	}
	if n.Links == nil {
		n.Links = []Link{}
	}

	partial := !n.Frontmatter || n.Kind == "" || n.Title == ""
	if n.ID == "" {
		sev := SeverityWarning
		if partial {
			sev = SeverityNote
		}
		diags = append(diags, diagAt(sev, CodeMissingID, "node has no id", 0, "id"))
	}
	if n.Kind == "" {
		n.Kind = DefaultKind
		if n.Frontmatter {
			diags = append(diags, diagAt(SeverityNote, CodeMissingKind, "node has no kind, defaulting to "+DefaultKind, 0, "kind"))
		}
	}
	if n.Title == "" {
		n.Title = res.heading
		if n.Title == "" {
			n.Title = strings.TrimSuffix(path.Base(name), path.Ext(name))
		}
		if n.Frontmatter {
			diags = append(diags, diag(CodeMissingTitle, "node has no title", 0, "title"))
		}
	}
	return n, diags
}

func ParseNodeReader(name string, r io.Reader) (*Node, []Diagnostic, error) {
	src, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, fmt.Errorf("read source: %w", err)
	}
	n, d := ParseNode(name, src)
	return n, d, nil
}

var codeSpan = regexp.MustCompile("`[^`\\n]*`")

func frontmatterLinks(fm frontmatter) []Link {
	var out []Link
	seen := map[string]bool{}
	for _, v := range fm.strings {
		masked := codeSpan.ReplaceAllStringFunc(v.text, func(s string) string { return strings.Repeat(" ", len(s)) })
		for _, m := range wikilink.FindAllStringSubmatchIndex(masked, -1) {
			target := strings.TrimSpace(masked[m[2]:m[3]])
			if target == "" || seen[target] {
				continue
			}
			seen[target] = true
			out = append(out, Link{Target: target, Line: v.line + strings.Count(masked[:m[0]], "\n")})
		}
	}
	return out
}

func mergeLinks(first, rest []Link) []Link {
	seen := map[string]bool{}
	var out []Link
	for _, l := range append(append([]Link{}, first...), rest...) {
		if !seen[l.Target] {
			seen[l.Target] = true
			out = append(out, l)
		}
	}
	return out
}
