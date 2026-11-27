package store

import (
	"sort"
	"strings"

	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/parse"
)

type Backlink struct {
	Rel  string `json:"rel"`
	Ref  string `json:"ref"`
	Line int    `json:"line"`
}

type Graph struct {
	Out      map[string][]parse.Link
	In       map[string][]Backlink
	Blockers map[string][]string
	States   map[string]event.State
}

func (s *Snapshot) BuildGraph() *Graph {
	g := &Graph{
		Out:      map[string][]parse.Link{},
		In:       map[string][]Backlink{},
		Blockers: map[string][]string{},
		States:   map[string]event.State{},
	}
	for _, nf := range s.Nodes {
		st := event.Fold(s.EventsOf(nf.Ref), event.Options{})
		g.States[nf.Ref] = st
		g.Out[nf.Ref] = s.foldLinks(nf.Node.Links, st.LinkOps)
	}
	seenIn := map[string]bool{}
	seenBlock := map[[2]string]bool{}
	for _, nf := range s.Nodes {
		for _, l := range g.Out[nf.Ref] {
			t, err := s.Resolve(l.Target)
			if err != nil || t.Ref == nf.Ref {
				continue
			}
			key := nf.Ref + "\x00" + l.Rel + "\x00" + t.Ref
			if !seenIn[key] {
				seenIn[key] = true
				g.In[t.Ref] = append(g.In[t.Ref], Backlink{Rel: parse.Inverse(l.Rel), Ref: nf.Ref, Line: l.Line})
			}
			blocked, blocker := "", ""
			switch l.Rel {
			case "blocks":
				blocked, blocker = t.Ref, nf.Ref
			case "blocked-by":
				blocked, blocker = nf.Ref, t.Ref
			}
			if blocked != "" && !seenBlock[[2]string{blocked, blocker}] {
				seenBlock[[2]string{blocked, blocker}] = true
				g.Blockers[blocked] = append(g.Blockers[blocked], blocker)
			}
		}
	}
	for _, list := range g.In {
		sort.SliceStable(list, func(i, j int) bool {
			if list[i].Ref != list[j].Ref {
				return list[i].Ref < list[j].Ref
			}
			return list[i].Rel < list[j].Rel
		})
	}
	for _, list := range g.Blockers {
		sort.Strings(list)
	}
	return g
}

func (s *Snapshot) linkKey(rel, target string) string {
	t := strings.TrimSpace(target)
	if n, err := s.Resolve(t); err == nil {
		t = n.Ref
	}
	return strings.ToLower(strings.TrimSpace(rel)) + "\x00" + strings.ToLower(t)
}

func (s *Snapshot) foldLinks(declared []parse.Link, ops []event.LinkOp) []parse.Link {
	out := append([]parse.Link{}, declared...)
	for _, op := range ops {
		rel := strings.ToLower(op.Rel)
		key := s.linkKey(rel, op.Target)
		if op.Add {
			out = append(out, parse.Link{Rel: rel, Target: op.Target})
			continue
		}
		kept := out[:0]
		for _, l := range out {
			if s.linkKey(l.Rel, l.Target) != key {
				kept = append(kept, l)
			}
		}
		out = kept
	}
	seen := map[string]bool{}
	res := []parse.Link{}
	for _, l := range out {
		k := s.linkKey(l.Rel, l.Target)
		if !seen[k] {
			seen[k] = true
			res = append(res, l)
		}
	}
	return res
}
