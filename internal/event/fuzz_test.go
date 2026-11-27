package event_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/parse"
)

func FuzzFold(f *testing.F) {
	f.Add("---\ntype: set-status\nactor: a\nat: 2026-11-27T10:00:00Z\nstatus: done\n---\n\x00---\ntype: set-status\nactor: b\nat: 2026-11-27T10:00:00Z\nstatus: active\n---\n")
	f.Add("---\ntype: note\nat: 2026-11-27T10:00:00Z\n---\n\x00garbage\x00---\ntype: [\n---\n")
	f.Fuzz(func(t *testing.T, joined string) {
		docs := strings.Split(joined, "\x00")
		if len(docs) > 8 {
			docs = docs[:8]
		}
		build := func() []*parse.Event {
			events := make([]*parse.Event, len(docs))
			for i, d := range docs {
				name := fmt.Sprintf("01HZ3M0A1B2C3D4E5F6G7H8J9%d.md", i%3)
				events[i], _ = parse.ParseEvent(name, []byte(d))
			}
			return events
		}
		forward := build()
		reversed := build()
		for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
			reversed[i], reversed[j] = reversed[j], reversed[i]
		}
		a := event.Fold(forward, event.Options{})
		b := event.Fold(reversed, event.Options{})
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("fold depends on input order:\n%+v\n%+v", a, b)
		}
		if a.Status == "" {
			t.Fatal("status must never be empty")
		}
		for _, e := range a.History {
			if !e.Event.Valid || e.Actor == "" {
				t.Fatalf("bad history entry %+v", e)
			}
		}
	})
}
