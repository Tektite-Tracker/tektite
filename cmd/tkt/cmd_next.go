package main

import (
	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/internal/policy"
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
			_, snap, err := a.load()
			if err != nil {
				return err
			}
			list := view.Eligible(snap, kind, a.now())
			p, warnings, err := a.loadPolicy()
			if err != nil {
				return err
			}
			if p != nil {
				who, _ := a.resolveActor(actor)
				env := policy.NewEnv(snap)
				open := list[:0]
				for _, nf := range list {
					if len(policy.Failed(p.Claim(env, nf, who, a.now()))) == 0 {
						open = append(open, nf)
					}
				}
				list = open
			}
			if !a.json {
				a.warn(warnings)
			}
			if len(list) == 0 {
				return fail(exitNotFound, "create a ticket with `tkt new ticket` or release a claim", "nothing is available")
			}
			if len(list) > limit {
				list = list[:limit]
			}
			if claim {
				if _, err := a.claimNode(actor, list[0].Ref, d, 0); err != nil {
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
