package event

import (
	"fmt"
	"strings"
	"time"

	"github.com/tektite-tracker/tektite/parse"
)

const DefaultTTL = 30 * time.Minute

type Lease struct {
	Holder  string        `json:"holder"`
	ClaimID string        `json:"claim_id"`
	Since   time.Time     `json:"since"`
	Expires time.Time     `json:"expires"`
	TTL     time.Duration `json:"ttl"`
}

func (l *Lease) LiveAt(t time.Time) bool {
	return l != nil && t.Before(l.Expires)
}

type LinkOp struct {
	Add    bool   `json:"add"`
	Rel    string `json:"rel"`
	Target string `json:"target"`
}

func ParseTTL(v any) (time.Duration, bool) {
	s, ok := v.(string)
	if !ok {
		return 0, false
	}
	d, err := time.ParseDuration(strings.TrimSpace(s))
	return d, err == nil && d > 0
}

func applyClaim(st *State, e *parse.Event, entry *Entry) {
	ttl, ok := ParseTTL(e.Fields["ttl"])
	if !ok {
		rejectField(st, entry, parse.CodeInvalidTTL, "ttl", fmt.Sprintf("event %q has ttl %v, which is not a positive duration such as 30m, the claim is ignored", e.ID, e.Fields["ttl"]))
		return
	}
	next := &Lease{Holder: entry.Actor, ClaimID: e.ID, Since: e.At, Expires: e.At.Add(ttl), TTL: ttl}
	cur := st.Lease
	if cur == nil || !cur.LiveAt(e.At) || cur.Holder == entry.Actor {
		st.Lease = next
		return
	}
	winner, loser := cur, next
	if strings.ToUpper(e.ID) < strings.ToUpper(cur.ClaimID) {
		winner, loser = next, cur
	}
	st.Lease = winner
	if loser == next {
		entry.Applied = false
	}
	st.Diagnostics = append(st.Diagnostics, note(parse.CodeClaimConflict,
		fmt.Sprintf("claims %q by %s and %q by %s overlap, the lower id %q holds the lease", cur.ClaimID, cur.Holder, e.ID, entry.Actor, winner.ClaimID)))
}

func applyHeartbeat(st *State, e *parse.Event, entry *Entry) {
	cur := st.Lease
	if cur == nil || cur.Holder != entry.Actor || !cur.LiveAt(e.At) {
		entry.Applied = false
		st.Diagnostics = append(st.Diagnostics, note(parse.CodeIgnoredLeaseEvent,
			fmt.Sprintf("heartbeat %q by %s is ignored because they hold no live lease", e.ID, entry.Actor)))
		return
	}
	renewed := *cur
	renewed.Expires = e.At.Add(cur.TTL)
	st.Lease = &renewed
}

func applyRelease(st *State, e *parse.Event, entry *Entry) {
	if st.Lease == nil || st.Lease.Holder != entry.Actor {
		entry.Applied = false
		st.Diagnostics = append(st.Diagnostics, note(parse.CodeIgnoredLeaseEvent,
			fmt.Sprintf("release %q by %s is ignored because they hold no lease", e.ID, entry.Actor)))
		return
	}
	st.Lease = nil
}

func applyLink(st *State, e *parse.Event, add bool) {
	st.LinkOps = append(st.LinkOps, LinkOp{
		Add:    add,
		Rel:    strings.TrimSpace(fmt.Sprint(e.Fields["rel"])),
		Target: strings.TrimSpace(fmt.Sprint(e.Fields["target"])),
	})
}
