package parse_test

import (
	"testing"
	"time"

	"github.com/simplehuman/tektite/parse"
)

func TestEvent_ParseEvent(t *testing.T) {
	t.Run("Parsing an event", func(t *testing.T) {
		e, d := goldenEvent(t, "set-status")
		if e.Type != "set-status" || e.Actor != "agent:claude-opus/s-4f2a" || e.Fields["status"] != "active" {
			t.Errorf("event = %+v", e)
		}
		if !e.At.Equal(time.Date(2026, 11, 27, 10, 15, 0, 0, time.UTC)) {
			t.Errorf("at = %v", e.At)
		}
		if e.Body != "Starting on the retry logic." || e.ID != "01HZ3M0A1B2C3D4E5F6G7H8J9K" {
			t.Errorf("body=%q id=%q", e.Body, e.ID)
		}
		if len(d) != 0 || !e.Valid {
			t.Errorf("diags = %v", d)
		}
	})
}

func TestEvent_ActorUnsigned(t *testing.T) {
	t.Run("The actor is declared and unsigned", func(t *testing.T) {
		e, d := goldenEvent(t, "actor-human")
		if e.Actor != "human:jesus" || len(d) != 0 {
			t.Errorf("actor=%q diags=%v", e.Actor, d)
		}
	})
}

func TestEvent_MissingActor(t *testing.T) {
	t.Run("A missing actor is repaired from the environment", func(t *testing.T) {
		e, d := goldenEvent(t, "missing-actor")
		if !hasCode(d, parse.CodeMissingActor) || d[0].Severity != parse.SeverityWarning {
			t.Errorf("diags = %v", d)
		}
		if e.Actor != "" || !e.Valid {
			t.Errorf("event should stay valid with empty actor: %+v", e)
		}
	})
}

func TestEvent_UnknownType(t *testing.T) {
	t.Run("Unknown event types are kept and ignored by the fold", func(t *testing.T) {
		e, d := goldenEvent(t, "unknown-type")
		if e.Type != "reaction" || e.Known || !e.Valid || e.Fields["emoji"] != "thumbs-up" {
			t.Errorf("event = %+v", e)
		}
		if !hasCode(d, parse.CodeUnknownEventType) {
			t.Errorf("diags = %v", d)
		}
	})

	t.Run("The list can only grow in a minor version", func(t *testing.T) {
		e, _ := goldenEvent(t, "unknown-type")
		if !e.Valid || e.Type == "" {
			t.Errorf("unknown type must be kept: %+v", e)
		}
	})
}

func TestEvent_RequiredFields(t *testing.T) {
	t.Run("A missing required field makes the event invalid", func(t *testing.T) {
		e, d := goldenEvent(t, "missing-required")
		if e.Valid || !hasCode(d, parse.CodeMissingRequiredField) || d[0].Field != "ttl" {
			t.Errorf("event=%+v diags=%v", e, d)
		}
	})

	t.Run("Each type has defined fields and effect", func(t *testing.T) {
		for typ, required := range parse.EventRequiredFields {
			t.Run(typ, func(t *testing.T) {
				if len(required) == 0 {
					return
				}
				src := []byte("---\ntype: " + typ + "\nactor: a\nat: 2026-11-27T10:00:00Z\n---\n")
				e, d := parse.ParseEvent("x.md", src)
				if e.Valid || !hasCode(d, parse.CodeMissingRequiredField) {
					t.Errorf("%s without its required fields should be invalid", typ)
				}
			})
		}
	})
}

func TestEvent_Malformed(t *testing.T) {
	t.Run("A malformed event is skipped in the fold", func(t *testing.T) {
		e, d := goldenEvent(t, "malformed")
		if e.Valid || !hasCode(d, parse.CodeMalformedFrontmatter) || d[0].Line == 0 {
			t.Errorf("event=%+v diags=%v", e, d)
		}
		mt, mtd := goldenEvent(t, "missing-type")
		if mt.Valid || !hasCode(mtd, parse.CodeMissingEventType) {
			t.Errorf("missing type: %v", mtd)
		}
		bt, btd := goldenEvent(t, "bad-time")
		if bt.Valid || !hasCode(btd, parse.CodeInvalidEventTime) {
			t.Errorf("bad time: %v", btd)
		}
	})
}
