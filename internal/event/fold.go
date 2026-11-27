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

type Entry struct {
	Event    *parse.Event `json:"event"`
	Actor    string       `json:"actor"`
	Applied  bool         `json:"applied"`
	Override string       `json:"override,omitempty"`
}

type State struct {
	Status      string             `json:"status"`
	Lease       *Lease             `json:"lease"`
	LinkOps     []LinkOp           `json:"link_ops"`
	History     []Entry            `json:"history"`
	Diagnostics []parse.Diagnostic `json:"diagnostics"`
}

type Options struct {
	DefaultActor string
}

type handler func(st *State, e *parse.Event, entry *Entry)

var handlers = map[string]handler{
	"set-status": applySetStatus,
	"note":       func(*State, *parse.Event, *Entry) {},
	"claim":      applyClaim,
	"heartbeat":  applyHeartbeat,
	"release":    applyRelease,
	"link":       func(st *State, e *parse.Event, _ *Entry) { applyLink(st, e, true) },
	"unlink":     func(st *State, e *parse.Event, _ *Entry) { applyLink(st, e, false) },
}

func Order(events []*parse.Event) []*parse.Event {
	out := make([]*parse.Event, 0, len(events))
	for _, e := range events {
		if e != nil {
			out = append(out, e)
		}
	}
	keys := make(map[*parse.Event]string, len(out))
	for _, e := range out {
		keys[e] = fmt.Sprintf("%#v", *e)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if !a.At.Equal(b.At) {
			return a.At.Before(b.At)
		}
		if ai, bi := strings.ToUpper(a.ID), strings.ToUpper(b.ID); ai != bi {
			return ai < bi
		}
		return keys[a] < keys[b]
	})
	return out
}

func Fold(events []*parse.Event, opts Options) State {
	if opts.DefaultActor == "" {
		opts.DefaultActor = UnknownActor
	}
	st := State{Status: StatusOpen, LinkOps: []LinkOp{}, History: []Entry{}, Diagnostics: []parse.Diagnostic{}}
	seen := map[string]bool{}
	warned := map[string]bool{}
	var prev *Entry
	for _, e := range Order(events) {
		key := strings.ToUpper(e.ID)
		if seen[key] && !warned[key] {
			warned[key] = true
			st.Diagnostics = append(st.Diagnostics, parse.Diagnostic{
				Code:     parse.CodeDuplicateEventID,
				Severity: parse.SeverityWarning,
				Message:  fmt.Sprintf("event id %q is used by more than one event, all are kept", e.ID),
			})
		}
		seen[key] = true
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
			if e.Type == "set-status" {
				if prev != nil && prev.Event.At.Equal(e.At) && prev.Actor != entry.Actor {
					st.Diagnostics = append(st.Diagnostics, note(parse.CodeConcurrentStatus,
						fmt.Sprintf("events %q and %q change the status at the same time from different actors, %q wins", prev.Event.ID, e.ID, e.ID)))
				}
				cur := entry
				prev = &cur
			}
		}
		st.History = append(st.History, entry)
	}
	return st
}

func applySetStatus(st *State, e *parse.Event, entry *Entry) {
	status := strings.TrimSpace(fmt.Sprint(e.Fields["status"]))
	if lower := strings.ToLower(status); !knownStatuses[status] && knownStatuses[lower] {
		st.Diagnostics = append(st.Diagnostics, parse.Diagnostic{
			Code:     parse.CodeUnknownStatus,
			Severity: parse.SeverityWarning,
			Message:  fmt.Sprintf("event %q sets status %q, which differs in case from the shared status %q", e.ID, status, lower),
			Field:    "status",
		})
	} else if !knownStatuses[status] {
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
