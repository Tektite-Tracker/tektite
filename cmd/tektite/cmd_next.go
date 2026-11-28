package main

import (
	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/internal/service"
	"github.com/tektite-tracker/tektite/internal/view"
	"github.com/tektite-tracker/tektite/parse"
)

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
			svc, err := a.svc()
			if err != nil {
				return err
			}
			list, warnings, err := svc.Next(kind, actor)
			if err != nil {
				return err
			}
			if !a.json {
				a.warn(warningTexts(warnings))
			}
			if len(list) == 0 {
				return fail(exitNotFound, "create a ticket with `tektite new ticket` or release a claim", "nothing is available")
			}
			if len(list) > limit {
				list = list[:limit]
			}
			if claim {
				if _, err := svc.Claim(service.ClaimInput{Ref: list[0].Ref, Actor: actor, TTL: d}); err != nil {
					return err
				}
			}
			items := []view.NextItem{}
			for _, nf := range list {
				items = append(items, view.NewNextItem(nf))
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
	cmd.Flags().StringVar(&kind, "kind", parse.KindTicket, "only nodes of this kind, empty for any")
	cmd.Flags().IntVar(&limit, "limit", 1, "how many nodes to return")
	cmd.Flags().BoolVar(&claim, "claim", false, "claim the first node in the same command")
	addActorFlag(cmd, &actor)
	cmd.Flags().StringVar(&ttl, "ttl", event.DefaultTTL.String(), "lease duration with --claim")
	return cmd
}
