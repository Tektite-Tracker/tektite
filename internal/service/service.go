package service

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/internal/node"
	"github.com/tektite-tracker/tektite/internal/store"
	"github.com/tektite-tracker/tektite/parse"
)

type Kind int

const (
	KindInternal Kind = iota
	KindUsage
	KindNotFound
	KindConflict
	KindPolicy
)

const usageHint = "run with --help to see the usage"

type Error struct {
	Kind  Kind
	Msg   string
	Hint  string
	Gates []GateResult
}

func (e *Error) Error() string {
	return e.Msg
}

func newError(kind Kind, hint, format string, args ...any) *Error {
	return &Error{Kind: kind, Msg: fmt.Sprintf(format, args...), Hint: hint}
}

func usageError(format string, args ...any) *Error {
	return newError(KindUsage, usageHint, format, args...)
}

type Git interface {
	User() string
	Head() string
	Dirty() bool
	CommitsSince(since string, paths []string) ([]string, error)
}

type NoGit struct{}

func (NoGit) User() string { return "" }
func (NoGit) Head() string { return "" }
func (NoGit) Dirty() bool  { return false }
func (NoGit) CommitsSince(string, []string) ([]string, error) {
	return nil, nil
}

type Service struct {
	Repo   *store.Repo
	Now    func() time.Time
	Rand   io.Reader
	Getenv func(string) string
	Git    Git
	Exec   Executor
}

type Appended struct {
	Ref      string             `json:"ref"`
	Event    string             `json:"event"`
	Type     string             `json:"type"`
	Actor    string             `json:"actor"`
	At       string             `json:"at"`
	Path     string             `json:"path"`
	Fields   []node.Field       `json:"-"`
	Warnings []parse.Diagnostic `json:"warnings"`
}

type context struct {
	snap  *store.Snapshot
	nf    *store.NodeFile
	actor string
}

type step struct {
	fields   []node.Field
	warnings []parse.Diagnostic
}

type prepareFunc func(c context) (step, error)

type eventInput struct {
	typ       string
	query     string
	actorFlag string
	actor     string
	body      string
	prepare   prepareFunc
}

func warning(code, format string, args ...any) parse.Diagnostic {
	return parse.Diagnostic{Code: code, Severity: parse.SeverityWarning, Message: fmt.Sprintf(format, args...)}
}

func (s *Service) git() Git {
	if s.Git == nil {
		return NoGit{}
	}
	return s.Git
}

func (s *Service) load() (*store.Snapshot, error) {
	snap, err := s.Repo.Load()
	if err != nil {
		return nil, fmt.Errorf("load repository: %w", err)
	}
	return snap, nil
}

func (s *Service) ResolveActor(flag string) (string, []parse.Diagnostic) {
	if v := strings.TrimSpace(flag); v != "" {
		return v, nil
	}
	if v := strings.TrimSpace(s.Getenv("TKT_ACTOR")); v != "" {
		return v, nil
	}
	if v := strings.TrimSpace(s.git().User()); v != "" {
		return "human:" + v, nil
	}
	return event.UnknownActor, []parse.Diagnostic{warning(parse.CodeMissingActor, "no actor given, set --actor or TKT_ACTOR, recorded as %q", event.UnknownActor)}
}

func (s *Service) appendEvent(in eventInput) (*Appended, error) {
	snap, err := s.load()
	if err != nil {
		return nil, err
	}
	nf, err := snap.Resolve(in.query)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", in.typ, err)
	}
	actor, warnings := in.actor, []parse.Diagnostic(nil)
	if actor == "" {
		actor, warnings = s.ResolveActor(in.actorFlag)
	}
	at := s.Now()
	var st step
	if in.prepare != nil {
		if st, err = in.prepare(context{snap: snap, nf: nf, actor: actor}); err != nil {
			return nil, err
		}
		warnings = append(st.warnings, warnings...)
	}
	id, err := node.NewID(at, s.Rand)
	if err != nil {
		return nil, fmt.Errorf("new id: %w", err)
	}
	data, err := event.Render(event.Draft{Type: in.typ, Actor: actor, At: at, Fields: st.fields, Body: in.body})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", in.typ, err)
	}
	path, err := s.Repo.WriteEvent(nf.Ref, id, data)
	if err != nil {
		return nil, fmt.Errorf("append %s event: %w", in.typ, err)
	}
	return &Appended{Ref: nf.Ref, Event: id, Type: in.typ, Actor: actor, At: parse.FormatTime(at), Path: path, Fields: st.fields, Warnings: warnings}, nil
}
