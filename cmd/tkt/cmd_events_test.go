package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tektite-tracker/tektite/parse"
)

func eventFiles(t *testing.T, s *sandbox, ref string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(s.root, ".tektite", "events", ref))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatal(err)
	}
	out := []string{}
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func eventSandbox(t *testing.T) *sandbox {
	s := newSandbox(t)
	s.node("retry-plan-9de", idC, "plan", "", "", "plan")
	s.node("auth-retry-3fa", idA, "ticket", "", "", "x")
	s.env = map[string]string{"TKT_ACTOR": "agent:claude-opus/s-4f2a"}
	return s
}

func TestCLI_Verbs(t *testing.T) {
	t.Run("Setting a status", func(t *testing.T) {
		s := eventSandbox(t)
		got := s.tkt("set-status", "auth-retry-3fa", "review", "--json")
		var doc map[string]any
		decode(t, got.stdout, &doc)
		if got.code != 0 || doc["type"] != "set-status" || doc["actor"] != "agent:claude-opus/s-4f2a" {
			t.Fatalf("result = %+v", got)
		}
		files := eventFiles(t, s, "auth-retry-3fa")
		if len(files) != 1 || !parse.IsULID(strings.TrimSuffix(files[0], ".md")) {
			t.Fatalf("files = %v", files)
		}
		ev, diags := parse.ParseEvent(files[0], []byte(s.read(".tektite/events/auth-retry-3fa/"+files[0])))
		if !ev.Valid || ev.Fields["status"] != "review" || len(diags) != 0 {
			t.Errorf("event = %+v diags = %v", ev, diags)
		}
		if got := s.tkt("show", "3fa"); !strings.Contains(got.stdout, "auth-retry-3fa  ticket  review") {
			t.Errorf("show = %s", got.stdout)
		}
		if got := s.tkt("ls"); !strings.Contains(got.stdout, "review") {
			t.Errorf("ls = %s", got.stdout)
		}
	})

	t.Run("An override is a field on set-status, not a separate type", func(t *testing.T) {
		s := eventSandbox(t)
		if got := s.tkt("set-status", "auth-retry-3fa", "done", "--override", "accepted risk"); got.code != 0 {
			t.Fatalf("result = %+v", got)
		}
		files := eventFiles(t, s, "auth-retry-3fa")
		ev, _ := parse.ParseEvent(files[0], []byte(s.read(".tektite/events/auth-retry-3fa/"+files[0])))
		if ev.Type != "set-status" || ev.Fields["override"] != "accepted risk" {
			t.Errorf("event = %+v", ev)
		}
	})

	t.Run("An unknown status is accepted with a warning", func(t *testing.T) {
		s := eventSandbox(t)
		got := s.tkt("set-status", "auth-retry-3fa", "parked")
		if got.code != 0 || !strings.Contains(got.stderr, "TKT-W009") || !strings.Contains(got.stderr, "parked") {
			t.Fatalf("result = %+v", got)
		}
		if got := s.tkt("show", "3fa"); !strings.Contains(got.stdout, "ticket  parked") {
			t.Errorf("show = %s", got.stdout)
		}
		if got := s.tkt("set-status", "auth-retry-3fa", "  "); got.code != exitUsage {
			t.Errorf("empty status: %+v", got)
		}
	})

	t.Run("A verb on a node that does not exist", func(t *testing.T) {
		s := eventSandbox(t)
		for _, args := range [][]string{{"note", "ghost-000", "hi"}, {"set-status", "ghost-000", "done"}, {"link", "ghost-000", "implements", "x-123"}, {"unlink", "ghost-000", "implements", "x-123"}} {
			got := s.tkt(args...)
			if got.code != exitNotFound {
				t.Errorf("%v: %+v", args, got)
			}
		}
		if _, err := os.Stat(filepath.Join(s.root, ".tektite", "events", "ghost-000")); err == nil {
			t.Error("an event directory was created for a missing node")
		}
	})

	t.Run("Adding a note", func(t *testing.T) {
		s := eventSandbox(t)
		if got := s.tkt("note", "auth-retry-3fa", "backoff needs jitter"); got.code != 0 {
			t.Fatalf("result = %+v", got)
		}
		s.now = s.now.Add(time.Second)
		if got := s.tktIn(s.root, "from stdin\nsecond line", "note", "auth-retry-3fa", "-"); got.code != 0 {
			t.Fatalf("stdin note: %+v", got)
		}
		if got := s.tkt("note", "auth-retry-3fa", " "); got.code != exitUsage {
			t.Errorf("empty note: %+v", got)
		}
		got := s.tkt("show", "auth-retry-3fa")
		for _, want := range []string{"notes:", "agent:claude-opus/s-4f2a  backoff needs jitter", "from stdin"} {
			if !strings.Contains(got.stdout, want) {
				t.Errorf("missing %q in\n%s", want, got.stdout)
			}
		}
		var doc struct {
			Notes []map[string]string `json:"notes"`
		}
		decode(t, s.tkt("show", "auth-retry-3fa", "--json").stdout, &doc)
		if len(doc.Notes) != 2 || doc.Notes[0]["body"] != "backoff needs jitter" {
			t.Errorf("notes = %v", doc.Notes)
		}
	})

	t.Run("A link verb stores the full reference of the target", func(t *testing.T) {
		s := eventSandbox(t)
		if got := s.tkt("link", "auth-retry-3fa", "implements", "9de"); got.code != 0 || got.stderr != "" {
			t.Fatalf("link: %+v", got)
		}
		got := s.tkt("unlink", "auth-retry-3fa", "implements", "gone-000")
		if got.code != 0 || !strings.Contains(got.stderr, "matches no node") {
			t.Fatalf("unlink: %+v", got)
		}
		files := eventFiles(t, s, "auth-retry-3fa")
		if len(files) != 2 {
			t.Fatalf("files = %v", files)
		}
		for _, f := range files {
			ev, _ := parse.ParseEvent(f, []byte(s.read(".tektite/events/auth-retry-3fa/"+f)))
			if !ev.Valid || ev.Fields["rel"] != "implements" {
				t.Errorf("event = %+v", ev)
			}
			if ev.Type == "link" && ev.Fields["target"] != "retry-plan-9de" {
				t.Errorf("target = %v", ev.Fields["target"])
			}
		}
	})
}

func TestCLI_Actor(t *testing.T) {
	last := func(t *testing.T, s *sandbox) string {
		files := eventFiles(t, s, "auth-retry-3fa")
		f := files[len(files)-1]
		ev, _ := parse.ParseEvent(f, []byte(s.read(".tektite/events/auth-retry-3fa/"+f)))
		return ev.Actor
	}

	t.Run("The actor comes from the environment first", func(t *testing.T) {
		s := eventSandbox(t)
		s.git = "jesus"
		s.tkt("note", "auth-retry-3fa", "a")
		if got := last(t, s); got != "agent:claude-opus/s-4f2a" {
			t.Errorf("actor = %q", got)
		}
	})

	t.Run("The actor flag wins", func(t *testing.T) {
		s := eventSandbox(t)
		s.tkt("note", "auth-retry-3fa", "a", "--actor", "agent:b")
		if got := last(t, s); got != "agent:b" {
			t.Errorf("actor = %q", got)
		}
	})

	t.Run("A human actor falls back to git config", func(t *testing.T) {
		s := eventSandbox(t)
		s.env = nil
		s.git = "jesus\n"
		s.tkt("note", "auth-retry-3fa", "a")
		if got := last(t, s); got != "human:jesus" {
			t.Errorf("actor = %q", got)
		}
	})

	t.Run("No actor available", func(t *testing.T) {
		s := eventSandbox(t)
		s.env = nil
		got := s.tkt("note", "auth-retry-3fa", "a")
		if got.code != 0 || !strings.Contains(got.stderr, "TKT-W006") || last(t, s) != "unknown" {
			t.Errorf("result = %+v actor = %q", got, last(t, s))
		}
		var doc map[string]any
		decode(t, s.tkt("note", "auth-retry-3fa", "b", "--json").stdout, &doc)
		if w, _ := doc["warnings"].([]any); len(w) != 1 {
			t.Errorf("doc = %v", doc)
		}
	})
}
