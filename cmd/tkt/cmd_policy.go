package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/check"
	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/internal/node"
	"github.com/tektite-tracker/tektite/internal/policy"
	"github.com/tektite-tracker/tektite/internal/store"
	"github.com/tektite-tracker/tektite/parse"
)

func (a *app) loadPolicy() (*policy.Policy, []string, error) {
	r, err := a.repo()
	if err != nil {
		return nil, nil, err
	}
	p, err := policy.Load(r)
	var bad *policy.MalformedError
	if errors.As(err, &bad) {
		return nil, []string{fmt.Sprintf("%s: %s: %s, so no gate is enforced, run `tkt check` for details", check.CodePolicyMalformed, bad.Path, bad.Msg)}, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("load policy: %w", err)
	}
	return p, nil, nil
}

func gateBlocked(verb, ref string, failed []policy.Result, agentOverride bool) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%s on %s is blocked by policy", verb, ref)
	hints := []string{}
	for _, f := range failed {
		fmt.Fprintf(&b, "\n  gate %q: %s", f.Name, f.Reason)
		for _, it := range f.Items {
			fmt.Fprintf(&b, "\n    - %s", it)
		}
		switch f.Rule {
		case policy.RuleApproval:
			hints = append(hints, "ask a human to run `tkt approve` on the plan")
		case policy.RuleEvidence:
			hints = append(hints, "attach passing evidence with `tkt attach-evidence`")
		case policy.RuleNoGaps:
			hints = append(hints, "close the open gaps")
		}
	}
	if agentOverride {
		b.WriteString("\n  an agent cannot override a gate")
	} else if verb == "set-status" {
		hints = append(hints, "a human can pass --override with a reason")
	}
	return &cliError{code: exitPolicy, msg: b.String(), hint: strings.Join(dedupe(hints), "; "), gates: failed}
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func (a *app) gateClaim(snap *store.Snapshot, nf *store.NodeFile, actor string) ([]string, error) {
	p, warnings, err := a.loadPolicy()
	if err != nil {
		return nil, err
	}
	if failed := policy.Failed(p.Claim(policy.NewEnv(snap), nf, actor, a.now())); len(failed) > 0 {
		return nil, gateBlocked("claim", nf.Ref, failed, false)
	}
	return warnings, nil
}

func (a *app) gateDone(snap *store.Snapshot, nf *store.NodeFile, actor, override string) ([]string, error) {
	p, warnings, err := a.loadPolicy()
	if err != nil {
		return nil, err
	}
	failed := policy.Failed(p.Done(policy.NewEnv(snap), nf))
	if len(failed) == 0 {
		return warnings, nil
	}
	if override != "" && event.IsHuman(actor) {
		return warnings, nil
	}
	return nil, gateBlocked("set-status", nf.Ref, failed, override != "")
}

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
			d := &event.Draft{Type: parse.EventApprove}
			check := func(snap *store.Snapshot, nf *store.NodeFile, who string) ([]string, error) {
				if !event.IsHuman(who) {
					return nil, &cliError{code: exitPolicy, hint: "run it as a human, for example with --actor human:name", msg: fmt.Sprintf("approve on %s is blocked: only a human can approve, got %q", nf.Ref, who)}
				}
				current := a.held(snap, nf.Ref).EffectiveRevision()
				if revision > 0 && revision != current {
					return nil, usage("%s is at revision %d, so revision %d cannot be approved", nf.Ref, current, revision)
				}
				d.Fields = []node.Field{{Key: "revision", Value: current}}
				return nil, nil
			}
			return a.appendEvent(actor, args[0], d, check)
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
			p, warnings, err := a.loadPolicy()
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
			nf, err := snap.Resolve(args[0])
			if err != nil {
				return fmt.Errorf("policy explain: %w", err)
			}
			res := explained{Ref: nf.Ref, Gates: p.Explain(policy.NewEnv(snap), nf)}
			if a.json {
				return a.writeJSON(res)
			}
			a.warn(warnings)
			if len(res.Gates) == 0 {
				a.printf(a.out, "%s: no gate applies\n", nf.Ref)
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
