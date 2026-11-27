package event

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tektite-tracker/tektite/parse"
)

const (
	StatusOpen   = "open"
	UnknownActor = "unknown"
)

var knownStatuses = map[string]bool{
	"open": true, "active": true, "blocked": true, "review": true, "done": true, "dropped": true,
}

// Entry is one folded event in a node's history.
type Entry struct {
	Event    *parse.Event `json:"event"`
	Actor    string       `json:"actor"`
	Applied  bool         `json:"applied"`
	Override string       `json:"override,omitempty"`
}

// State is a node's state computed from its events.
type State struct {
	Status      string             `json:"status"`
	History     []Entry            `json:"history"`
	Diagnostics []parse.Diagnostic `json:"diagnostics"`
}

// Options tunes a fold. DefaultActor attributes events that declare no actor.
type Options struct {
	DefaultActor string
}

type handler func(st *State, e *parse.Event, entry *Entry)

var handlers = map[string]handler{
	"set-status": applySetStatus,
}

// Order sorts events by time, then by ULID, without changing its input.
func Order(events []*parse.Event) []*parse.Event {
	out := make([]*parse.Event, 0, len(events))
	for _, e := range events {
		if e != nil {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if !a.At.Equal(b.At) {
			return a.At.Before(b.At)
		}
		return strings.ToUpper(a.ID) < strings.ToUpper(b.ID)
	})
	return out
}

// Fold computes a node's state from its parsed events.
func Fold(events []*parse.Event, opts Options) State {
	if opts.DefaultActor == "" {
		opts.DefaultActor = UnknownActor
	}
	st := State{Status: StatusOpen, History: []Entry{}, Diagnostics: []parse.Diagnostic{}}
	for _, e := range Order(events) {
		if !e.Valid {
			st.Diagnostics = append(st.Diagnostics, note(parse.CodeEventSkipped, fmt.Sprintf("event %q is invalid and skipped", e.ID)))
			continue
		}
		entry := Entry{Event: e, Actor: e.Actor}
		if entry.Actor == "" {
			entry.Actor = opts.DefaultActor
		}
		if h, ok := handlers[e.Type]; ok {
			entry.Applied = true
			h(&st, e, &entry)
		}
		st.History = append(st.History, entry)
	}
	return st
}

func applySetStatus(st *State, e *parse.Event, entry *Entry) {
	status := fmt.Sprint(e.Fields["status"])
	if !knownStatuses[status] {
		st.Diagnostics = append(st.Diagnostics, parse.Diagnostic{
			Code:     parse.CodeUnknownStatus,
			Severity: parse.SeverityWarning,
			Message:  fmt.Sprintf("event %q sets unknown status %q", e.ID, status),
			Field:    "status",
		})
	}
	st.Status = status
	if v, ok := e.Fields["override"]; ok && v != nil && v != "" {
		entry.Override = fmt.Sprint(v)
	}
}

func note(code, msg string) parse.Diagnostic {
	return parse.Diagnostic{Code: code, Severity: parse.SeverityNote, Message: msg}
}
