package main

import (
	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/policy"
	"github.com/tektite-tracker/tektite/internal/service"
)

func (a *app) approveCmd() *cobra.Command {
	var actor string
	var revision int
	cmd := &cobra.Command{
		Use:   "approve <ref>",
		Short: "Approve the current revision of a plan as a human",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkRevisionFlag(cmd, revision); err != nil {
				return err
			}
			svc, err := a.svc()
			if err != nil {
				return err
			}
			return a.reportEvent(svc.Approve(service.ApproveInput{Ref: args[0], Actor: actor, Revision: revision}))
		},
	}
	addActorFlag(cmd, &actor)
	cmd.Flags().IntVar(&revision, "revision", 0, "revision to approve, which must be the current one")
	return cmd
}

type explained struct {
	Ref   string          `json:"ref"`
	Gates []policy.Result `json:"gates"`
}

func (a *app) policyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "policy",
		Short: "Inspect the policy gates",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usage("unknown policy command %q", args[0])
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "explain <ref>",
		Short: "List the gates that apply to a node and whether each is met",
		Args:  exactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			svc, err := a.svc()
			if err != nil {
				return err
			}
			ex, err := svc.Explain(args[0])
			if err != nil {
				return err
			}
			res := explained{Ref: ex.Ref, Gates: ex.Gates}
			if a.json {
				return a.writeJSON(res)
			}
			a.warn(warningTexts(ex.Warnings))
			if len(res.Gates) == 0 {
				a.printf(a.out, "%s: no gate applies\n", res.Ref)
				return nil
			}
			for _, g := range res.Gates {
				state := "met"
				if !g.Met {
					state = "not met"
				}
				a.printf(a.out, "%s  %s  %s\n", g.Name, state, g.Reason)
				for _, it := range g.Items {
					a.printf(a.out, "  - %s\n", it)
				}
			}
			return nil
		},
	})
	return cmd
}
