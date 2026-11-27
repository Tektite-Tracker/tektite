package main

import (
	"strings"
	"testing"
)

type ctxEntry struct {
	Ref      string `json:"ref"`
	Mode     string `json:"mode"`
	Tokens   int    `json:"tokens"`
	Revision int    `json:"revision"`
	Block    string `json:"block"`
	Stale    bool   `json:"stale"`
	Text     string `json:"text"`
}

type ctxDoc struct {
	Budget    int        `json:"budget"`
	Tokens    int        `json:"tokens"`
	Node      []ctxEntry `json:"node"`
	Ancestors []ctxEntry `json:"ancestors"`
	Gaps      []ctxEntry `json:"gaps"`
	Evidence  []ctxEntry `json:"evidence"`
	Links     []ctxEntry `json:"links"`
	History   []ctxEntry `json:"history"`
	Shortened []string   `json:"shortened"`
	Omitted   []struct {
		Ref   string `json:"ref"`
		Fetch string `json:"fetch"`
	} `json:"omitted"`
}

func para(word string, n int) string {
	return strings.TrimSpace(strings.Repeat(word+" ", n))
}

func contextSandbox(t *testing.T, ticketBody string) *sandbox {
	s := newSandbox(t)
	s.node("auth-spec-1aa", "01HZ3K8Q5W2X9N4T7V6B1C0D01", "spec", "", "", "Spec intro paragraph.\n\n"+para("spec", 400))
	s.node("retry-plan-9de", "01HZ3K8Q5W2X9N4T7V6B1C0D02", "plan", "auth-spec-1aa", "", "Plan intro paragraph.\n\n"+para("plan", 400))
	s.node("auth-retry-3fa", "01HZ3K8Q5W2X9N4T7V6B1C0D03", "ticket", "retry-plan-9de", "blocks: [other-5ab]\n", ticketBody)
	s.node("other-5ab", "01HZ3K8Q5W2X9N4T7V6B1C0D04", "ticket", "", "", "Other intro.\n\n"+para("other", 300))
	s.event("auth-retry-3fa", "01HZ3M0A1B2C3D4E5F6G7H8J91", "note", "2026-11-27T09:00:00Z", "")
	return s
}

func (s *sandbox) context(args ...string) (ctxDoc, result) {
	s.t.Helper()
	res := s.tkt(append([]string{"context", "--json"}, args...)...)
	var doc ctxDoc
	if res.code == 0 {
		decode(s.t, res.stdout, &doc)
	}
	return doc, res
}

func TestCLI_Context(t *testing.T) {
	t.Run("The node is always included", func(t *testing.T) {
		s := contextSandbox(t, "Ticket body text.")
		doc, res := s.context("auth-retry-3fa", "--budget", "200")
		if res.code != 0 || len(doc.Node) != 1 || !strings.Contains(doc.Node[0].Text, "Ticket body text.") || doc.Node[0].Mode != "full" {
			t.Errorf("result = %+v", res)
		}
	})

	t.Run("Tiers are filled in order", func(t *testing.T) {
		s := contextSandbox(t, "Ticket body.\n\n- [ ] first gap <!-- g:01HZ3KB2 -->\n")
		res := s.tkt("context", "auth-retry-3fa")
		if res.code != 0 {
			t.Fatalf("result = %+v", res)
		}
		last := -1
		for _, h := range []string{"## Node", "## Ancestor specs and plans", "## Open gaps", "## Recent evidence", "## Linked nodes", "## Recent history"} {
			i := strings.Index(res.stdout, h)
			if i <= last {
				t.Fatalf("section %q out of order in\n%s", h, res.stdout)
			}
			last = i
		}
		if strings.Contains(res.stdout, "## Omitted") {
			t.Errorf("nothing should be omitted:\n%s", res.stdout)
		}
	})

	t.Run("The current plan revision is used", func(t *testing.T) {
		s := contextSandbox(t, "Ticket body.")
		s.event("retry-plan-9de", "01HZ3M0A1B2C3D4E5F6G7H8J91", "revision", "2026-11-27T08:00:00Z", "number: 1\n")
		s.write(".tektite/events/retry-plan-9de/01HZ3M0A1B2C3D4E5F6G7H8J93.md", "---\ntype: revision\nactor: human:x\nat: 2026-11-27T09:30:00Z\nnumber: 3\n---\nRevision three body.\n")
		doc, _ := s.context("auth-retry-3fa")
		var plan ctxEntry
		for _, e := range doc.Ancestors {
			if e.Ref == "retry-plan-9de" {
				plan = e
			}
		}
		if plan.Revision != 3 || !strings.Contains(plan.Text, "Revision three body.") {
			t.Errorf("plan = %+v", plan)
		}
		if md := s.tkt("context", "auth-retry-3fa"); !strings.Contains(md.stdout, "revision 3") {
			t.Errorf("markdown = %s", md.stdout)
		}
	})

	t.Run("Bodies shrink to summaries when over budget", func(t *testing.T) {
		s := contextSandbox(t, "Ticket body.")
		doc, _ := s.context("auth-retry-3fa", "--budget", "300")
		if len(doc.Ancestors) == 0 {
			t.Fatalf("doc = %+v", doc)
		}
		for _, e := range doc.Ancestors {
			if e.Mode != "summary" || strings.Contains(e.Text, "spec spec spec spec") {
				t.Errorf("entry = %+v", e)
			}
		}
		if len(doc.Shortened) == 0 {
			t.Errorf("shortened = %v", doc.Shortened)
		}
		if md := s.tkt("context", "auth-retry-3fa", "--budget", "300"); !strings.Contains(md.stdout, "Shortened: ") {
			t.Errorf("markdown = %s", md.stdout)
		}
	})

	t.Run("Lower tiers are dropped before higher ones", func(t *testing.T) {
		s := contextSandbox(t, "Ticket body.")
		doc, _ := s.context("auth-retry-3fa", "--budget", "10")
		if len(doc.Node) != 1 || len(doc.Links) != 0 || len(doc.History) != 0 {
			t.Errorf("doc = %+v", doc)
		}
	})

	t.Run("The bundle reports what it left out", func(t *testing.T) {
		s := contextSandbox(t, "Ticket body.")
		doc, _ := s.context("auth-retry-3fa", "--budget", "10")
		if len(doc.Omitted) == 0 || !strings.HasPrefix(doc.Omitted[0].Fetch, "tkt show ") {
			t.Errorf("omitted = %+v", doc.Omitted)
		}
		md := s.tkt("context", "auth-retry-3fa", "--budget", "10").stdout
		tail := md[strings.Index(md, "## Omitted"):]
		if !strings.Contains(tail, "`tkt show auth-spec-1aa`") || strings.Contains(tail, "## Node") {
			t.Errorf("tail = %s", tail)
		}
	})

	t.Run("A node that alone exceeds the budget is truncated with a marker", func(t *testing.T) {
		s := contextSandbox(t, "First paragraph.\n\n"+para("word", 300)+"\n\n"+para("tail", 300))
		doc, _ := s.context("auth-retry-3fa", "--budget", "100")
		text := doc.Node[0].Text
		if doc.Node[0].Mode != "truncated" || !strings.Contains(text, "[body truncated") || !strings.HasPrefix(text, "First paragraph.\n\n[body truncated") {
			t.Errorf("node = %+v", doc.Node[0])
		}
	})

	t.Run("The default budget comes from configuration", func(t *testing.T) {
		s := contextSandbox(t, "Ticket body.")
		s.write(".tektite/config", "auto_commit: false\ncontext_budget: 8000\n")
		if doc, _ := s.context("auth-retry-3fa"); doc.Budget != 8000 {
			t.Errorf("budget = %d", doc.Budget)
		}
		s.write(".tektite/config", "context_budget: 500\n")
		if doc, _ := s.context("auth-retry-3fa"); doc.Budget != 500 {
			t.Errorf("budget = %d", doc.Budget)
		}
		if doc, _ := s.context("auth-retry-3fa", "--budget", "900"); doc.Budget != 900 {
			t.Errorf("budget = %d", doc.Budget)
		}
	})

	t.Run("Only open gaps are listed", func(t *testing.T) {
		body := "Body.\n\n## Gaps\n\n- [ ] first open <!-- g:01HZ3KB1 -->\n- [ ] second open <!-- g:01HZ3KB2 -->\n- [x] closed one <!-- g:01HZ3KB3 -->\n"
		s := contextSandbox(t, body)
		doc, _ := s.context("auth-retry-3fa")
		if len(doc.Gaps) != 2 || doc.Gaps[0].Block != "01HZ3KB1" || doc.Gaps[1].Block != "01HZ3KB2" {
			t.Errorf("gaps = %+v", doc.Gaps)
		}
	})

	t.Run("Evidence shows claim and provenance but not binary content", func(t *testing.T) {
		s := contextSandbox(t, "Body.")
		s.node("retry-banner-shot-5de", "01HZ3N5R8T2V4X6Z9B1D3F5H7J", "evidence", "auth-retry-3fa",
			"evidences: [auth-retry-3fa]\nclaim: The banner is shown\nattachments:\n  - blobs/9f86d081.png\nprovenance:\n  at: 2026-11-27T10:40:00Z\n  commit: 3f2a9c1\n", "Screenshot after 429.")
		doc, _ := s.context("auth-retry-3fa")
		if len(doc.Evidence) != 1 {
			t.Fatalf("evidence = %+v", doc.Evidence)
		}
		for _, want := range []string{"Claim: The banner is shown", "Commit: 3f2a9c1", "Attachment: blobs/9f86d081.png"} {
			if !strings.Contains(doc.Evidence[0].Text, want) {
				t.Errorf("missing %q in %s", want, doc.Evidence[0].Text)
			}
		}
	})

	t.Run("Stale evidence is flagged", func(t *testing.T) {
		s := contextSandbox(t, "Body.")
		s.node("retry-banner-shot-5de", "01HZ3N5R8T2V4X6Z9B1D3F5H7J", "evidence", "auth-retry-3fa",
			"claim: c\nprovenance:\n  at: 2026-11-27T10:40:00Z\n  commit: 3f2a9c1\n", "x")
		s.head = "aaaaaaa1111111"
		doc, _ := s.context("auth-retry-3fa")
		if len(doc.Evidence) != 1 || !doc.Evidence[0].Stale || !strings.Contains(doc.Evidence[0].Text, "Possibly stale") {
			t.Errorf("evidence = %+v", doc.Evidence)
		}
		s.head = "3f2a9c1deadbeef"
		if doc, _ := s.context("auth-retry-3fa"); doc.Evidence[0].Stale {
			t.Errorf("matching commit flagged stale")
		}
	})

	t.Run("JSON output", func(t *testing.T) {
		s := contextSandbox(t, "Body.")
		doc, _ := s.context("auth-retry-3fa")
		if len(doc.Node) != 1 || doc.Node[0].Ref != "auth-retry-3fa" || doc.Node[0].Tokens == 0 || len(doc.Ancestors) != 2 || len(doc.Links) != 1 || len(doc.History) != 1 {
			t.Errorf("doc = %+v", doc)
		}
	})

	t.Run("Markdown output is the default", func(t *testing.T) {
		s := contextSandbox(t, "Body.")
		res := s.tkt("context", "auth-retry-3fa")
		if res.code != 0 || strings.Count(res.stdout, "\n## ")+strings.Count(res.stdout[:3], "## ") != 6 {
			t.Errorf("markdown = %s", res.stdout)
		}
	})

	t.Run("Token counts are estimates", func(t *testing.T) {
		s := contextSandbox(t, "Body.")
		doc, _ := s.context("auth-retry-3fa")
		if doc.Node[0].Tokens != (len(doc.Node[0].Text)+3)/4 {
			t.Errorf("tokens = %d for %d bytes", doc.Node[0].Tokens, len(doc.Node[0].Text))
		}
	})

	t.Run("Unknown nodes fail clearly", func(t *testing.T) {
		s := contextSandbox(t, "Body.")
		if res := s.tkt("context", "ghost-000"); res.code != exitNotFound {
			t.Errorf("result = %+v", res)
		}
	})

	t.Run("Output is deterministic", func(t *testing.T) {
		s := contextSandbox(t, "Body.")
		a := s.tkt("context", "auth-retry-3fa", "--budget", "300").stdout
		b := s.tkt("context", "auth-retry-3fa", "--budget", "300").stdout
		if a != b {
			t.Errorf("outputs differ")
		}
	})
}
