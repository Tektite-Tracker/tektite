package main

import (
	"strings"
	"testing"
	"time"
)

type revDoc struct {
	Number   int    `json:"number"`
	Event    string `json:"event"`
	Current  bool   `json:"current"`
	Diverged bool   `json:"diverged"`
	Bytes    int    `json:"bytes"`
}

type planShow struct {
	Body         string `json:"body"`
	Revision     int    `json:"revision"`
	BuiltAgainst *int   `json:"built_against"`
	History      []struct {
		Type string `json:"type"`
	} `json:"history"`
}

func planSandbox(t *testing.T) *sandbox {
	s := newSandbox(t)
	s.node("retry-plan-9de", idA, "plan", "", "", "Original plan body.")
	s.node("auth-retry-3fa", idB, "ticket", "", "implements: [retry-plan-9de]\n", "x")
	s.env = map[string]string{"TKT_ACTOR": "agent:claude-opus/s-4f2a"}
	return s
}

func (s *sandbox) revise(body string, extra ...string) result {
	s.t.Helper()
	s.now = s.now.Add(time.Minute)
	return s.tektiteIn(s.root, body, append([]string{"revise", "retry-plan-9de", "--json"}, extra...)...)
}

func (s *sandbox) revisionEvent(id, number, at, body string) {
	s.write(".tektite/events/retry-plan-9de/"+id+".md", "---\ntype: revision\nactor: human:x\nat: "+at+"\nnumber: "+number+"\n---\n"+body+"\n")
}

func (s *sandbox) revisions() []revDoc {
	s.t.Helper()
	var items []revDoc
	decode(s.t, s.tektite("revisions", "retry-plan-9de", "--json").stdout, &items)
	return items
}

func (s *sandbox) plan(ref string) planShow {
	s.t.Helper()
	var doc planShow
	decode(s.t, s.tektite("show", ref, "--json").stdout, &doc)
	return doc
}

func TestCLI_Revisions(t *testing.T) {
	t.Run("Revising a plan", func(t *testing.T) {
		s := planSandbox(t)
		res := s.revise("First revised body.\n\nSecond paragraph.")
		var doc revDoc
		decode(t, res.stdout, &doc)
		if res.code != 0 || doc.Number != 1 {
			t.Fatalf("result = %+v", res)
		}
		src := s.read(".tektite/events/retry-plan-9de/" + doc.Event + ".md")
		if !strings.Contains(src, "type: revision") || !strings.Contains(src, "number: 1") || !strings.HasSuffix(src, "First revised body.\n\nSecond paragraph.\n") {
			t.Errorf("event = %s", src)
		}
		if second := s.revise("Another body."); !strings.Contains(second.stdout, `"number": 2`) {
			t.Errorf("second = %+v", second)
		}
	})

	t.Run("The next revision follows the highest number", func(t *testing.T) {
		s := planSandbox(t)
		s.revisionEvent("01HZ3M0A1B2C3D4E5F6G7H8J91", "1", "2026-11-27T08:00:00Z", "one")
		s.revisionEvent("01HZ3M0A1B2C3D4E5F6G7H8J92", "3", "2026-11-27T09:00:00Z", "three")
		if res := s.revise("four"); !strings.Contains(res.stdout, `"number": 4`) {
			t.Errorf("result = %+v", res)
		}
	})

	t.Run("The current plan body is the latest revision", func(t *testing.T) {
		s := planSandbox(t)
		s.revise("Body one.")
		s.revise("Body two.")
		s.revise("Body three.")
		doc := s.plan("retry-plan-9de")
		if doc.Body != "Body three." || doc.Revision != 3 {
			t.Errorf("show = %+v", doc)
		}
		if out := s.tektite("show", "retry-plan-9de"); !strings.Contains(out.stdout, "revision: 3") || !strings.Contains(out.stdout, "Body three.") || strings.Contains(out.stdout, "Original plan body.") {
			t.Errorf("show = %s", out.stdout)
		}
		list := s.revisions()
		if len(list) != 3 || !list[2].Current || list[0].Current {
			t.Errorf("revisions = %+v", list)
		}
	})

	t.Run("A plan without revisions keeps its file body", func(t *testing.T) {
		s := planSandbox(t)
		if doc := s.plan("retry-plan-9de"); doc.Body != "Original plan body." || doc.Revision != 0 {
			t.Errorf("show = %+v", doc)
		}
		if out := s.tektite("revisions", "retry-plan-9de"); out.code != 0 || !strings.Contains(out.stdout, "has no revisions") {
			t.Errorf("revisions = %+v", out)
		}
	})

	t.Run("Diffing two revisions", func(t *testing.T) {
		s := planSandbox(t)
		s.revise("Line one\nLine two\nLine three")
		s.revise("Line one\nLine two\nLine three")
		s.revise("Line one\nLine 2\nLine three\nLine four")
		res := s.tektite("diff", "retry-plan-9de", "1", "3")
		want := "--- retry-plan-9de revision 1\n+++ retry-plan-9de revision 3\n@@ -1,3 +1,4 @@\n Line one\n-Line two\n+Line 2\n Line three\n+Line four\n"
		if res.code != 0 || res.stdout != want {
			t.Errorf("diff = %q", res.stdout)
		}
		if same := s.tektite("diff", "retry-plan-9de", "1", "2"); same.code != 0 || same.stdout != "" {
			t.Errorf("same = %+v", same)
		}
		if missing := s.tektite("diff", "retry-plan-9de", "1", "9"); missing.code != exitNotFound {
			t.Errorf("missing = %+v", missing)
		}
		if bad := s.tektite("diff", "retry-plan-9de", "one", "3"); bad.code != exitUsage {
			t.Errorf("bad = %+v", bad)
		}
	})

	t.Run("Concurrent revisions both survive", func(t *testing.T) {
		s := planSandbox(t)
		s.revisionEvent("01HZ3M0A1B2C3D4E5F6G7H8J91", "2", "2026-11-27T08:00:00Z", "branch a")
		s.revisionEvent("01HZ3M0A1B2C3D4E5F6G7H8J92", "2", "2026-11-27T08:00:00Z", "branch b")
		doc := s.plan("retry-plan-9de")
		revs := 0
		for _, h := range doc.History {
			if h.Type == "revision" {
				revs++
			}
		}
		if revs != 2 || doc.Body != "branch b" || doc.Revision != 2 {
			t.Errorf("show = %+v", doc)
		}
		list := s.revisions()
		if len(list) != 2 || !list[0].Diverged || !list[1].Diverged {
			t.Errorf("revisions = %+v", list)
		}
		out := s.tektite("check")
		if out.code != 0 || strings.Count(out.stdout, "TKT-W013") != 1 || !strings.Contains(out.stdout, "diverged") {
			t.Errorf("check = %+v", out)
		}
	})

	t.Run("Work records which revision it was built against", func(t *testing.T) {
		s := planSandbox(t)
		s.revise("one")
		s.revise("two")
		s.now = s.now.Add(time.Minute)
		if res := s.tektite("claim", "auth-retry-3fa", "--revision", "2"); res.code != 0 {
			t.Fatalf("claim = %+v", res)
		}
		if doc := s.plan("auth-retry-3fa"); doc.BuiltAgainst == nil || *doc.BuiltAgainst != 2 {
			t.Errorf("show = %+v", doc)
		}
		if out := s.tektite("check"); strings.Contains(out.stdout, "TKT-N104") {
			t.Errorf("up to date work flagged: %s", out.stdout)
		}
		s.revise("three")
		s.revise("four")
		out := s.tektite("check")
		if out.code != 0 || !strings.Contains(out.stdout, "note TKT-N104") || !strings.Contains(out.stdout, "built against revision 2 of retry-plan-9de, which is now at revision 4") {
			t.Errorf("check = %+v", out)
		}
		s.now = s.now.Add(time.Minute)
		if res := s.tektite("set-status", "auth-retry-3fa", "review", "--revision", "4"); res.code != 0 {
			t.Fatalf("set-status = %+v", res)
		}
		if out := s.tektite("check"); strings.Contains(out.stdout, "TKT-N104") {
			t.Errorf("latest revision recorded but still flagged: %s", out.stdout)
		}
		if text := s.tektite("show", "auth-retry-3fa"); !strings.Contains(text.stdout, "built against revision: 4") {
			t.Errorf("show = %s", text.stdout)
		}
	})

	t.Run("A revision flag must be positive", func(t *testing.T) {
		s := planSandbox(t)
		if res := s.tektite("claim", "auth-retry-3fa", "--revision", "0"); res.code != exitUsage {
			t.Errorf("claim = %+v", res)
		}
		if res := s.tektite("set-status", "auth-retry-3fa", "done", "--revision", "-1"); res.code != exitUsage {
			t.Errorf("set-status = %+v", res)
		}
	})

	t.Run("Revising needs a body", func(t *testing.T) {
		s := planSandbox(t)
		if res := s.revise("  \n"); res.code != exitUsage {
			t.Errorf("empty = %+v", res)
		}
		if res := s.tektiteIn(s.root, "x", "revise", "ghost-000"); res.code != exitNotFound {
			t.Errorf("unknown = %+v", res)
		}
		s.write("body.md", "From a file.\n")
		if res := s.tektite("revise", "retry-plan-9de", "--file", "body.md"); res.code != 0 {
			t.Errorf("file = %+v", res)
		}
		if doc := s.plan("retry-plan-9de"); doc.Body != "From a file." {
			t.Errorf("show = %+v", doc)
		}
	})

	t.Run("An invalid revision number is ignored with a warning", func(t *testing.T) {
		s := planSandbox(t)
		s.revisionEvent("01HZ3M0A1B2C3D4E5F6G7H8J91", "zero", "2026-11-27T08:00:00Z", "bad")
		out := s.tektite("check")
		if !strings.Contains(out.stdout, "TKT-W012") || s.plan("retry-plan-9de").Revision != 0 {
			t.Errorf("check = %+v", out)
		}
	})
}
