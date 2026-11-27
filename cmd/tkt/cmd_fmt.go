package main

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/node"
	"github.com/tektite-tracker/tektite/internal/store"
	"github.com/tektite-tracker/tektite/parse"
)

type fmtItem struct {
	Path    string   `json:"path"`
	Changes []string `json:"changes"`
	Written bool     `json:"written"`
	Skipped string   `json:"skipped,omitempty"`
}

func (a *app) fmtCmd() *cobra.Command {
	var dry bool
	cmd := &cobra.Command{
		Use:   "fmt",
		Short: "Assign missing ids and normalise frontmatter",
		Args:  exactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			r, err := a.repo()
			if err != nil {
				return err
			}
			snap, err := r.Load()
			if err != nil {
				return fmt.Errorf("load repository: %w", err)
			}
			gen := node.NewGenerator(a.rand)
			items := []fmtItem{}
			for _, nf := range snap.Nodes {
				item, data, err := a.formatNode(r, nf, gen)
				if err != nil {
					return err
				}
				if item == nil {
					continue
				}
				if data != nil && !dry {
					if err := r.ReplaceFile(nf.Path, data); err != nil {
						return fmt.Errorf("fmt: %w", err)
					}
					item.Written = true
				}
				items = append(items, *item)
			}
			if a.json {
				return a.writeJSON(items)
			}
			for _, it := range items {
				switch {
				case it.Skipped != "":
					a.printf(a.out, "skipped %s: %s\n", it.Path, it.Skipped)
				case it.Written:
					a.printf(a.out, "updated %s: %s\n", it.Path, strings.Join(it.Changes, ", "))
				default:
					a.printf(a.out, "would update %s: %s\n", it.Path, strings.Join(it.Changes, ", "))
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dry, "dry-run", false, "show the changes without writing")
	return cmd
}

func (a *app) formatNode(r *store.Repo, nf *store.NodeFile, gen *node.Generator) (*fmtItem, []byte, error) {
	for _, d := range nf.Diags {
		if d.Code == parse.CodeMalformedFrontmatter || d.Code == parse.CodeUnterminatedFrontmtr {
			return &fmtItem{Path: nf.Path, Changes: []string{}, Skipped: d.Message}, nil, nil
		}
	}
	src, err := r.ReadFile(nf.Path)
	if err != nil {
		return nil, nil, fmt.Errorf("fmt: %w", err)
	}
	n := nf.Node
	changes := []string{}
	id := n.ID
	if id == "" {
		if id, err = gen.New(a.now()); err != nil {
			return nil, nil, fmt.Errorf("new id: %w", err)
		}
		changes = append(changes, "assign id")
	}
	fields := []node.Field{{Key: "id", Value: id}, {Key: "kind", Value: n.Kind}, {Key: "title", Value: n.Title}, {Key: "parent", Value: n.Parent}}
	rawYAML, body, hadFrontmatter := parse.SplitFrontmatter(src)
	var out []byte
	if hadFrontmatter {
		out, err = node.Reformat(rawYAML, fields, string(body))
	} else {
		out, err = node.Render(fields, string(body))
	}
	if err != nil {
		return nil, nil, fmt.Errorf("fmt %s: %w", nf.Path, err)
	}
	if reason := changesValues(n, out); reason != "" {
		return &fmtItem{Path: nf.Path, Changes: []string{}, Skipped: reason}, nil, nil
	}
	if bytes.Equal(out, src) {
		return nil, nil, nil
	}
	if !hadFrontmatter {
		changes = append(changes, "add frontmatter")
	} else if len(changes) == 0 {
		changes = append(changes, "normalise frontmatter")
	}
	return &fmtItem{Path: nf.Path, Changes: changes}, out, nil
}

func changesValues(before *parse.Node, out []byte) string {
	after, _ := parse.ParseNode("x.md", out)
	if after.Body != before.Body {
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
