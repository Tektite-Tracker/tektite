package store

import (
	"slices"

	"github.com/tektite-tracker/tektite/internal/node"
)

type Tree struct {
	Parents  map[string]string
	Children map[string][]string
	Roots    []string
	Cycles   [][]string
}

type Visit struct {
	Ref   string
	Depth int
}

func buildTree(refs []string, parents map[string]string) *Tree {
	t := &Tree{Parents: parents, Children: map[string][]string{}, Roots: []string{}}
	for _, ref := range refs {
		p := parents[ref]
		if p == "" {
			t.Roots = append(t.Roots, ref)
			continue
		}
		t.Children[p] = append(t.Children[p], ref)
	}
	for _, kids := range t.Children {
		slices.Sort(kids)
	}
	slices.Sort(t.Roots)
	t.Cycles = node.Cycles(parents)
	return t
}

func (t *Tree) Ancestors(ref string) ([]string, bool) {
	return node.Ancestors(t.Parents, ref)
}

func (t *Tree) Descendants(ref string) []Visit {
	out := []Visit{}
	seen := map[string]bool{ref: true}
	var walk func(string, int)
	walk = func(cur string, depth int) {
		for _, kid := range t.Children[cur] {
			if seen[kid] {
				continue
			}
			seen[kid] = true
			out = append(out, Visit{Ref: kid, Depth: depth})
			walk(kid, depth+1)
		}
	}
	walk(ref, 1)
	return out
}
