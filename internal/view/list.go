package view

import (
	"time"

	"github.com/tektite-tracker/tektite/internal/policy"
	"github.com/tektite-tracker/tektite/internal/store"
)

type ListItem struct {
	Ref    string     `json:"ref"`
	ID     string     `json:"id"`
	Kind   string     `json:"kind"`
	Title  string     `json:"title"`
	Parent string     `json:"parent"`
	Status string     `json:"status"`
	Claim  *ClaimItem `json:"claim"`
	Path   string     `json:"path"`
}

type ListFilter struct {
	Kind     string
	Status   string
	UnderRef string
	All      bool
	Needs    string
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
		switch {
		case f.Kind != "" && nf.Node.Kind != f.Kind,
			f.Status != "" && st.Status != f.Status,
			f.UnderRef != "" && !scope[nf.Ref],
			!f.All && graph.Superseded(nf.Ref),
			f.Needs != "" && !needs(snap, env, f.Policy, nf, st, f.Needs):
			continue
		}
		items = append(items, ListItem{Ref: nf.Ref, ID: nf.Node.ID, Kind: nf.Node.Kind, Title: nf.Node.Title, Parent: nf.ParentOrRaw(), Status: st.Status, Claim: ClaimOf(st, now), Path: nf.Path})
	}
	return items
}
