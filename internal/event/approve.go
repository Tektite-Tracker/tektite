package event

import (
	"fmt"
	"strings"
	"time"

	"github.com/tektite-tracker/tektite/parse"
)

const HumanPrefix = "human:"

type Approval struct {
	Revision int       `json:"revision"`
	Event    string    `json:"event"`
	Actor    string    `json:"actor"`
	At       time.Time `json:"at"`
}

func IsHuman(actor string) bool {
	return strings.HasPrefix(actor, HumanPrefix) && strings.TrimSpace(strings.TrimPrefix(actor, HumanPrefix)) != ""
}

func applyApprove(st *State, e *parse.Event, entry *Entry) {
	n, ok := revisionNumber(e.Fields["revision"])
	if !ok {
		entry.Applied = false
		st.Diagnostics = append(st.Diagnostics, parse.Diagnostic{
			Code:     parse.CodeInvalidRevision,
			Severity: parse.SeverityWarning,
			Message:  fmt.Sprintf("approve event %q has revision %v, which is not a positive integer, so it is ignored", e.ID, e.Fields["revision"]),
			Field:    "revision",
		})
		return
	}
	st.Approvals = append(st.Approvals, Approval{Revision: n, Event: e.ID, Actor: entry.Actor, At: e.At})
}

func (s State) EffectiveRevision() int {
	if s.Revision < 1 {
		return 1
	}
	return s.Revision
}

func (s State) Approved() (bool, string) {
	current := s.EffectiveRevision()
	var humans []Approval
	for _, a := range s.Approvals {
		if IsHuman(a.Actor) {
			humans = append(humans, a)
		}
	}
	for _, a := range humans {
		if a.Revision == current {
			return true, fmt.Sprintf("approved by %s at revision %d", a.Actor, current)
		}
	}
	if len(humans) > 0 {
		last := humans[len(humans)-1]
		return false, fmt.Sprintf("approval was for revision %d, the current revision is %d", last.Revision, current)
	}
	if len(s.Approvals) > 0 {
		return false, "it has no human approval, approvals by agents do not count"
	}
	return false, "it has no approval"
}
