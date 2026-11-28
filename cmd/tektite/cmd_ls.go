package main

import (
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/service"
)

func (a *app) lsCmd() *cobra.Command {
	var kind, status, under, needs string
	var all bool
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List nodes",
		Args:  exactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			sv, err := a.svc()
			if err != nil {
				return err
			}
			items, err := sv.List(service.ListInput{Kind: kind, Status: status, Under: under, All: all, Needs: needs})
			if err != nil {
				return err
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
	cmd.Flags().StringVar(&needs, "needs", "", "only nodes waiting on a human: approval, override or review")
	cmd.Flags().BoolVar(&all, "all", false, "include superseded nodes")
	return cmd
}
