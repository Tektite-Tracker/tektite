package service

import (
	"bytes"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/tektite-tracker/tektite/internal/node"
	"github.com/tektite-tracker/tektite/internal/store"
	"github.com/tektite-tracker/tektite/parse"
)

type FormatItem struct {
	Path    string   `json:"path"`
	Changes []string `json:"changes"`
	Written bool     `json:"written"`
	Skipped string   `json:"skipped,omitempty"`
}

var checklistLine = regexp.MustCompile(`^(\s*(?:[-*+]|\d+[.)])\s+\[[ xX]\]\s*)(.*)$`)

const blockIDLen = 8

func (s *Service) Format(dryRun bool) ([]FormatItem, error) {
	snap, err := s.load()
	if err != nil {
		return nil, err
	}
	gen := node.NewGenerator(s.Rand)
	items := []FormatItem{}
	for _, nf := range snap.Nodes {
		item, data, err := s.formatNode(nf, gen)
		if err != nil {
			return nil, err
		}
		if item == nil {
			continue
		}
		if data != nil && !dryRun {
			if err := s.Repo.ReplaceFile(nf.Path, data); err != nil {
				return nil, fmt.Errorf("fmt: %w", err)
			}
			item.Written = true
		}
		items = append(items, *item)
	}
	return items, nil
}

func (s *Service) formatNode(nf *store.NodeFile, gen *node.Generator) (*FormatItem, []byte, error) {
	for _, d := range nf.Diags {
		if d.Code == parse.CodeMalformedFrontmatter || d.Code == parse.CodeUnterminatedFrontmtr {
			return &FormatItem{Path: nf.Path, Changes: []string{}, Skipped: d.Message}, nil, nil
		}
	}
	src, err := s.Repo.ReadFile(nf.Path)
	if err != nil {
		return nil, nil, fmt.Errorf("fmt: %w", err)
	}
	n := nf.Node
	changes := []string{}
	id := n.ID
	if id == "" {
		if id, err = gen.New(s.Now()); err != nil {
			return nil, nil, fmt.Errorf("new id: %w", err)
		}
		changes = append(changes, "assign id")
	}
	fields := []node.Field{{Key: "id", Value: id}, {Key: "kind", Value: n.Kind}, {Key: "title", Value: n.Title}, {Key: "parent", Value: n.Parent}}
	rawYAML, body, hadFrontmatter := parse.SplitFrontmatter(src)
	text, added, err := addBlockIDs(string(body), gen, s.Now)
	if err != nil {
		return nil, nil, err
	}
	if len(added) > 0 {
		changes = append(changes, "assign block ids")
	}
	var out []byte
	if hadFrontmatter {
		out, err = node.Reformat(rawYAML, fields, text)
	} else {
		out, err = node.Render(fields, text)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("fmt %s: %w", nf.Path, err)
	}
	if reason := changesValues(n, out, added); reason != "" {
		return &FormatItem{Path: nf.Path, Changes: []string{}, Skipped: reason}, nil, nil
	}
	if bytes.Equal(out, src) {
		return nil, nil, nil
	}
	if !hadFrontmatter {
		changes = append(changes, "add frontmatter")
	} else if len(changes) == 0 {
		changes = append(changes, "normalise frontmatter")
	}
	return &FormatItem{Path: nf.Path, Changes: changes}, out, nil
}

func addBlockIDs(body string, gen *node.Generator, now func() time.Time) (string, []string, error) {
	lines := strings.Split(body, "\n")
	added := []string{}
	for _, b := range parse.BodyBlocks([]byte(body)) {
		if b.ID != "" || b.Line < 1 || b.Line > len(lines) {
			continue
		}
		m := checklistLine.FindStringSubmatch(lines[b.Line-1])
		if m == nil {
			continue
		}
		ulid, err := gen.New(now())
		if err != nil {
			return "", nil, fmt.Errorf("new block id: %w", err)
		}
		marker := "t"
		if b.Gap {
			marker = "g"
		}
		comment := fmt.Sprintf("<!-- %s:%s -->", marker, ulid[len(ulid)-blockIDLen:])
		lines[b.Line-1] = m[1] + strings.TrimRight(comment+" "+m[2], " ")
		added = append(added, comment)
	}
	return strings.Join(lines, "\n"), added, nil
}

func changesValues(before *parse.Node, out []byte, added []string) string {
	after, _ := parse.ParseNode("x.md", out)
	body := after.Body
	for _, comment := range added {
		body = strings.Replace(body, comment+" ", "", 1)
		body = strings.Replace(body, comment, "", 1)
	}
	if body != before.Body {
		return "formatting would change the body"
	}
	for k, v := range before.Fields {
		switch k {
		case "id", "kind", "title", "parent":
			continue
		}
		if !reflect.DeepEqual(v, after.Fields[k]) {
			return "formatting would change the value of a field"
		}
	}
	if len(after.Fields) < len(before.Fields) {
		return "formatting would drop a field"
	}
	return ""
}
