package service

import (
	"time"

	"github.com/tektite-tracker/tektite/internal/node"
	"github.com/tektite-tracker/tektite/parse"
)

type ClaimInput struct {
	Ref      string
	Actor    string
	TTL      time.Duration
	Revision int
}

func (s *Service) Claim(in ClaimInput) (*Appended, error) {
	fields := []node.Field{{Key: "ttl", Value: in.TTL.String()}}
	if in.Revision > 0 {
		fields = append(fields, node.Field{Key: "revision", Value: in.Revision})
	}
	res, err := s.appendEvent(eventInput{typ: parse.EventClaim, query: in.Ref, actorFlag: in.Actor, prepare: func(c context) (step, error) {
		warnings, err := s.gateClaim(c, s.Now())
		if err != nil {
			return step{}, err
		}
		l := c.snap.State(c.nf.Ref).Lease
		if l.LiveAt(s.Now()) && l.Holder != c.actor {
			return step{}, newError(KindConflict, "wait for the lease to expire or pick another node with `tkt next`",
				"%s is claimed by %s until %s", c.nf.Ref, l.Holder, parse.FormatTime(l.Expires))
		}
		return step{fields: fields, warnings: warnings}, nil
	}})
	if err != nil {
		return nil, err
	}
	snap, err := s.load()
	if err != nil {
		return nil, err
	}
	if l := snap.State(res.Ref).Lease; l == nil || l.Holder != res.Actor {
		holder := "another actor"
		if l != nil {
			holder = l.Holder
		}
		return nil, newError(KindConflict, "another claim won the race, pick another node with `tkt next`", "%s was claimed by %s first", res.Ref, holder)
	}
	return res, nil
}
