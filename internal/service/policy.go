package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tektite-tracker/tektite/internal/check"
	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/internal/policy"
	"github.com/tektite-tracker/tektite/parse"
)

type GateResult = policy.Result

func (s *Service) loadPolicy() (*policy.Policy, []parse.Diagnostic, error) {
	p, err := policy.Load(s.Repo)
	var bad *policy.MalformedError
	if errors.As(err, &bad) {
		return nil, []parse.Diagnostic{warning(check.CodePolicyMalformed, "%s: %s, so no gate is enforced, run `tkt check` for details", bad.Path, bad.Msg)}, nil
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
		if h := policy.Hint(f.Rule); h != "" {
			hints = append(hints, h)
		}
	}
	if agentOverride {
		b.WriteString("\n  an agent cannot override a gate")
	} else if verb == parse.EventSetStatus {
		hints = append(hints, "a human can pass --override with a reason")
	}
	return &Error{Kind: KindPolicy, Msg: b.String(), Hint: strings.Join(dedupe(hints), "; "), Gates: failed}
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

func (s *Service) gateClaim(c context, now time.Time) ([]parse.Diagnostic, error) {
	p, warnings, err := s.loadPolicy()
	if err != nil {
		return nil, err
	}
	if failed := policy.Failed(p.Claim(policy.NewEnv(c.snap), c.nf, c.actor, now)); len(failed) > 0 {
		return nil, gateBlocked("claim", c.nf.Ref, failed, false)
	}
	return warnings, nil
}

func (s *Service) gateDone(c context, override string) ([]parse.Diagnostic, error) {
	p, warnings, err := s.loadPolicy()
	if err != nil {
		return nil, err
	}
	failed := policy.Failed(p.Done(policy.NewEnv(c.snap), c.nf))
	if len(failed) == 0 {
		return warnings, nil
	}
	if override != "" && event.IsHuman(c.actor) {
		return warnings, nil
	}
	return nil, gateBlocked(parse.EventSetStatus, c.nf.Ref, failed, override != "")
}

type Explained struct {
	Ref      string
	Gates    []policy.Result
	Warnings []parse.Diagnostic
}

func (s *Service) Explain(ref string) (*Explained, error) {
	p, warnings, err := s.loadPolicy()
	if err != nil {
		return nil, err
	}
	snap, err := s.load()
	if err != nil {
		return nil, err
	}
	nf, err := snap.Resolve(ref)
	if err != nil {
		return nil, fmt.Errorf("policy explain: %w", err)
	}
	return &Explained{Ref: nf.Ref, Gates: p.Explain(policy.NewEnv(snap), nf), Warnings: warnings}, nil
}
