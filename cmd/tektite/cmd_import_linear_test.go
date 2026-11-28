package main

import (
	"strings"
	"testing"
)

const linearHeader = "ID,Title,Description,Status,Priority,Labels,Assignee,Team,Estimate,Created,Updated,Started,Completed,Canceled,Parent issue,Related to,Blocked by,Duplicate of\n"

const linearFixture = linearHeader +
	`T-1,Epic work,"Top level, with a comma",In Dev,High,"area:x, ui",sam@x.io,Core,3,2026-01-01T10:00:00.000Z,2026-02-01T00:00:00.000Z,2026-01-02T10:00:00.000Z,,,,"T-2",,` + "\n" +
	`T-2,Child work,,Shipped,Urgent,,,Core,,2026-01-01T11:00:00.000Z,2026-02-01T00:00:00.000Z,2026-01-03T10:00:00.000Z,2026-01-05T10:00:00.000Z,,T-1,T-1,T-3,` + "\n" +
	`T-3,Blocker,,Backlog,No priority,,,,,2026-01-01T12:00:00.000Z,,,,,,,,` + "\n" +
	`T-4,Not doing this,,Won't Fix,Low,,,,,2026-01-01T13:00:00.000Z,,,,2026-01-06T10:00:00.000Z,,,,T-3` + "\n" +
	`T-5,Orphan,,Backlog,Medium,,,,,2026-01-01T14:00:00.000Z,,,,,T-99,,,` + "\n"

func linearSandbox(t *testing.T, src string) *sandbox {
	s := newSandbox(t)
	s.write("linear.csv", src)
	return s
}

func (s *sandbox) importLinear(t *testing.T, args ...string) (result, migrateDoc) {
	t.Helper()
	got := s.tektite(append([]string{"import", "linear", "--json", "--from", s.root + "/linear.csv"}, args...)...)
	var doc migrateDoc
	if got.code == 0 {
		decode(t, got.stdout, &doc)
	}
	return got, doc
}

func TestCLI_ImportLinear(t *testing.T) {
	t.Run("A source needs an export file", func(t *testing.T) {
		s := newSandbox(t)
		if got := s.tektite("import", "linear"); got.code != exitUsage {
			t.Fatalf("got = %+v", got)
		}
	})

	t.Run("An export of the wrong shape is refused", func(t *testing.T) {
		s := linearSandbox(t, "Name,State\nA,B\n")
		got, _ := s.importLinear(t)
		if got.code != exitUsage || !strings.Contains(got.stderr, "ID") {
			t.Fatalf("got = %+v", got)
		}
	})

	t.Run("Importing Linear issues", func(t *testing.T) {
		s := linearSandbox(t, linearFixture)
		got, doc := s.importLinear(t)
		if got.code != 0 || doc.Created != 5 {
			t.Fatalf("got = %+v", got)
		}
		top := s.read(".tektite/nodes/" + s.refOf(doc, "T-1") + ".md")
		for _, want := range []string{"kind: ticket", "title: Epic work", "source: linear", "source_id: T-1", "priority: high", "assignee: sam@x.io", "area:x", "Top level, with a comma"} {
			if !strings.Contains(top, want) {
				t.Errorf("node lacks %q:\n%s", want, top)
			}
		}
	})

	t.Run("Linear statuses are mapped by workflow state", func(t *testing.T) {
		s := linearSandbox(t, linearFixture)
		_, doc := s.importLinear(t)
		want := map[string]string{"T-1": "active", "T-2": "done", "T-3": "open", "T-4": "dropped", "T-5": "open"}
		for _, it := range doc.Items {
			if it.Status != want[it.SourceID] {
				t.Errorf("%s status = %s, want %s", it.SourceID, it.Status, want[it.SourceID])
			}
		}
		mapping := s.read(".tektite/import/linear.yaml")
		if !strings.Contains(mapping, "Won't Fix: dropped") || !strings.Contains(mapping, "Shipped: done") {
			t.Fatalf("saved mapping = %s", mapping)
		}
		odd := linearSandbox(t, linearHeader+"Z-1,Odd,,Zebra,,,,,,2026-01-01T10:00:00.000Z,,,,,,,,\n")
		if got, _ := odd.importLinear(t); got.code != exitUsage || !strings.Contains(got.stderr, `status "Zebra"`) {
			t.Fatalf("an unexplained status must be refused, got %+v", got)
		}
	})

	t.Run("Linear parent issues become parents", func(t *testing.T) {
		s := linearSandbox(t, linearFixture)
		_, doc := s.importLinear(t)
		child := s.read(".tektite/nodes/" + s.refOf(doc, "T-2") + ".md")
		if !strings.Contains(child, "parent: "+s.refOf(doc, "T-1")) {
			t.Fatalf("child = %s", child)
		}
		warned := false
		for _, w := range doc.Warnings {
			warned = warned || strings.Contains(w.Message, "T-99")
		}
		if !warned {
			t.Fatalf("no warning for the missing parent: %+v", doc.Warnings)
		}
	})

	t.Run("Linear relations become typed links", func(t *testing.T) {
		s := linearSandbox(t, linearFixture)
		_, doc := s.importLinear(t)
		blocker, child, dup, top := s.refOf(doc, "T-3"), s.refOf(doc, "T-2"), s.refOf(doc, "T-4"), s.refOf(doc, "T-1")
		if ev := s.eventsOf(blocker); !strings.Contains(ev, "rel: blocks") || !strings.Contains(ev, "target: "+child) || !strings.Contains(ev, "rel: supersedes") || !strings.Contains(ev, "target: "+dup) {
			t.Fatalf("blocker events = %s", ev)
		}
		relates := strings.Count(s.eventsOf(top), "rel: relates-to") + strings.Count(s.eventsOf(child), "rel: relates-to")
		if relates != 1 || doc.Links != 3 {
			t.Fatalf("relates-to links = %d, links = %d", relates, doc.Links)
		}
	})

	t.Run("Linear timestamps become the history", func(t *testing.T) {
		s := linearSandbox(t, linearFixture)
		_, doc := s.importLinear(t)
		active, done := s.eventsOf(s.refOf(doc, "T-1")), s.eventsOf(s.refOf(doc, "T-2"))
		if !strings.Contains(active, `at: "2026-01-02T10:00:00Z"`) || !strings.Contains(done, `at: "2026-01-05T10:00:00Z"`) {
			t.Fatalf("active = %s\ndone = %s", active, done)
		}
	})

	t.Run("Linear columns without a mapping are kept as frontmatter", func(t *testing.T) {
		s := linearSandbox(t, linearFixture)
		_, doc := s.importLinear(t)
		top := s.read(".tektite/nodes/" + s.refOf(doc, "T-1") + ".md")
		if !strings.Contains(top, "team: Core") || !strings.Contains(top, "estimate: ") || strings.Contains(top, "updated") {
			t.Fatalf("node = %s", top)
		}
	})

	t.Run("Importing Linear issues again changes nothing", func(t *testing.T) {
		s := linearSandbox(t, linearFixture)
		s.importLinear(t)
		before := countFiles(t, s.root+"/.tektite")
		got, doc := s.importLinear(t)
		if got.code != 0 || doc.Skipped != 5 || doc.Created != 0 || countFiles(t, s.root+"/.tektite") != before {
			t.Fatalf("got = %+v, doc = %+v", got, doc)
		}
	})
}
