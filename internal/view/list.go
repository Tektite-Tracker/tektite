package view

import (
	"slices"
	"strings"
	"time"

	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/internal/node"
	"github.com/tektite-tracker/tektite/internal/policy"
	"github.com/tektite-tracker/tektite/internal/store"
	"github.com/tektite-tracker/tektite/parse"
)

type ListItem struct {
	Ref       string     `json:"ref"`
	ID        string     `json:"id"`
	Kind      string     `json:"kind"`
	Title     string     `json:"title"`
	Parent    string     `json:"parent"`
	Status    string     `json:"status"`
	Priority  any        `json:"priority"`
	Labels    []string   `json:"labels"`
	BlockedBy []string   `json:"blocked_by"`
	Claim     *ClaimItem `json:"claim"`
	Path      string     `json:"path"`
}

type ListFilter struct {
	Kind     string
	Status   string
	UnderRef string
	All      bool
	Needs    string
	Labels   []string
	Priority []string
	Blocked  bool
	Policy   *policy.Policy
}

func List(snap *store.Snapshot, f ListFilter, now time.Time) []ListItem {
	scope := map[string]bool{}
	if f.UnderRef != "" {
		for _, v := range snap.Tree.Descendants(f.UnderRef) {
			scope[v.Ref] = true
		}
	}
	graph := snap.BuildGraph()
	env := policy.NewEnv(snap)
	items := []ListItem{}
	for _, nf := range snap.Nodes {
		st := snap.State(nf.Ref)
		open := openBlockers(graph, snap, nf.Ref)
		switch {
		case f.Blocked && (len(open) == 0 || event.Closed(st.Status)),
			f.Kind != "" && nf.Node.Kind != f.Kind,
			f.Status != "" && st.Status != f.Status,
			f.UnderRef != "" && !scope[nf.Ref],
			!f.All && graph.Superseded(nf.Ref),
			len(f.Labels) > 0 && !hasAnyLabel(nf, f.Labels),
			len(f.Priority) > 0 && !hasAnyPriority(nf, f.Priority),
			f.Needs != "" && !needs(snap, env, f.Policy, nf, st, f.Needs):
			continue
		}
		items = append(items, ListItem{Ref: nf.Ref, ID: nf.Node.ID, Kind: nf.Node.Kind, Title: nf.Node.Title, Parent: nf.ParentOrRaw(), Status: st.Status, Priority: nf.Node.Fields["priority"], Labels: nf.Labels(), BlockedBy: open, Claim: ClaimOf(st, now), Path: nf.Path})
	}
	return items
}

func hasAnyLabel(nf *store.NodeFile, want []string) bool {
	have := nf.Labels()
	for _, w := range want {
		if slices.Contains(have, strings.TrimSpace(w)) {
			return true
		}
	}
	return false
}

func hasAnyPriority(nf *store.NodeFile, want []string) bool {
	have := node.NormalizePriority(parse.StringOf(nf.Node.Fields["priority"]))
	for _, w := range want {
		if node.NormalizePriority(w) == have {
			return true
		}
	}
	return false
}

func openBlockers(g *store.Graph, snap *store.Snapshot, ref string) []string {
	out := []string{}
	for _, b := range g.Blockers[ref] {
		if _, ok := snap.Get(b); ok && !event.Closed(g.States[b].Status) {
			out = append(out, b)
		}
	}
	return out
}
