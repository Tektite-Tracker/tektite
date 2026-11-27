package policy

import (
	"fmt"
	"strings"
	"time"

	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/internal/store"
	"github.com/tektite-tracker/tektite/parse"
)

type Result struct {
	Name   string   `json:"name"`
	Rule   string   `json:"rule"`
	Met    bool     `json:"met"`
	Reason string   `json:"reason"`
	Items  []string `json:"items,omitempty"`
}

type Env struct {
	Snap  *store.Snapshot
	Graph *store.Graph
}

func NewEnv(snap *store.Snapshot) *Env {
	return &Env{Snap: snap, Graph: snap.BuildGraph()}
}

func (e *Env) plan(nf *store.NodeFile) (*store.NodeFile, bool) {
	ancestors, _ := e.Snap.Tree.Ancestors(nf.Ref)
	for _, ref := range ancestors {
		if a, ok := e.Snap.Get(ref); ok && a.Node.Kind == parse.KindPlan {
			return a, true
		}
	}
	return nil, false
}

func (e *Env) approval(nf *store.NodeFile, g Gate, at time.Time) (Result, bool) {
	plan, ok := e.plan(nf)
	if !ok {
		return Result{}, false
	}
	events := e.Snap.EventsOf(plan.Ref)
	if !at.IsZero() {
		events = before(events, at)
	}
	met, why := event.Fold(events, event.Options{}).Approved()
	res := Result{Name: g.Name, Rule: g.Rule, Met: met}
	if met {
		res.Reason = fmt.Sprintf("plan %s is %s", plan.Ref, why)
	} else {
		res.Reason = fmt.Sprintf("plan %s is not approved: %s", plan.Ref, why)
	}
	return res, true
}

func before(events []*parse.Event, at time.Time) []*parse.Event {
	out := make([]*parse.Event, 0, len(events))
	for _, ev := range events {
		if ev != nil && !ev.At.After(at) {
			out = append(out, ev)
		}
	}
	return out
}

func (e *Env) evidence(nf *store.NodeFile, g Gate) Result {
	res := Result{Name: g.Name, Rule: g.Rule}
	all := e.Snap.EvidenceFor(e.Graph, nf.Ref)
	failing := 0
	for _, ev := range all {
		if e.Graph.Superseded(ev.Node.Ref) {
			continue
		}
		if ev.ExitCode == nil || *ev.ExitCode == 0 {
			res.Met = true
			res.Reason = fmt.Sprintf("%s has passing evidence %s", nf.Ref, ev.Node.Ref)
			return res
		}
		failing++
	}
	switch failing {
	case 0:
		res.Reason = fmt.Sprintf("%s has no evidence", nf.Ref)
	default:
		res.Reason = fmt.Sprintf("%s has no passing evidence, %d attached with a failing exit code", nf.Ref, failing)
	}
	return res
}

func (e *Env) OpenGaps(nf *store.NodeFile) []string {
	closed := e.Snap.ClosedBlocks(nf.Ref)
	out := []string{}
	for _, b := range nf.Node.Blocks {
		if !b.Gap || b.Checked || (b.ID != "" && hasKey(closed, strings.ToLower(b.ID))) {
			continue
		}
		item := b.Text
		if b.ID != "" {
			item = b.ID + ": " + b.Text
		}
		out = append(out, item)
	}
	for _, kid := range e.Snap.Tree.Children[nf.Ref] {
		child, ok := e.Snap.Get(kid)
		if !ok || child.Node.Kind != parse.KindGap {
			continue
		}
		if s := e.Graph.States[kid].Status; !event.Closed(s) {
			out = append(out, kid+": "+child.Node.Title)
		}
	}
	return out
}

func (e *Env) gaps(nf *store.NodeFile, g Gate) Result {
	open := e.OpenGaps(nf)
	res := Result{Name: g.Name, Rule: g.Rule, Met: len(open) == 0, Items: open}
	if res.Met {
		res.Reason = fmt.Sprintf("%s has no open gaps", nf.Ref)
	} else {
		res.Reason = fmt.Sprintf("%s has %d open gap(s)", nf.Ref, len(open))
	}
	return res
}

func (p *Policy) Claim(e *Env, nf *store.NodeFile, actor string, at time.Time) []Result {
	out := []Result{}
	if p == nil || event.IsHuman(actor) {
		return out
	}
	for _, g := range p.Gates {
		if g.Rule != RuleApproval || !g.Applies(nf) {
			continue
		}
		if res, ok := e.approval(nf, g, at); ok {
			out = append(out, res)
		}
	}
	return out
}

func (p *Policy) Done(e *Env, nf *store.NodeFile) []Result {
	out := []Result{}
	if p == nil {
		return out
	}
	for _, g := range p.Gates {
		if !g.Applies(nf) {
			continue
		}
		switch g.Rule {
		case RuleEvidence:
			out = append(out, e.evidence(nf, g))
		case RuleNoGaps:
			out = append(out, e.gaps(nf, g))
		}
	}
	return out
}

func (p *Policy) Explain(e *Env, nf *store.NodeFile) []Result {
	out := []Result{}
	if p == nil {
		return out
	}
	for _, g := range p.Gates {
		if !g.Applies(nf) {
			continue
		}
		switch g.Rule {
		case RuleApproval:
			if res, ok := e.approval(nf, g, time.Time{}); ok {
				out = append(out, res)
			}
		case RuleEvidence:
			out = append(out, e.evidence(nf, g))
		case RuleNoGaps:
			out = append(out, e.gaps(nf, g))
		}
	}
	return out
}

func Failed(rs []Result) []Result {
	out := []Result{}
	for _, r := range rs {
		if !r.Met {
			out = append(out, r)
		}
	}
	return out
}

func hasKey(m map[string]string, key string) bool {
	_, ok := m[key]
	return ok
}
