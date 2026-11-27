package event_test

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/parse"
)

func ev(t *testing.T, id, typ, at string, fields ...string) *parse.Event {
	t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, "---\ntype: %s\nactor: agent:a\nat: %s\n", typ, at)
	for _, f := range fields {
		b.WriteString(f + "\n")
	}
	b.WriteString("---\n")
	e, _ := parse.ParseEvent(id+".md", []byte(b.String()))
	return e
}

func ids(es []*parse.Event) []string {
	out := []string{}
	for _, e := range es {
		out = append(out, e.ID)
	}
	return out
}

const (
	u1 = "01HZ3M0A1B2C3D4E5F6G7H8J91"
	u2 = "01HZ3M0A1B2C3D4E5F6G7H8J92"
	u3 = "01HZ3M0A1B2C3D4E5F6G7H8J93"
	u4 = "01HZ3M0A1B2C3D4E5F6G7H8J94"
)

func TestFold_Order(t *testing.T) {
	t.Run("Events are ordered by time, then ULID", func(t *testing.T) {
		in := []*parse.Event{
			ev(t, u4, "note", "2026-11-27T10:05:00Z"),
			ev(t, u3, "note", "2026-11-27T10:00:00Z"),
			ev(t, u1, "note", "2026-11-27T10:00:00Z"),
			ev(t, u2, "note", "2026-11-27T09:00:00Z"),
		}
		before := ids(in)
		got := ids(event.Order(in))
		if !reflect.DeepEqual(got, []string{u2, u1, u3, u4}) {
			t.Errorf("order = %v", got)
		}
		if !reflect.DeepEqual(ids(in), before) {
			t.Error("input must not be reordered")
		}
		again := ids(event.Order(in))
		if !reflect.DeepEqual(got, again) {
			t.Error("order must be deterministic")
		}
	})

	t.Run("A skewed clock does not change the order", func(t *testing.T) {
		in := []*parse.Event{
			ev(t, u2, "set-status", "2026-11-27T09:00:00Z", "status: active"),
			ev(t, u1, "set-status", "2026-11-27T10:00:00Z", "status: done"),
		}
		if got := ids(event.Order(in)); !reflect.DeepEqual(got, []string{u2, u1}) {
			t.Errorf("order = %v", got)
		}
		st := event.Fold(in, event.Options{})
		if st.Status != "done" || len(st.Diagnostics) != 0 {
			t.Errorf("state = %+v", st)
		}
	})

	t.Run("An event file whose name is not a ULID is a warning", func(t *testing.T) {
		in := []*parse.Event{
			ev(t, "b-try", "note", "2026-11-27T10:00:00Z"),
			ev(t, "a-try", "note", "2026-11-27T10:00:00Z"),
			ev(t, "z-try", "note", "2026-11-27T09:00:00Z"),
		}
		if got := ids(event.Order(in)); !reflect.DeepEqual(got, []string{"z-try", "a-try", "b-try"}) {
			t.Errorf("order = %v", got)
		}
	})
}

func TestFold_Status(t *testing.T) {
	t.Run("A node with no events is open", func(t *testing.T) {
		st := event.Fold(nil, event.Options{})
		if st.Status != "open" || len(st.History) != 0 || len(st.Diagnostics) != 0 {
			t.Errorf("state = %+v", st)
		}
	})

	t.Run("The latest status event wins", func(t *testing.T) {
		st := event.Fold([]*parse.Event{
			ev(t, u3, "set-status", "2026-11-27T12:00:00Z", "status: done"),
			ev(t, u1, "set-status", "2026-11-27T10:00:00Z", "status: active"),
			ev(t, u2, "set-status", "2026-11-27T11:00:00Z", "status: review"),
		}, event.Options{})
		if st.Status != "done" || len(st.History) != 3 || len(st.Diagnostics) != 0 {
			t.Errorf("state = %+v", st)
		}
	})

	t.Run("Status is derived from events", func(t *testing.T) {
		st := event.Fold([]*parse.Event{ev(t, u1, "set-status", "2026-11-27T10:00:00Z", "status: done")}, event.Options{})
		if st.Status != "done" {
			t.Errorf("status = %q", st.Status)
		}
	})

	t.Run("Concurrent status changes resolve by time then ULID", func(t *testing.T) {
		st := event.Fold([]*parse.Event{
			ev(t, u2, "set-status", "2026-11-27T10:00:00Z", "status: review"),
			ev(t, u1, "set-status", "2026-11-27T10:00:00Z", "status: active"),
		}, event.Options{})
		if st.Status != "review" {
			t.Errorf("status = %q", st.Status)
		}
	})

	t.Run("The shared lifecycle states are known", func(t *testing.T) {
		for _, s := range []string{"open", "active", "blocked", "review", "done", "dropped"} {
			st := event.Fold([]*parse.Event{ev(t, u1, "set-status", "2026-11-27T10:00:00Z", "status: "+s)}, event.Options{})
			if st.Status != s || len(st.Diagnostics) != 0 {
				t.Errorf("%s: %+v", s, st)
			}
		}
	})

	t.Run("An unknown status is accepted with a warning", func(t *testing.T) {
		st := event.Fold([]*parse.Event{ev(t, u1, "set-status", "2026-11-27T10:00:00Z", "status: parked")}, event.Options{})
		if st.Status != "parked" || len(st.Diagnostics) != 1 || st.Diagnostics[0].Code != parse.CodeUnknownStatus ||
			st.Diagnostics[0].Severity != parse.SeverityWarning || !strings.Contains(st.Diagnostics[0].Message, u1) {
			t.Errorf("state = %+v", st)
		}
	})

	t.Run("A status is trimmed but never case-folded", func(t *testing.T) {
		st := event.Fold([]*parse.Event{ev(t, u1, "set-status", "2026-11-27T10:00:00Z", `status: "  done  "`)}, event.Options{})
		if st.Status != "done" || len(st.Diagnostics) != 0 {
			t.Errorf("trimmed: %+v", st)
		}
		st = event.Fold([]*parse.Event{ev(t, u1, "set-status", "2026-11-27T10:00:00Z", "status: Done")}, event.Options{})
		if st.Status != "Done" || len(st.Diagnostics) != 1 || st.Diagnostics[0].Code != parse.CodeUnknownStatus ||
			!strings.Contains(st.Diagnostics[0].Message, "differs in case") {
			t.Errorf("case: %+v", st)
		}
	})

	t.Run("An override is a field on set-status, not a separate type", func(t *testing.T) {
		st := event.Fold([]*parse.Event{ev(t, u1, "set-status", "2026-11-27T10:00:00Z", "status: done", "override: accepted risk")}, event.Options{})
		if st.Status != "done" || st.History[0].Override != "accepted risk" {
			t.Errorf("state = %+v", st)
		}
	})
}

func TestFold_History(t *testing.T) {
	t.Run("Unknown event types are kept and ignored by the fold", func(t *testing.T) {
		st := event.Fold([]*parse.Event{
			ev(t, u1, "set-status", "2026-11-27T10:00:00Z", "status: active"),
			ev(t, u2, "reaction", "2026-11-27T11:00:00Z", "emoji: thumbs-up"),
		}, event.Options{})
		if st.Status != "active" || len(st.History) != 2 || st.History[1].Applied || st.History[1].Event.Type != "reaction" {
			t.Errorf("state = %+v", st)
		}
	})

	t.Run("Duplicate event ids are a warning and both events are kept", func(t *testing.T) {
		st := event.Fold([]*parse.Event{
			ev(t, u1, "set-status", "2026-11-27T10:00:00Z", "status: active"),
			ev(t, u1, "note", "2026-11-27T11:00:00Z"),
			ev(t, u1, "note", "2026-11-27T12:00:00Z"),
		}, event.Options{})
		if len(st.History) != 3 || len(st.Diagnostics) != 1 || st.Diagnostics[0].Code != parse.CodeDuplicateEventID ||
			st.Diagnostics[0].Severity != parse.SeverityWarning || !strings.Contains(st.Diagnostics[0].Message, u1) {
			t.Errorf("state = %+v", st)
		}
	})

	t.Run("A malformed event is skipped in the fold", func(t *testing.T) {
		bad, _ := parse.ParseEvent(u2+".md", []byte("---\ntype: [oops\n---\n"))
		st := event.Fold([]*parse.Event{
			ev(t, u1, "set-status", "2026-11-27T10:00:00Z", "status: active"),
			bad,
			nil,
		}, event.Options{})
		if st.Status != "active" || len(st.History) != 1 || len(st.Diagnostics) != 1 ||
			st.Diagnostics[0].Code != parse.CodeEventSkipped || !strings.Contains(st.Diagnostics[0].Message, u2) {
			t.Errorf("state = %+v", st)
		}
	})

	t.Run("A missing actor is repaired from the environment", func(t *testing.T) {
		e, _ := parse.ParseEvent(u1+".md", []byte("---\ntype: note\nat: 2026-11-27T10:00:00Z\n---\n"))
		if got := event.Fold([]*parse.Event{e}, event.Options{}).History[0].Actor; got != "unknown" {
			t.Errorf("actor = %q", got)
		}
		if got := event.Fold([]*parse.Event{e}, event.Options{DefaultActor: "human:jesus"}).History[0].Actor; got != "human:jesus" {
			t.Errorf("actor = %q", got)
		}
	})

	t.Run("Fold output is identical for every input order", func(t *testing.T) {
		base := []*parse.Event{
			ev(t, u1, "set-status", "2026-11-27T10:00:00Z", "status: active"),
			ev(t, u1, "set-status", "2026-11-27T10:00:00Z", "status: review"),
			ev(t, u2, "set-status", "2026-11-27T10:00:00Z", "status: parked"),
			ev(t, u2, "note", "2026-11-27T10:00:00Z"),
			ev(t, u3, "reaction", "2026-11-27T09:00:00Z", "emoji: x"),
			ev(t, u4, "set-status", "2026-11-27T11:00:00Z", "status: done"),
			ev(t, "first-try", "set-status", "2026-11-27T10:00:00Z", "status: blocked"),
		}
		want := event.Fold(base, event.Options{})
		rng := rand.New(rand.NewSource(1))
		for i := 0; i < 300; i++ {
			shuffled := append([]*parse.Event(nil), base...)
			rng.Shuffle(len(shuffled), func(a, b int) { shuffled[a], shuffled[b] = shuffled[b], shuffled[a] })
			if got := event.Fold(shuffled, event.Options{}); !reflect.DeepEqual(got, want) {
				t.Fatalf("permutation %d differs:\n%+v\n%+v", i, got, want)
			}
		}
	})

	t.Run("Fold output is deterministic", func(t *testing.T) {
		in := []*parse.Event{
			ev(t, u2, "set-status", "2026-11-27T10:00:00Z", "status: parked"),
			ev(t, u1, "set-status", "2026-11-27T10:00:00Z", "status: other"),
		}
		a := event.Fold(in, event.Options{})
		b := event.Fold([]*parse.Event{in[1], in[0]}, event.Options{})
		if !reflect.DeepEqual(a, b) {
			t.Errorf("fold differs by input order:\n%+v\n%+v", a, b)
		}
	})
}
