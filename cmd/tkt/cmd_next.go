package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/internal/store"
)

type nextItem struct {
	Ref      string `json:"ref"`
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Title    string `json:"title"`
	Parent   string `json:"parent"`
	Status   string `json:"status"`
	Priority any    `json:"priority"`
	Path     string `json:"path"`
}

var priorityNames = map[string]float64{"critical": 4, "high": 3, "medium": 2, "normal": 2, "low": 1}

func priorityOf(v any) float64 {
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

func (a *app) eligible(snap *store.Snapshot, kind string) []*store.NodeFile {
	states := map[string]event.State{}
	for _, nf := range snap.Nodes {
		states[nf.Ref] = a.held(snap, nf.Ref)
	}
	blocked := snap.BuildGraph().Blockers
	out := []*store.NodeFile{}
	for _, nf := range snap.Nodes {
		st := states[nf.Ref]
		if (kind != "" && nf.Node.Kind != kind) || st.Status != event.StatusOpen || st.Lease.LiveAt(a.now()) {
			continue
		}
		held := false
		for _, b := range blocked[nf.Ref] {
			if bs, ok := states[b]; ok && bs.Status != "done" && bs.Status != "dropped" {
				held = true
			}
		}
		if !held {
			out = append(out, nf)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		pi, pj := priorityOf(out[i].Node.Fields["priority"]), priorityOf(out[j].Node.Fields["priority"])
		if pi != pj {
			return pi > pj
		}
		if out[i].Node.ID != out[j].Node.ID {
			return out[i].Node.ID < out[j].Node.ID
		}
		return out[i].Ref < out[j].Ref
	})
	return out
}

func (a *app) nextCmd() *cobra.Command {
	var kind, actor, ttl string
	var limit int
	var claim bool
	cmd := &cobra.Command{
		Use:   "next",
		Short: "Show the best node to work on next",
		Args:  exactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			if limit < 1 {
				return usage("--limit must be at least 1")
			}
			d, err := parseTTLFlag(ttl)
			if err != nil {
				return err
			}
			r, err := a.repo()
			if err != nil {
				return err
			}
			snap, err := r.Load()
			if err != nil {
				return fmt.Errorf("load repository: %w", err)
			}
			list := a.eligible(snap, kind)
			if len(list) == 0 {
				return fail(exitNotFound, "create a ticket with `tkt new ticket` or release a claim", "nothing is available")
			}
			if len(list) > limit {
				list = list[:limit]
			}
			if claim {
				if _, err := a.claimNode(actor, list[0].Ref, d); err != nil {
					return err
				}
			}
			items := []nextItem{}
			for _, nf := range list {
				parent := nf.ParentRef
				if parent == "" {
					parent = nf.Node.Parent
				}
				items = append(items, nextItem{
					Ref: nf.Ref, ID: nf.Node.ID, Kind: nf.Node.Kind, Title: nf.Node.Title, Parent: parent,
					Status: event.StatusOpen, Priority: nf.Node.Fields["priority"], Path: nf.Path,
				})
			}
			if a.json {
				return a.writeJSON(items)
			}
			for _, it := range items {
				a.printf(a.out, "%s  %s  %s\n", it.Ref, it.Kind, it.Title)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "ticket", "only nodes of this kind, empty for any")
	cmd.Flags().IntVar(&limit, "limit", 1, "how many nodes to return")
	cmd.Flags().BoolVar(&claim, "claim", false, "claim the first node in the same command")
	cmd.Flags().StringVar(&actor, "actor", "", "actor recorded on the claim")
	cmd.Flags().StringVar(&ttl, "ttl", event.DefaultTTL.String(), "lease duration with --claim")
	return cmd
}
