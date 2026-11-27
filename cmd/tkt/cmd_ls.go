package main

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/event"
)

type lsItem struct {
	Ref    string     `json:"ref"`
	ID     string     `json:"id"`
	Kind   string     `json:"kind"`
	Title  string     `json:"title"`
	Parent string     `json:"parent"`
	Status string     `json:"status"`
	Claim  *claimItem `json:"claim"`
	Path   string     `json:"path"`
}

func (a *app) lsCmd() *cobra.Command {
	var kind, status, under string
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List nodes",
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
			scope := map[string]bool{}
			if under != "" {
				root, err := snap.Resolve(under)
				if err != nil {
					return fmt.Errorf("--under: %w", err)
				}
				for _, v := range snap.Tree.Descendants(root.Ref) {
					scope[v.Ref] = true
				}
			}
			items := []lsItem{}
			for _, nf := range snap.Nodes {
				st := event.Fold(snap.EventsOf(nf.Ref), event.Options{})
				switch {
				case kind != "" && nf.Node.Kind != kind,
					status != "" && st.Status != status,
					under != "" && !scope[nf.Ref]:
					continue
				}
				parent := nf.ParentRef
				if parent == "" {
					parent = nf.Node.Parent
				}
				items = append(items, lsItem{Ref: nf.Ref, ID: nf.Node.ID, Kind: nf.Node.Kind, Title: nf.Node.Title, Parent: parent, Status: st.Status, Claim: claimOf(st, a.now()), Path: nf.Path})
			}
			if a.json {
				return a.writeJSON(items)
			}
			w := tabwriter.NewWriter(a.out, 0, 4, 2, ' ', 0)
			for _, it := range items {
				holder := "-"
				if it.Claim != nil && it.Claim.Live {
					holder = it.Claim.Holder
				}
				a.printf(w, "%s\t%s\t%s\t%s\t%s\n", it.Ref, it.Kind, it.Status, holder, it.Title)
			}
			if err := w.Flush(); err != nil {
				return fmt.Errorf("write list: %w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&kind, "kind", "", "only nodes of this kind")
	cmd.Flags().StringVar(&status, "status", "", "only nodes with this status")
	cmd.Flags().StringVar(&under, "under", "", "only descendants of this node")
	return cmd
}
