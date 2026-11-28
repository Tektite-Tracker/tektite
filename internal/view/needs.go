package view

import (
	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/internal/policy"
	"github.com/tektite-tracker/tektite/internal/store"
	"github.com/tektite-tracker/tektite/parse"
)

const (
	NeedsApproval = "approval"
	NeedsOverride = "override"
	NeedsReview   = "review"
)

var NeedsValues = []string{NeedsApproval, NeedsOverride, NeedsReview}

func ValidNeeds(v string) bool {
	for _, n := range NeedsValues {
		if v == n {
			return true
		}
	}
	return false
}

func needs(snap *store.Snapshot, env *policy.Env, p *policy.Policy, nf *store.NodeFile, st event.State, kind string) bool {
	switch kind {
	case NeedsReview:
		return st.Status == event.StatusReview
	case NeedsApproval:
		if nf.Node.Kind != parse.KindPlan || p == nil || event.Closed(st.Status) {
			return false
		}
		for _, g := range p.Gates {
			if g.Rule == policy.RuleApproval {
				ok, _ := st.Approved()
				return !ok
			}
		}
		return false
	case NeedsOverride:
		if st.Status != event.StatusReview {
			return false
		}
		return len(policy.Failed(p.Done(env, nf))) > 0
	}
	return false
}
