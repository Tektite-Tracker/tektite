package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/internal/node"
	"github.com/tektite-tracker/tektite/internal/store"
	"github.com/tektite-tracker/tektite/parse"
)

const timeLayout = "2006-01-02T15:04:05Z"

type claimItem struct {
	Holder    string `json:"holder"`
	ExpiresAt string `json:"expires_at"`
	Live      bool   `json:"live"`
}

func claimOf(st event.State, now time.Time) *claimItem {
	if st.Lease == nil {
		return nil
	}
	return &claimItem{Holder: st.Lease.Holder, ExpiresAt: st.Lease.Expires.UTC().Format(timeLayout), Live: st.Lease.LiveAt(now)}
}

func (a *app) held(snap *store.Snapshot, ref string) event.State {
	return event.Fold(snap.EventsOf(ref), event.Options{})
}

func (a *app) claimNode(actorFlag, query string, ttl time.Duration, revision int) (*appended, error) {
	d := &event.Draft{Type: parse.EventClaim, Fields: []node.Field{{Key: "ttl", Value: ttl.String()}}}
	if revision > 0 {
		d.Fields = append(d.Fields, node.Field{Key: "revision", Value: revision})
	}
	check := func(snap *store.Snapshot, nf *store.NodeFile, actor string) ([]string, error) {
		warnings, err := a.gateClaim(snap, nf, actor)
		if err != nil {
			return nil, err
		}
		l := a.held(snap, nf.Ref).Lease
		if l.LiveAt(a.now()) && l.Holder != actor {
			return nil, fail(exitConflict, "wait for the lease to expire or pick another node with `tkt next`",
				"%s is claimed by %s until %s", nf.Ref, l.Holder, l.Expires.UTC().Format(timeLayout))
		}
		return warnings, nil
	}
	res, err := a.writeEvent(actorFlag, query, d, check)
	if err != nil {
		return nil, err
	}
	r, err := a.repo()
	if err != nil {
		return nil, err
	}
	snap, err := r.Load()
	if err != nil {
		return nil, fmt.Errorf("load repository: %w", err)
	}
	if l := a.held(snap, res.Ref).Lease; l == nil || l.Holder != res.Actor {
		holder := "another actor"
		if l != nil {
			holder = l.Holder
		}
		return nil, fail(exitConflict, "another claim won the race, pick another node with `tkt next`", "%s was claimed by %s first", res.Ref, holder)
	}
	return res, nil
}

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
			res, err := a.claimNode(actor, args[0], d, revision)
			if err != nil {
				return err
			}
			return a.report(res)
		},
	}
	cmd.Flags().StringVar(&actor, "actor", "", "actor recorded on the event")
	cmd.Flags().StringVar(&ttl, "ttl", event.DefaultTTL.String(), "lease duration such as 30m")
	cmd.Flags().IntVar(&revision, "revision", 0, "plan revision the work is built against")
	return cmd
}

func (a *app) leaseCmd(typ, short string, needLive bool) *cobra.Command {
	var actor string
	cmd := &cobra.Command{
		Use:   typ + " <ref>",
		Short: short,
		Args:  exactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			check := func(snap *store.Snapshot, nf *store.NodeFile, who string) ([]string, error) {
				l := a.held(snap, nf.Ref).Lease
				switch {
				case l == nil:
					return nil, fail(exitConflict, "claim the node first with `tkt claim`", "%s is not claimed", nf.Ref)
				case l.Holder != who:
					return nil, fail(exitConflict, "only the holder can do this", "%s is claimed by %s until %s", nf.Ref, l.Holder, l.Expires.UTC().Format(timeLayout))
				case needLive && !l.LiveAt(a.now()):
					return nil, fail(exitConflict, "claim the node again with `tkt claim`", "the lease on %s expired at %s", nf.Ref, l.Expires.UTC().Format(timeLayout))
				}
				return nil, nil
			}
			return a.appendEvent(actor, args[0], &event.Draft{Type: typ}, check)
		},
	}
	cmd.Flags().StringVar(&actor, "actor", "", "actor recorded on the event")
	return cmd
}
