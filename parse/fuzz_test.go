package parse_test

import (
	"testing"

	"github.com/tektite-tracker/tektite/parse"
)

func FuzzParseNode(f *testing.F) {
	f.Add([]byte("---\nid: 1\ntitle: t\n---\n# H\n\n- [ ] <!-- g:ABC --> x [[a|b]]\n"))
	f.Add([]byte("---"))
	f.Add([]byte("---\ntitle: [unclosed\n---\n"))
	f.Fuzz(func(t *testing.T, src []byte) {
		n, d := parse.ParseNode("x.md", src)
		if n == nil || d == nil || n.Title == "" || n.Kind == "" || n.Blocks == nil || n.Links == nil {
			t.Fatalf("incomplete result for %q: %+v %v", src, n, d)
		}
	})
}

func FuzzParseEvent(f *testing.F) {
	f.Add([]byte("---\ntype: note\nactor: a\nat: 2026-11-27T10:00:00Z\n---\nbody"))
	f.Add([]byte("---"))
	f.Add([]byte("---\ntype: link\nat: 5\n---\n"))
	f.Fuzz(func(t *testing.T, src []byte) {
		e, d := parse.ParseEvent("01HZ3M0A1B2C3D4E5F6G7H8J9K.md", src)
		if e == nil || d == nil || e.Fields == nil {
			t.Fatalf("incomplete result for %q", src)
		}
		if e.Valid && (e.Type == "" || e.At.IsZero()) {
			t.Fatalf("valid event without type or time: %+v", e)
		}
	})
}
