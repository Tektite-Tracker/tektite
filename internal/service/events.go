package service

import (
	"strings"
	"time"

	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/internal/node"
	"github.com/tektite-tracker/tektite/parse"
)

type SetStatusInput struct {
	Ref      string
	Status   string
	Actor    string
	Override string
	Revision int
}

func (s *Service) SetStatus(in SetStatusInput) (*Appended, error) {
	status := strings.TrimSpace(in.Status)
	if status == "" {
		return nil, usageError("status must not be empty")
	}
	override := strings.TrimSpace(in.Override)
	fields := []node.Field{{Key: "status", Value: status}}
	if override != "" {
		fields = append(fields, node.Field{Key: "override", Value: override})
	}
	if in.Revision > 0 {
		fields = append(fields, node.Field{Key: "revision", Value: in.Revision})
	}
	return s.appendEvent(eventInput{typ: parse.EventSetStatus, query: in.Ref, actorFlag: in.Actor, prepare: func(c context) (step, error) {
		warnings := []parse.Diagnostic{}
		if status == event.StatusDone {
			w, err := s.gateDone(c, override)
			if err != nil {
				return step{}, err
			}
			warnings = w
		}
		if !event.KnownStatus(status) {
			warnings = append(warnings, warning(parse.CodeUnknownStatus, "unknown status %q, expected one of open, active, blocked, review, done, dropped", status))
		}
		return step{fields: fields, warnings: warnings}, nil
	}})
}

type NoteInput struct {
	Ref   string
	Text  string
	Actor string
	At    time.Time
}

func (s *Service) Note(in NoteInput) (*Appended, error) {
	if strings.TrimSpace(in.Text) == "" {
		return nil, usageError("note text must not be empty")
	}
	return s.appendEvent(eventInput{typ: parse.EventNote, query: in.Ref, actorFlag: in.Actor, body: in.Text, at: in.At})
}

type LinkInput struct {
	Verb   string
	Ref    string
	Rel    string
	Target string
	Actor  string
	At     time.Time
}

func (s *Service) Link(in LinkInput) (*Appended, error) {
	rel, target := strings.TrimSpace(in.Rel), strings.TrimSpace(in.Target)
	if rel == "" || target == "" {
		return nil, usageError("rel and target must not be empty")
	}
	return s.appendEvent(eventInput{typ: in.Verb, query: in.Ref, actorFlag: in.Actor, at: in.At, prepare: func(c context) (step, error) {
		warnings := []parse.Diagnostic{}
		resolved := target
		if t, err := c.snap.Resolve(target); err == nil {
			resolved = t.Ref
		} else {
			warnings = append(warnings, warning("", "target %q matches no node", target))
		}
		return step{fields: []node.Field{{Key: "rel", Value: rel}, {Key: "target", Value: resolved}}, warnings: warnings}, nil
	}})
}

type ApproveInput struct {
	Ref      string
	Actor    string
	Revision int
}

func (s *Service) Approve(in ApproveInput) (*Appended, error) {
	return s.appendEvent(eventInput{typ: parse.EventApprove, query: in.Ref, actorFlag: in.Actor, prepare: func(c context) (step, error) {
		if !event.IsHuman(c.actor) {
			return step{}, newError(KindPolicy, "run it as a human, for example with --actor human:name", "approve on %s is blocked: only a human can approve, got %q", c.nf.Ref, c.actor)
		}
		current := c.snap.State(c.nf.Ref).EffectiveRevision()
		if in.Revision > 0 && in.Revision != current {
			return step{}, usageError("%s is at revision %d, so revision %d cannot be approved", c.nf.Ref, current, in.Revision)
		}
		return step{fields: []node.Field{{Key: "revision", Value: current}}}, nil
	}})
}

type ReviseInput struct {
	Ref   string
	Body  string
	Actor string
}

type Revised struct {
	*Appended
	Number int `json:"number"`
}

func (s *Service) Revise(in ReviseInput) (*Revised, error) {
	res, err := s.appendEvent(eventInput{typ: parse.EventRevision, query: in.Ref, actorFlag: in.Actor, body: in.Body, prepare: func(c context) (step, error) {
		return step{fields: []node.Field{{Key: "number", Value: c.snap.State(c.nf.Ref).NextRevision()}}}, nil
	}})
	if err != nil {
		return nil, err
	}
	number, _ := res.Fields[0].Value.(int)
	return &Revised{Appended: res, Number: number}, nil
}

type LeaseInput struct {
	Verb  string
	Ref   string
	Actor string
}

func (s *Service) Lease(in LeaseInput) (*Appended, error) {
	needLive := in.Verb == parse.EventHeartbeat
	return s.appendEvent(eventInput{typ: in.Verb, query: in.Ref, actorFlag: in.Actor, prepare: func(c context) (step, error) {
		l := c.snap.State(c.nf.Ref).Lease
		switch {
		case l == nil:
			return step{}, newError(KindConflict, "claim the node first with `tektite claim`", "%s is not claimed", c.nf.Ref)
		case l.Holder != c.actor:
			return step{}, newError(KindConflict, "only the holder can do this", "%s is claimed by %s until %s", c.nf.Ref, l.Holder, parse.FormatTime(l.Expires))
		case needLive && !l.LiveAt(s.Now()):
			return step{}, newError(KindConflict, "claim the node again with `tektite claim`", "the lease on %s expired at %s", c.nf.Ref, parse.FormatTime(l.Expires))
		}
		return step{}, nil
	}})
}
