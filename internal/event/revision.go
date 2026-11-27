package event

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/tektite-tracker/tektite/parse"
)

type Revision struct {
	Number int       `json:"number"`
	Event  string    `json:"event"`
	Actor  string    `json:"actor"`
	At     time.Time `json:"at"`
	Body   string    `json:"body"`
}

func revisionNumber(v any) (int, bool) {
	if v == nil {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(fmt.Sprint(v)))
	return n, err == nil && n >= 1
}

func applyRevision(st *State, e *parse.Event, entry *Entry) {
	n, ok := revisionNumber(e.Fields["number"])
	if !ok {
		rejectField(st, entry, parse.CodeInvalidRevision, "number", fmt.Sprintf("revision event %q has number %v, which is not a positive integer, so it is ignored", e.ID, e.Fields["number"]))
		return
	}
	for _, prev := range st.Revisions {
		if prev.Number == n && !divergedReported(st, n) {
			st.Diagnostics = append(st.Diagnostics, parse.Diagnostic{
				Code:     parse.CodePlanDiverged,
				Severity: parse.SeverityWarning,
				Message:  fmt.Sprintf("the plan has diverged: revision %d is written by events %q and %q, the later one is current", n, prev.Event, e.ID),
				Field:    "number",
			})
			break
		}
	}
	actor := entry.Actor
	st.Revisions = append(st.Revisions, Revision{Number: n, Event: e.ID, Actor: actor, At: e.At, Body: e.Body})
	if n >= st.Revision {
		st.Revision, st.Body = n, e.Body
	}
}

func divergedReported(st *State, n int) bool {
	marker := fmt.Sprintf("revision %d is written", n)
	for _, d := range st.Diagnostics {
		if d.Code == parse.CodePlanDiverged && strings.Contains(d.Message, marker) {
			return true
		}
	}
	return false
}

func (s State) RevisionNumbered(n int) (Revision, bool) {
	for i := len(s.Revisions) - 1; i >= 0; i-- {
		if s.Revisions[i].Number == n {
			return s.Revisions[i], true
		}
	}
	return Revision{}, false
}

func (s State) Diverged(n int) bool {
	count := 0
	for _, r := range s.Revisions {
		if r.Number == n {
			count++
		}
	}
	return count > 1
}

func (s State) NextRevision() int {
	highest := 0
	for _, r := range s.Revisions {
		if r.Number > highest {
			highest = r.Number
		}
	}
	return highest + 1
}
