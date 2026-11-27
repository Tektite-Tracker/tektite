package event_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/parse"
)

func lev(id, typ, actor, at, extra string) *parse.Event {
	src := fmt.Sprintf("---\ntype: %s\nactor: %s\nat: %s\n%s---\n", typ, actor, at, extra)
	e, _ := parse.ParseEvent(id+".md", []byte(src))
	return e
}

func at(h, m int) time.Time {
	return time.Date(2026, 11, 27, h, m, 0, 0, time.UTC)
}

func TestLeaseFold(t *testing.T) {
	t.Run("The lease time comes from the event, not the clock of the reader", func(t *testing.T) {
		st := event.Fold([]*parse.Event{lev("01HZ3M0A1B2C3D4E5F6G7H8J91", "claim", "agent:a", "2026-11-27T10:00:00Z", "ttl: 30m\n")}, event.Options{})
		if st.Lease == nil || st.Lease.Holder != "agent:a" {
			t.Fatalf("lease = %+v", st.Lease)
		}
		if !st.Lease.LiveAt(at(10, 20)) || st.Lease.LiveAt(at(10, 31)) {
			t.Errorf("expires = %v", st.Lease.Expires)
		}
	})

	t.Run("Simultaneous claims are resolved deterministically", func(t *testing.T) {
		lo := lev("01HZ3M0A1B2C3D4E5F6G7H8J91", "claim", "agent:a", "2026-11-27T10:00:00Z", "ttl: 30m\n")
		hi := lev("01HZ3M0A1B2C3D4E5F6G7H8J92", "claim", "agent:b", "2026-11-27T10:00:00Z", "ttl: 30m\n")
		for _, in := range [][]*parse.Event{{lo, hi}, {hi, lo}} {
			st := event.Fold(in, event.Options{})
			if st.Lease.Holder != "agent:a" {
				t.Errorf("holder = %q", st.Lease.Holder)
			}
			found := false
			for _, d := range st.Diagnostics {
				found = found || d.Code == parse.CodeClaimConflict
			}
			if !found {
				t.Errorf("no conflict note: %v", st.Diagnostics)
			}
		}
		skewed := lev("01HZ3M0A1B2C3D4E5F6G7H8J90", "claim", "agent:b", "2026-11-27T10:05:00Z", "ttl: 30m\n")
		st := event.Fold([]*parse.Event{lo, skewed}, event.Options{})
		if st.Lease.Holder != "agent:b" {
			t.Errorf("lower ULID holds, got %q", st.Lease.Holder)
		}
	})

	t.Run("An expired lease can be taken over", func(t *testing.T) {
		a := lev("01HZ3M0A1B2C3D4E5F6G7H8J91", "claim", "agent:a", "2026-11-27T10:00:00Z", "ttl: 30m\n")
		b := lev("01HZ3M0A1B2C3D4E5F6G7H8J92", "claim", "agent:b", "2026-11-27T10:40:00Z", "ttl: 30m\n")
		st := event.Fold([]*parse.Event{a, b}, event.Options{})
		if st.Lease.Holder != "agent:b" || len(st.Diagnostics) != 0 {
			t.Errorf("lease = %+v diags = %v", st.Lease, st.Diagnostics)
		}
	})

	t.Run("A heartbeat extends the lease", func(t *testing.T) {
		a := lev("01HZ3M0A1B2C3D4E5F6G7H8J91", "claim", "agent:a", "2026-11-27T10:00:00Z", "ttl: 30m\n")
		hb := lev("01HZ3M0A1B2C3D4E5F6G7H8J92", "heartbeat", "agent:a", "2026-11-27T10:25:00Z", "")
		late := lev("01HZ3M0A1B2C3D4E5F6G7H8J93", "heartbeat", "agent:a", "2026-11-27T11:00:00Z", "")
		st := event.Fold([]*parse.Event{a, hb}, event.Options{})
		if !st.Lease.Expires.Equal(at(10, 55)) {
			t.Errorf("expires = %v", st.Lease.Expires)
		}
		st = event.Fold([]*parse.Event{a, hb, late}, event.Options{})
		if !st.Lease.Expires.Equal(at(10, 55)) {
			t.Errorf("an expired lease was revived: %v", st.Lease.Expires)
		}
	})

	t.Run("Only the holder can heartbeat or release", func(t *testing.T) {
		a := lev("01HZ3M0A1B2C3D4E5F6G7H8J91", "claim", "agent:a", "2026-11-27T10:00:00Z", "ttl: 30m\n")
		hb := lev("01HZ3M0A1B2C3D4E5F6G7H8J92", "heartbeat", "agent:b", "2026-11-27T10:10:00Z", "")
		rel := lev("01HZ3M0A1B2C3D4E5F6G7H8J93", "release", "agent:b", "2026-11-27T10:11:00Z", "")
		st := event.Fold([]*parse.Event{a, hb, rel}, event.Options{})
		if st.Lease == nil || st.Lease.Holder != "agent:a" || !st.Lease.Expires.Equal(at(10, 30)) {
			t.Errorf("lease = %+v", st.Lease)
		}
	})

	t.Run("Releasing frees the node immediately", func(t *testing.T) {
		a := lev("01HZ3M0A1B2C3D4E5F6G7H8J91", "claim", "agent:a", "2026-11-27T10:00:00Z", "ttl: 30m\n")
		rel := lev("01HZ3M0A1B2C3D4E5F6G7H8J92", "release", "agent:a", "2026-11-27T10:05:00Z", "")
		if st := event.Fold([]*parse.Event{a, rel}, event.Options{}); st.Lease != nil {
			t.Errorf("lease = %+v", st.Lease)
		}
	})

	t.Run("An invalid ttl is ignored with a warning", func(t *testing.T) {
		bad := lev("01HZ3M0A1B2C3D4E5F6G7H8J91", "claim", "agent:a", "2026-11-27T10:00:00Z", "ttl: soon\n")
		st := event.Fold([]*parse.Event{bad}, event.Options{})
		if st.Lease != nil || len(st.Diagnostics) != 1 || st.Diagnostics[0].Code != parse.CodeInvalidTTL {
			t.Errorf("lease = %+v diags = %v", st.Lease, st.Diagnostics)
		}
	})
}
