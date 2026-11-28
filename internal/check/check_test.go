package check_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tektite-tracker/tektite/internal/check"
	"github.com/tektite-tracker/tektite/internal/store"
	"github.com/tektite-tracker/tektite/parse"
)

const (
	idA = "01HZ3K8Q5W2X9N4T7V6B1C0D3F"
	idB = "01HZ3K8Q5W2X9N4T7V6B1C0D9E"
	idC = "01HZ3K8Q5W2X9N4T7V6B1C0D7C"
	e1  = "01HZ3M0A1B2C3D4E5F6G7H8J91"
	e2  = "01HZ3M0A1B2C3D4E5F6G7H8J92"
)

type fixture struct {
	t    *testing.T
	root string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".tektite"), 0o755); err != nil {
		t.Fatal(err)
	}
	return &fixture{t: t, root: root}
}

func (f *fixture) write(rel, content string) *fixture {
	f.t.Helper()
	path := filepath.Join(f.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
	return f
}

func (f *fixture) node(ref, id, kind, parent, extra, body string) *fixture {
	return f.write(".tektite/nodes/"+ref+".md", "---\nid: "+id+"\nkind: "+kind+"\ntitle: "+ref+"\nparent: "+parent+"\n"+extra+"---\n"+body+"\n")
}

func (f *fixture) event(ref, id, content string) *fixture {
	return f.write(".tektite/events/"+ref+"/"+id+".md", content)
}

func (f *fixture) run() check.Report {
	f.t.Helper()
	r, err := store.Open(f.root)
	if err != nil {
		f.t.Fatal(err)
	}
	rep, err := check.Run(r)
	if err != nil {
		f.t.Fatal(err)
	}
	return rep
}

func find(rep check.Report, code string) []check.Problem {
	out := []check.Problem{}
	for _, p := range rep.Problems {
		if p.Code == code {
			out = append(out, p)
		}
	}
	return out
}

func only(t *testing.T, rep check.Report, code string) check.Problem {
	t.Helper()
	got := find(rep, code)
	if len(got) != 1 {
		t.Fatalf("%s: want one problem, got %+v in %+v", code, got, rep.Problems)
	}
	return got[0]
}

func statusEvent(status string) string {
	return "---\ntype: set-status\nactor: human:jesus\nat: 2026-11-27T10:00:00Z\nstatus: " + status + "\n---\n"
}

func TestCheck_Nodes(t *testing.T) {
	t.Run("A clean repository has no problems", func(t *testing.T) {
		rep := newFixture(t).node("plan-d3f", idA, "plan", "", "", "p").node("tk-9e1", idB, "ticket", "plan-d3f", "", "see [[plan-d3f]]").run()
		if len(rep.Problems) != 0 || rep.HasErrors() {
			t.Errorf("problems = %+v", rep.Problems)
		}
	})

	t.Run("A missing id or kind on an imported note is a note", func(t *testing.T) {
		rep := newFixture(t).write(".tektite/nodes/plain-3fa.md", "# Heading\ntext\n").write(".tektite/nodes/partial-9e1.md", "---\ntags: [a]\n---\n# Heading\n").run()
		if rep.Errors != 0 || rep.Warnings != 1 || rep.Notes != 3 || rep.HasErrors() {
			t.Errorf("report = %+v", rep)
		}
		for _, p := range rep.Problems {
			if (p.Code == parse.CodeMissingID || p.Code == parse.CodeMissingKind) && p.Severity != parse.SeverityNote {
				t.Errorf("problem = %+v", p)
			}
		}
		if p := only(t, rep, parse.CodeMissingTitle); p.Severity != parse.SeverityWarning {
			t.Errorf("problem = %+v", p)
		}
	})

	t.Run("A root node has no parent", func(t *testing.T) {
		rep := newFixture(t).node("spec-d3f", idA, "spec", "", "", "x").run()
		if len(rep.Problems) != 0 {
			t.Errorf("problems = %+v", rep.Problems)
		}
	})

	t.Run("A priority outside critical, high, medium and low is a warning", func(t *testing.T) {
		rep := newFixture(t).
			node("bad-3fa", idA, "ticket", "", "priority: urgent\n", "x").
			node("up-9e1", idB, "ticket", "", "priority: HIGH\n", "x").
			node("low-7c1", idC, "ticket", "", "priority: low\n", "x").
			run()
		p := only(t, rep, check.CodeInvalidPriority)
		if p.Severity != parse.SeverityWarning || !strings.Contains(p.Message, "bad-3fa") || !strings.Contains(p.Message, "critical, high, medium, low") {
			t.Errorf("problem = %+v", p)
		}
	})

	t.Run("An unknown kind is accepted", func(t *testing.T) {
		rep := newFixture(t).node("retro-d3f", idA, "retro", "", "", "x").run()
		if len(rep.Problems) != 0 {
			t.Errorf("problems = %+v", rep.Problems)
		}
	})

	t.Run("Custom kinds need no schema", func(t *testing.T) {
		rep := newFixture(t).node("retro-d3f", idA, "retro", "", "", "x").run()
		if len(rep.Problems) != 0 {
			t.Errorf("problems = %+v", rep.Problems)
		}
	})

	t.Run("A status field in frontmatter is ignored with a warning", func(t *testing.T) {
		rep := newFixture(t).node("spec-d3f", idA, "spec", "", "status: done\n", "x").run()
		p := only(t, rep, parse.CodeStatusInFrontmatter)
		if p.Severity != parse.SeverityWarning || p.Path != ".tektite/nodes/spec-d3f.md" || p.Hint == "" {
			t.Errorf("problem = %+v", p)
		}
		if len(find(rep, check.CodeDisallowedStatus)) != 0 {
			t.Error("computed status stays open")
		}
	})

	t.Run("Malformed frontmatter does not block reads", func(t *testing.T) {
		rep := newFixture(t).write(".tektite/nodes/bad-3fa.md", "---\ntitle: [unclosed\n---\nbody\n").run()
		p := find(rep, parse.CodeMalformedFrontmatter)
		if len(p) != 1 || p[0].Line == 0 || p[0].Severity != parse.SeverityError || !rep.HasErrors() {
			t.Errorf("problems = %+v", rep.Problems)
		}
	})

	t.Run("Duplicate ids are reported and both nodes stay readable", func(t *testing.T) {
		rep := newFixture(t).node("one-d3f", idA, "spec", "", "", "x").node("two-9e1", idA, "spec", "", "", "y").run()
		got := find(rep, check.CodeDuplicateID)
		if len(got) != 2 || got[0].Severity != parse.SeverityError || !rep.HasErrors() {
			t.Fatalf("problems = %+v", rep.Problems)
		}
		if !strings.Contains(got[0].Message, ".tektite/nodes/two-9e1.md") || !strings.Contains(got[1].Message, ".tektite/nodes/one-d3f.md") {
			t.Errorf("messages = %q / %q", got[0].Message, got[1].Message)
		}
	})

	t.Run("A cycle in parents is reported and not followed", func(t *testing.T) {
		rep := newFixture(t).node("a-d3f", idA, "plan", "b-9e1", "", "x").node("b-9e1", idB, "plan", "a-d3f", "", "y").run()
		p := only(t, rep, check.CodeParentCycle)
		if p.Severity != parse.SeverityError || p.Code != "TKT-E106" || !strings.Contains(p.Message, "a-d3f -> b-9e1 -> a-d3f") || !rep.HasErrors() {
			t.Errorf("problem = %+v", p)
		}
	})

	t.Run("A parent that matches no node is a warning", func(t *testing.T) {
		rep := newFixture(t).node("tk-d3f", idA, "ticket", "ghost-000", "", "x").run()
		p := only(t, rep, check.CodeBrokenParent)
		if p.Severity != parse.SeverityWarning || !strings.Contains(p.Message, "ghost-000") {
			t.Errorf("problem = %+v", p)
		}
	})
}

func TestCheck_Links(t *testing.T) {
	t.Run("A link to a missing node", func(t *testing.T) {
		rep := newFixture(t).node("tk-d3f", idA, "ticket", "", "blocks: [ghost-000]\n", "x").run()
		p := only(t, rep, check.CodeBrokenLink)
		if p.Severity != parse.SeverityWarning || !strings.Contains(p.Message, "tk-d3f") || !strings.Contains(p.Message, "blocks") || !strings.Contains(p.Message, "ghost-000") {
			t.Errorf("problem = %+v", p)
		}
		if rep.HasErrors() {
			t.Error("a broken link is not an error")
		}
	})

	t.Run("A broken wikilink is a warning, not an error", func(t *testing.T) {
		rep := newFixture(t).node("tk-d3f", idA, "ticket", "", "", "see [[does-not-exist-000]]").run()
		p := only(t, rep, check.CodeBrokenLink)
		if p.Severity != parse.SeverityWarning || p.Line == 0 || !strings.Contains(p.Message, "mentions") || rep.HasErrors() {
			t.Errorf("problem = %+v", p)
		}
	})

	t.Run("A link to oneself", func(t *testing.T) {
		rep := newFixture(t).node("tk-d3f", idA, "ticket", "", "implements: [tk-d3f]\n", "x").run()
		p := only(t, rep, check.CodeSelfLink)
		if p.Severity != parse.SeverityWarning {
			t.Errorf("problem = %+v", p)
		}
	})

	t.Run("Link targets accept any unambiguous reference", func(t *testing.T) {
		rep := newFixture(t).node("auth-retry-3fa", idA, "ticket", "", "", "x").node("other-9e1", idB, "ticket", "", "implements: [3fa]\n", "x").run()
		if len(rep.Problems) != 0 {
			t.Errorf("problems = %+v", rep.Problems)
		}
	})

	t.Run("A reference resolves by its hash suffix even if the slug part is stale", func(t *testing.T) {
		rep := newFixture(t).node("add-retry-on-429-3fa", idA, "ticket", "", "", "x").node("other-9e1", idB, "ticket", "", "", "[[retry-ticket-3fa]]").run()
		p := only(t, rep, check.CodeStaleSlug)
		if p.Severity != parse.SeverityNote || !strings.Contains(p.Message, "add-retry-on-429-3fa") || len(find(rep, check.CodeBrokenLink)) != 0 {
			t.Errorf("problems = %+v", rep.Problems)
		}
	})

	t.Run("Wikilinks by file name, title or alias are not resolved", func(t *testing.T) {
		f := newFixture(t).write(".tektite/nodes/auth-retry-3fa.md", "---\nid: "+idA+"\nkind: ticket\ntitle: Auth Retry\nparent: \"\"\naliases: [shortname]\n---\nx\n").
			node("other-9e1", idB, "ticket", "", "", "[[Auth Retry]] [[auth-retry]] [[shortname]] [[auth-retry.md]] [[auth-retry-3fa]]")
		got := find(f.run(), check.CodeBrokenLink)
		if len(got) != 4 {
			t.Fatalf("want four broken links, got %+v", got)
		}
		for _, p := range got {
			if strings.Contains(p.Message, `"auth-retry-3fa"`) {
				t.Errorf("the real ref must resolve: %+v", p)
			}
		}
	})

	t.Run("A wikilink in a frontmatter string value is a link", func(t *testing.T) {
		f := newFixture(t).node("tk-d3f", idA, "ticket", "", "related: \"[[ghost-000]] and [[tk-d3f]]\"\n", "x")
		rep := f.run()
		broken := only(t, rep, check.CodeBrokenLink)
		self := only(t, rep, check.CodeSelfLink)
		if broken.Line != 6 || !strings.Contains(broken.Message, "ghost-000") || self.Line != 6 {
			t.Errorf("broken=%+v self=%+v", broken, self)
		}
	})

	t.Run("A link added by event is checked like any other", func(t *testing.T) {
		f := newFixture(t).node("tk-d3f", idA, "ticket", "", "", "x")
		f.event("tk-d3f", e1, "---\ntype: link\nactor: human:jesus\nat: 2026-11-27T10:00:00Z\nrel: blocks\ntarget: ghost-000\n---\n")
		p := only(t, f.run(), check.CodeBrokenLink)
		if !strings.Contains(p.Message, "blocks") {
			t.Errorf("problem = %+v", p)
		}
	})

	t.Run("An ambiguous or too short link target is a warning", func(t *testing.T) {
		rep := newFixture(t).node("a-a3fa", idA, "ticket", "", "", "x").node("b-b3fa", idB, "ticket", "", "", "x").
			node("c-7c1", idC, "ticket", "", "blocks: [fa, 3fa]\n", "x").run()
		short := only(t, rep, check.CodeShortRef)
		amb := only(t, rep, check.CodeAmbiguousRef)
		if !strings.Contains(short.Message, `"fa"`) || !strings.Contains(amb.Message, "a-a3fa") || !strings.Contains(amb.Message, "b-b3fa") || rep.HasErrors() {
			t.Errorf("problems = %+v", rep.Problems)
		}
	})
}

func TestCheck_Events(t *testing.T) {
	t.Run("An event for a missing node is kept and reported", func(t *testing.T) {
		f := newFixture(t).node("tk-d3f", idA, "ticket", "", "", "x")
		f.event("ghost-000", e1, statusEvent("done"))
		rep := f.run()
		p := only(t, rep, check.CodeOrphanEvents)
		if p.Severity != parse.SeverityWarning || p.Path != ".tektite/events/ghost-000" || len(rep.Problems) != 1 {
			t.Errorf("problems = %+v", rep.Problems)
		}
	})

	t.Run("Concurrent status changes resolve by time then ULID", func(t *testing.T) {
		f := newFixture(t).node("tk-d3f", idA, "ticket", "", "", "x")
		f.event("tk-d3f", e1, statusEvent("active"))
		f.event("tk-d3f", e2, "---\ntype: set-status\nactor: agent:other\nat: 2026-11-27T10:00:00Z\nstatus: review\n---\n")
		rep := f.run()
		p := only(t, rep, parse.CodeConcurrentStatus)
		if p.Severity != parse.SeverityNote || !strings.Contains(p.Message, e2) || rep.HasErrors() {
			t.Errorf("problem = %+v", p)
		}
	})

	t.Run("A malformed event is skipped in the fold", func(t *testing.T) {
		f := newFixture(t).node("tk-d3f", idA, "ticket", "", "", "x")
		f.event("tk-d3f", e1, "---\ntype: [unclosed\n---\n")
		rep := f.run()
		p := only(t, rep, parse.CodeMalformedFrontmatter)
		if p.Path != ".tektite/events/tk-d3f/"+e1+".md" || !rep.HasErrors() {
			t.Errorf("problem = %+v", p)
		}
		if len(find(rep, parse.CodeEventSkipped)) != 0 {
			t.Error("the skip note duplicates the file error")
		}
	})

	t.Run("An unknown status is accepted with a warning", func(t *testing.T) {
		f := newFixture(t).node("tk-d3f", idA, "ticket", "", "", "x")
		f.event("tk-d3f", e1, statusEvent("parked"))
		rep := f.run()
		p := only(t, rep, parse.CodeUnknownStatus)
		if p.Severity != parse.SeverityWarning || !strings.Contains(p.Message, "parked") || !strings.Contains(p.Message, e1) {
			t.Errorf("problem = %+v", p)
		}
	})

	t.Run("A missing actor is repaired from the environment", func(t *testing.T) {
		f := newFixture(t).node("tk-d3f", idA, "ticket", "", "", "x")
		f.event("tk-d3f", e1, "---\ntype: note\nat: 2026-11-27T10:00:00Z\n---\nhi\n")
		rep := f.run()
		p := only(t, rep, parse.CodeMissingActor)
		if p.Severity != parse.SeverityWarning || rep.HasErrors() {
			t.Errorf("problem = %+v", p)
		}
	})

	t.Run("Duplicate event ids are a warning and both events are kept", func(t *testing.T) {
		f := newFixture(t).node("tk-d3f", idA, "ticket", "", "", "x")
		f.event("tk-d3f", e1, statusEvent("active"))
		f.event("old-slug-d3f", e1, statusEvent("done"))
		rep := f.run()
		if len(find(rep, parse.CodeDuplicateEventID)) != 1 {
			t.Errorf("problems = %+v", rep.Problems)
		}
	})
}

func TestCheck_Schemas(t *testing.T) {
	const ticketRequiresEstimate = "---\nkind: ticket\nrequired: [estimate]\n---\n"
	const gapStatuses = "---\nkind: gap\nstatuses: [open, done]\n---\n"
	const ticketParents = "---\nkind: ticket\nparent_kinds: [plan]\n---\n"

	t.Run("A schema in .tektite/ can constrain a kind", func(t *testing.T) {
		f := newFixture(t).write(".tektite/schemas/ticket.md", ticketRequiresEstimate).node("tk-d3f", idA, "ticket", "", "", "x")
		p := only(t, f.run(), check.CodeMissingRequired)
		if !strings.Contains(p.Message, "tk-d3f") || !strings.Contains(p.Message, "estimate") {
			t.Errorf("problem = %+v", p)
		}
	})

	t.Run("A missing required field", func(t *testing.T) {
		f := newFixture(t).write(".tektite/schemas/ticket.md", ticketRequiresEstimate).
			node("tk-d3f", idA, "ticket", "", "", "x").node("ok-9e1", idB, "ticket", "", "estimate: 3\n", "x").node("empty-7c1", idC, "ticket", "", "estimate: \"\"\n", "x")
		got := find(f.run(), check.CodeMissingRequired)
		if len(got) != 2 || got[0].Path != ".tektite/nodes/empty-7c1.md" || got[1].Path != ".tektite/nodes/tk-d3f.md" {
			t.Errorf("problems = %+v", got)
		}
	})

	t.Run("A disallowed status", func(t *testing.T) {
		f := newFixture(t).write(".tektite/schemas/gap.md", gapStatuses).node("gap-d3f", idA, "gap", "", "", "x")
		f.event("gap-d3f", e1, statusEvent("review"))
		p := only(t, f.run(), check.CodeDisallowedStatus)
		if !strings.Contains(p.Message, "gap-d3f") || !strings.Contains(p.Message, "review") {
			t.Errorf("problem = %+v", p)
		}
	})

	t.Run("A kind schema can restrict statuses", func(t *testing.T) {
		f := newFixture(t).write(".tektite/schemas/gap.md", gapStatuses).node("gap-d3f", idA, "gap", "", "", "x")
		f.event("gap-d3f", e1, statusEvent("done"))
		f.event("gap-d3f", e2, statusEvent("review"))
		rep := f.run()
		p := only(t, rep, check.CodeDisallowedStatus)
		if !strings.Contains(p.Message, "review") || !strings.Contains(p.Message, e2) || strings.Contains(p.Message, e1) {
			t.Errorf("problem = %+v", p)
		}
	})

	t.Run("A disallowed parent kind", func(t *testing.T) {
		f := newFixture(t).write(".tektite/schemas/ticket.md", ticketParents).node("spec-d3f", idA, "spec", "", "", "x").node("tk-9e1", idB, "ticket", "spec-d3f", "", "x")
		p := only(t, f.run(), check.CodeDisallowedParent)
		if !strings.Contains(p.Message, "tk-9e1") || !strings.Contains(p.Message, "ticket") || !strings.Contains(p.Message, "spec") {
			t.Errorf("problem = %+v", p)
		}
	})

	t.Run("A root node ignores parent_kinds", func(t *testing.T) {
		f := newFixture(t).write(".tektite/schemas/ticket.md", ticketParents).node("tk-d3f", idA, "ticket", "", "", "x")
		if rep := f.run(); len(rep.Problems) != 0 {
			t.Errorf("problems = %+v", rep.Problems)
		}
	})

	t.Run("Schema warnings are not errors", func(t *testing.T) {
		f := newFixture(t).write(".tektite/schemas/ticket.md", "---\nkind: ticket\nrequired: [estimate]\nparent_kinds: [plan]\n---\n").
			node("spec-d3f", idA, "spec", "", "", "x").node("tk-9e1", idB, "ticket", "spec-d3f", "", "x")
		rep := f.run()
		if rep.Warnings != 2 || rep.HasErrors() {
			t.Errorf("report = %+v", rep)
		}
	})

	t.Run("A malformed schema is reported and ignored", func(t *testing.T) {
		f := newFixture(t).write(".tektite/schemas/ticket.md", "---\nkind: [unclosed\n---\n").node("tk-d3f", idA, "ticket", "", "", "x")
		rep := f.run()
		p := only(t, rep, store.CodeSchemaMalformed)
		if p.Path != ".tektite/schemas/ticket.md" || p.Line == 0 || !rep.HasErrors() {
			t.Errorf("problem = %+v", p)
		}
	})

	t.Run("The file name must match the kind", func(t *testing.T) {
		f := newFixture(t).write(".tektite/schemas/ticket.md", "---\nkind: plan\nrequired: [owner]\n---\n").node("plan-d3f", idA, "plan", "", "", "x")
		rep := f.run()
		only(t, rep, store.CodeSchemaKindMismatch)
		only(t, rep, check.CodeMissingRequired)
	})
}

func TestCheck_Report(t *testing.T) {
	t.Run("Problems are ordered and counted", func(t *testing.T) {
		f := newFixture(t).node("b-9e1", idB, "ticket", "ghost-000", "", "x").node("a-d3f", idA, "spec", "", "status: done\n", "x").
			write(".tektite/nodes/c-7c1.md", "---\ntitle: [x\n---\n")
		rep := f.run()
		if rep.Errors != 1 || rep.Warnings < 2 || rep.Errors+rep.Warnings+rep.Notes != len(rep.Problems) {
			t.Errorf("report = %+v", rep)
		}
		for i := 1; i < len(rep.Problems); i++ {
			if rep.Problems[i-1].Path > rep.Problems[i].Path {
				t.Errorf("not sorted by path: %+v", rep.Problems)
			}
		}
		again := f.run()
		if len(again.Problems) != len(rep.Problems) {
			t.Error("report must be deterministic")
		}
	})
}
