package view

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/internal/store"
)

type NextItem struct {
	Ref      string `json:"ref"`
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Title    string `json:"title"`
	Parent   string `json:"parent"`
	Status   string `json:"status"`
	Priority any    `json:"priority"`
	Path     string `json:"path"`
}

func NewNextItem(nf *store.NodeFile) NextItem {
	return NextItem{
		Ref: nf.Ref, ID: nf.Node.ID, Kind: nf.Node.Kind, Title: nf.Node.Title, Parent: nf.ParentOrRaw(),
		Status: event.StatusOpen, Priority: nf.Node.Fields["priority"], Path: nf.Path,
	}
}

var priorityNames = map[string]float64{"critical": 4, "high": 3, "medium": 2, "normal": 2, "low": 1}

func Priority(v any) float64 {
	switch p := v.(type) {
	case int:
		return float64(p)
	case int64:
		return float64(p)
	case float64:
		return p
	case string:
		s := strings.ToLower(strings.TrimSpace(p))
		if n, ok := priorityNames[s]; ok {
			return n
		}
		if n, err := strconv.ParseFloat(s, 64); err == nil {
			return n
		}
	}
	return 0
}

func Eligible(snap *store.Snapshot, kind string, now time.Time) []*store.NodeFile {
	states := map[string]event.State{}
	for _, nf := range snap.Nodes {
		states[nf.Ref] = snap.State(nf.Ref)
	}
	blocked := snap.BuildGraph().Blockers
	out := []*store.NodeFile{}
	for _, nf := range snap.Nodes {
		st := states[nf.Ref]
		if (kind != "" && nf.Node.Kind != kind) || st.Status != event.StatusOpen || st.Lease.LiveAt(now) {
			continue
		}
		held := false
		for _, b := range blocked[nf.Ref] {
			if bs, ok := states[b]; ok && !event.Closed(bs.Status) {
				held = true
			}
		}
		if !held {
			out = append(out, nf)
		}
	}
	slices.SortStableFunc(out, func(a, b *store.NodeFile) int {
		return cmp.Or(
			cmp.Compare(Priority(b.Node.Fields["priority"]), Priority(a.Node.Fields["priority"])),
			cmp.Compare(a.Node.ID, b.Node.ID),
			cmp.Compare(a.Ref, b.Ref),
		)
	})
	return out
}
