package event_test

import (
	"testing"
	"time"

	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/internal/node"
	"github.com/tektite-tracker/tektite/parse"
)

func TestRender(t *testing.T) {
	t.Run("A written event parses back", func(t *testing.T) {
		at := time.Date(2026, 11, 27, 10, 15, 0, 0, time.UTC)
		data, err := event.Render(event.Draft{
			Type: "set-status", Actor: "agent:claude-opus/s-4f2a", At: at,
			Fields: []node.Field{{Key: "status", Value: "done"}, {Key: "override", Value: "accepted risk: yes"}},
			Body:   "Starting on the retry logic.\n",
		})
		if err != nil {
			t.Fatal(err)
		}
		ev, diags := parse.ParseEvent("01HZ3M0A1B2C3D4E5F6G7H8J9K.md", data)
		if !ev.Valid || len(diags) != 0 {
			t.Fatalf("event = %+v diags = %v\n%s", ev, diags, data)
		}
		if ev.Type != "set-status" || ev.Actor != "agent:claude-opus/s-4f2a" || !ev.At.Equal(at) ||
			ev.Fields["status"] != "done" || ev.Fields["override"] != "accepted risk: yes" || ev.Body != "Starting on the retry logic." {
			t.Errorf("event = %+v", ev)
		}
		if _, err := event.Render(event.Draft{}); err == nil {
			t.Error("an event without a type renders")
		}
	})
}
