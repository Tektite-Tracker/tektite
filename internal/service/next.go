package service

import (
	"github.com/tektite-tracker/tektite/internal/policy"
	"github.com/tektite-tracker/tektite/internal/store"
	"github.com/tektite-tracker/tektite/internal/view"
	"github.com/tektite-tracker/tektite/parse"
)

func (s *Service) Next(kind, actorFlag string) ([]*store.NodeFile, []parse.Diagnostic, error) {
	snap, err := s.load()
	if err != nil {
		return nil, nil, err
	}
	list := view.Eligible(snap, kind, s.Now())
	p, warnings, err := s.loadPolicy()
	if err != nil {
		return nil, nil, err
	}
	if p == nil {
		return list, warnings, nil
	}
	who, _ := s.ResolveActor(actorFlag)
	env := policy.NewEnv(snap)
	open := list[:0]
	for _, nf := range list {
		if len(policy.Failed(p.Claim(env, nf, who, s.Now()))) == 0 {
			open = append(open, nf)
		}
	}
	return open, warnings, nil
}
