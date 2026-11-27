package main

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/internal/service"
)

func parseTTLFlag(v string) (time.Duration, error) {
	d, ok := event.ParseTTL(v)
	if !ok {
		return 0, usage("--ttl must be a positive duration such as 30m, got %q", v)
	}
	return d, nil
}

func (a *app) claimCmd() *cobra.Command {
	var actor, ttl string
	var revision int
	cmd := &cobra.Command{
		Use:   "claim <ref>",
		Short: "Claim a node with an expiring lease",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := parseTTLFlag(ttl)
			if err != nil {
				return err
			}
			if err := checkRevisionFlag(cmd, revision); err != nil {
				return err
			}
			svc, err := a.svc()
			if err != nil {
				return err
			}
			return a.reportEvent(svc.Claim(service.ClaimInput{Ref: args[0], Actor: actor, TTL: d, Revision: revision}))
		},
	}
	addActorFlag(cmd, &actor)
	cmd.Flags().StringVar(&ttl, "ttl", event.DefaultTTL.String(), "lease duration such as 30m")
	addRevisionFlag(cmd, &revision)
	return cmd
}

func (a *app) leaseCmd(typ, short string) *cobra.Command {
	var actor string
	cmd := &cobra.Command{
		Use:   typ + " <ref>",
		Short: short,
		Args:  exactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			svc, err := a.svc()
			if err != nil {
				return err
			}
			return a.reportEvent(svc.Lease(service.LeaseInput{Verb: typ, Ref: args[0], Actor: actor}))
		},
	}
	addActorFlag(cmd, &actor)
	return cmd
}
