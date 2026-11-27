package check

import (
	"errors"
	"fmt"
	"strings"

	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/internal/policy"
	"github.com/tektite-tracker/tektite/parse"
)

const (
	CodePolicyMalformed = "TKT-E104"
	CodePolicyViolation = "TKT-E105"
)

func (c *run) policy() {
	if c.repo == nil {
		return
	}
	p, err := policy.Load(c.repo)
	var bad *policy.MalformedError
	if errors.As(err, &bad) {
		c.add(parse.SeverityError, CodePolicyMalformed, bad.Path, bad.Line, bad.Msg+", so no gate is enforced")
		return
	}
	if err != nil || p == nil {
		return
	}
	env := &policy.Env{Snap: c.snap, Graph: c.graph}
	for _, nf := range c.snap.Nodes {
		paths := map[*parse.Event]string{}
		for _, f := range c.snap.Events[nf.Ref] {
			paths[f.Event] = f.Path
		}
		for _, h := range c.graph.States[nf.Ref].History {
			if !h.Applied {
				continue
			}
			var results []policy.Result
			switch h.Event.Type {
			case "set-status":
				if strings.TrimSpace(fmt.Sprint(h.Event.Fields["status"])) != event.StatusDone || (h.Override != "" && event.IsHuman(h.Actor)) {
					continue
				}
				results = p.Done(env, nf)
			case "claim":
				results = p.Claim(env, nf, h.Actor, h.Event.At)
			default:
				continue
			}
			for _, r := range policy.Failed(results) {
				msg := fmt.Sprintf("%s: event %s by %s breaks gate %q: %s", nf.Ref, h.Event.ID, h.Actor, r.Name, r.Reason)
				if len(r.Items) > 0 {
					msg += " (" + strings.Join(r.Items, "; ") + ")"
				}
				c.add(parse.SeverityError, CodePolicyViolation, paths[h.Event], 0, msg)
			}
		}
	}
}
