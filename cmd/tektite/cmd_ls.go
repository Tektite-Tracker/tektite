package main

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/service"
	"github.com/tektite-tracker/tektite/parse"
)

func (a *app) lsCmd() *cobra.Command {
	var kind, status, under, needs string
	var labels, priorities []string
	var all, blocked bool
	cmd := &cobra.Command{
		Use:   "ls",
		Short: "List nodes",
		Args:  exactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			sv, err := a.svc()
			if err != nil {
				return err
			}
			items, err := sv.List(service.ListInput{Kind: kind, Status: status, Under: under, All: all, Needs: needs, Labels: labels, Priority: priorities, Blocked: blocked})
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
				prio := "-"
				if p := parse.StringOf(it.Priority); it.Priority != nil && p != "" {
					prio = p
				}
				title := it.Title
				if len(it.BlockedBy) > 0 {
					title += "  (blocked by " + strings.Join(it.BlockedBy, ", ") + ")"
				}
				a.printf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", it.Ref, it.Kind, it.Status, prio, holder, title)
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
	cmd.Flags().StringArrayVar(&labels, "label", nil, "only nodes with this label, repeatable, any of them matches")
	cmd.Flags().StringArrayVar(&priorities, "priority", nil, "only nodes with this priority: critical, high, medium or low, repeatable")
	cmd.Flags().BoolVar(&blocked, "blocked", false, "only nodes held by a blocker that is not done or dropped")
	cmd.Flags().BoolVar(&all, "all", false, "include superseded nodes")
	return cmd
}
