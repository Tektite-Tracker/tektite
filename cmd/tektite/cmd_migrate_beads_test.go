package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const beadsFixture = `{"id":"bd-1","title":"Ship the thing","description":"Top level","issue_type":"epic","status":"open","priority":0}
{"id":"bd-2","title":"Fix the bug","description":"It breaks","design":"Use a lock","issue_type":"bug","status":"in_progress","priority":1,"labels":["api","urgent"],"assignee":"sam","estimate":30,"dependencies":[{"issue_id":"bd-2","depends_on_id":"bd-1","type":"parent-child"}]}
{"id":"bd-3","title":"Write docs","status":"blocked","priority":2,"dependencies":[{"issue_id":"bd-3","depends_on_id":"bd-2","type":"blocks"},{"issue_id":"bd-3","depends_on_id":"bd-1","type":"related"},{"issue_id":"bd-3","depends_on_id":"bd-99","type":"blocks"}]}
{"id":"bd-4","title":"Later","status":"deferred","priority":3,"dependencies":[{"issue_id":"bd-4","depends_on_id":"bd-2","type":"discovered-from"}]}
{"id":"bd-5","title":"All done","status":"closed","close_reason":"shipped","priority":4,"comments":[{"author":"sam","text":"looks good"}]}
`

type migrateDoc struct {
	Items []struct {
		BeadsID string `json:"beads_id"`
		Ref     string `json:"ref"`
		Action  string `json:"action"`
		Status  string `json:"status"`
	} `json:"items"`
	Created  int `json:"created"`
	Skipped  int `json:"skipped"`
	Links    int `json:"links"`
	Warnings []struct {
		Code     string `json:"code"`
		Severity string `json:"severity"`
		Message  string `json:"message"`
	} `json:"warnings"`
}

func beadsSandbox(t *testing.T, src string) *sandbox {
	s := newSandbox(t)
	s.write(".beads/issues.jsonl", src)
	return s
}

func (s *sandbox) migrate(t *testing.T, args ...string) (result, migrateDoc) {
	t.Helper()
	got := s.tektite(append([]string{"migrate-beads", "--json"}, args...)...)
	var doc migrateDoc
	if got.code == 0 {
		decode(t, got.stdout, &doc)
	}
	return got, doc
}

func (s *sandbox) refOf(doc migrateDoc, beadsID string) string {
	for _, it := range doc.Items {
		if it.BeadsID == beadsID {
			return it.Ref
		}
	}
	return ""
}

func countFiles(t *testing.T, dir string) int {
	n := 0
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, _ error) error {
		if d != nil && !d.IsDir() {
			n++
		}
		return nil
	})
	return n
}

const beadsTombstoneFixture = `{"id":"bd-1","title":"Live work","status":"open","priority":1,"dependencies":[{"issue_id":"bd-1","depends_on_id":"bd-2","type":"blocks"}]}
{"id":"bd-2","title":"Deleted work","status":"tombstone","priority":2}
{"id":"bd-3","title":"Found while deleting","status":"open","dependencies":[{"issue_id":"bd-3","depends_on_id":"bd-2","type":"discovered-from"}]}
{"id":"bd-4","title":"Deleted too","status":"tombstone","dependencies":[{"issue_id":"bd-4","depends_on_id":"bd-2","type":"blocks"}]}
{"id":"bd-5","title":"Mystery","status":"weird"}
`

const beadsTimedFixture = `{"id":"bd-1","title":"Timed work","status":"closed","created_at":"2025-03-01T10:00:00Z","updated_at":"2025-04-01T00:00:00Z","closed_at":"2025-03-20T15:30:00Z","close_reason":"shipped","_type":"issue","comment_count":1,"dependency_count":1,"dependent_count":0,"created_by":"sam","owner":"sam@example.com","comments":[{"author":"lee","text":"nice","created_at":"2025-03-05T09:00:00Z"}],"dependencies":[{"issue_id":"bd-1","depends_on_id":"bd-2","type":"discovered-from"}]}
{"id":"bd-2","title":"Gone","status":"tombstone","created_at":"2025-02-01T08:00:00Z","closed_at":"2025-02-02T08:00:00Z"}
{"id":"bd-3","title":"Untimed","status":"in_progress"}
`

func (s *sandbox) eventsOf(ref string) string {
	entries, _ := os.ReadDir(filepath.Join(s.root, ".tektite", "events", ref))
	var sb strings.Builder
	for _, e := range entries {
		sb.WriteString(s.read(".tektite/events/" + ref + "/" + e.Name()))
	}
	return sb.String()
}

func TestCLI_MigrateBeads(t *testing.T) {
	t.Run("Importing beads issues", func(t *testing.T) {
		s := beadsSandbox(t, beadsFixture)
		got, doc := s.migrate(t)
		if got.code != 0 || doc.Created != 5 {
			t.Fatalf("got = %+v", got)
		}
		bug := s.read(".tektite/nodes/" + s.refOf(doc, "bd-2") + ".md")
		for _, want := range []string{"kind: ticket", "title: Fix the bug", "beads_id: bd-2", "It breaks", "## Design", "Use a lock", "issue_type: bug"} {
			if !strings.Contains(bug, want) {
				t.Errorf("bug missing %q:\n%s", want, bug)
			}
		}
		if epic := s.read(".tektite/nodes/" + s.refOf(doc, "bd-1") + ".md"); !strings.Contains(epic, "kind: plan") {
			t.Errorf("epic = %s", epic)
		}
		if s.read(".beads/issues.jsonl") != beadsFixture {
			t.Errorf("source file changed")
		}
	})

	t.Run("Beads statuses map to tektite statuses", func(t *testing.T) {
		s := beadsSandbox(t, beadsFixture)
		_, doc := s.migrate(t)
		want := map[string]string{"bd-1": "open", "bd-2": "active", "bd-3": "blocked", "bd-4": "blocked", "bd-5": "done"}
		for id, status := range want {
			show := s.tektite("show", s.refOf(doc, id), "--json")
			if show.code != 0 || !strings.Contains(show.stdout, `"status": "`+status+`"`) {
				t.Errorf("%s want %s: %+v", id, status, show)
			}
		}
		found := false
		for _, w := range doc.Warnings {
			found = found || (strings.Contains(w.Message, "bd-4") && strings.Contains(w.Message, "deferred"))
		}
		if !found {
			t.Errorf("no deferred warning: %+v", doc.Warnings)
		}
	})

	t.Run("Deleted beads issues are imported as dropped", func(t *testing.T) {
		s := beadsSandbox(t, beadsTombstoneFixture)
		got, doc := s.migrate(t)
		if got.code != 0 || doc.Created != 5 {
			t.Fatalf("got = %+v", got)
		}
		gone := s.refOf(doc, "bd-2")
		if show := s.tektite("show", gone, "--json"); !strings.Contains(show.stdout, `"status": "dropped"`) {
			t.Errorf("tombstone status: %s", show.stdout)
		}
		unknown, skipped := 0, 0
		for _, w := range doc.Warnings {
			if strings.Contains(w.Message, "bd-2") && w.Code == "TKT-W014" {
				t.Errorf("tombstone warned: %+v", w)
			}
			if w.Code == "TKT-W014" && strings.Contains(w.Message, "bd-5") {
				unknown++
			}
			if w.Code == "TKT-W015" && strings.Contains(w.Message, "bd-1") && strings.Contains(w.Message, "deleted") {
				skipped++
			}
		}
		if unknown != 1 || skipped != 1 {
			t.Errorf("unknown = %d skipped = %d warnings = %+v", unknown, skipped, doc.Warnings)
		}
		if blocker := s.tektite("show", gone, "--json"); !strings.Contains(blocker.stdout, `"blocks"`) || !strings.Contains(blocker.stdout, s.refOf(doc, "bd-4")) {
			t.Errorf("dropped-to-dropped link missing: %s", blocker.stdout)
		}
		if strings.Contains(s.tektite("show", gone, "--json").stdout, s.refOf(doc, "bd-1")) {
			t.Errorf("live ticket is blocked by a dropped one")
		}
		if derived := s.tektite("show", s.refOf(doc, "bd-3"), "--json"); !strings.Contains(derived.stdout, `"derived-from"`) || !strings.Contains(derived.stdout, gone) {
			t.Errorf("derived-from link missing: %s", derived.stdout)
		}
		before := countFiles(t, filepath.Join(s.root, ".tektite"))
		again, redo := s.migrate(t)
		if again.code != 0 || redo.Created != 0 || redo.Skipped != 5 || countFiles(t, filepath.Join(s.root, ".tektite")) != before {
			t.Errorf("rerun = %+v %+v", again, redo)
		}
	})

	t.Run("Beads bookkeeping fields are not imported", func(t *testing.T) {
		s := beadsSandbox(t, beadsTimedFixture)
		_, doc := s.migrate(t)
		src := s.read(".tektite/nodes/" + s.refOf(doc, "bd-1") + ".md")
		for _, bad := range []string{"_type", "comment_count", "dependency_count", "dependent_count", "updated_at"} {
			if strings.Contains(src, bad) {
				t.Errorf("%s was imported:\n%s", bad, src)
			}
		}
		if !strings.Contains(src, "created_by: sam") || !strings.Contains(src, "owner: sam@example.com") {
			t.Errorf("created_by or owner missing:\n%s", src)
		}
	})

	t.Run("Beads timestamps become the history", func(t *testing.T) {
		s := beadsSandbox(t, beadsTimedFixture)
		_, doc := s.migrate(t)
		done, gone := s.refOf(doc, "bd-1"), s.refOf(doc, "bd-2")
		src := s.read(".tektite/nodes/" + done + ".md")
		if strings.Contains(src, "created_at") || strings.Contains(src, "closed_at") {
			t.Errorf("timestamps in frontmatter:\n%s", src)
		}
		events := s.eventsOf(done)
		for _, want := range []string{"2025-03-20T15:30:00Z", "2025-03-05T09:00:00Z", "2025-03-01T10:00:00Z"} {
			if !strings.Contains(events, want) {
				t.Errorf("events of %s missing %s:\n%s", done, want, events)
			}
		}
		if dropped := s.eventsOf(gone); !strings.Contains(dropped, "2025-02-02T08:00:00Z") || strings.Contains(dropped, "2025-02-01T08:00:00Z") {
			t.Errorf("dropped events:\n%s", dropped)
		}
		before := countFiles(t, filepath.Join(s.root, ".tektite"))
		again, redo := s.migrate(t)
		if again.code != 0 || redo.Created != 0 || redo.Skipped != 3 || countFiles(t, filepath.Join(s.root, ".tektite")) != before {
			t.Errorf("rerun = %+v %+v", again, redo)
		}
	})

	t.Run("Beads issues without timestamps use the import time", func(t *testing.T) {
		s := beadsSandbox(t, beadsTimedFixture)
		_, doc := s.migrate(t)
		events := s.eventsOf(s.refOf(doc, "bd-3"))
		if !strings.Contains(events, "2026-11-27T12:00:00Z") || strings.Contains(events, "2025-") {
			t.Errorf("events:\n%s", events)
		}
	})

	t.Run("Beads priorities and labels are kept", func(t *testing.T) {
		s := beadsSandbox(t, beadsFixture)
		_, doc := s.migrate(t)
		bug := s.read(".tektite/nodes/" + s.refOf(doc, "bd-2") + ".md")
		if !strings.Contains(bug, "priority: high") || !strings.Contains(bug, "api") || !strings.Contains(bug, "urgent") || !strings.Contains(bug, "assignee: sam") {
			t.Errorf("bug = %s", bug)
		}
		if epic := s.read(".tektite/nodes/" + s.refOf(doc, "bd-1") + ".md"); !strings.Contains(epic, "priority: critical") {
			t.Errorf("epic = %s", epic)
		}
	})

	t.Run("Beads dependencies become parents and links", func(t *testing.T) {
		s := beadsSandbox(t, beadsFixture)
		_, doc := s.migrate(t)
		epic, bug, docs, later := s.refOf(doc, "bd-1"), s.refOf(doc, "bd-2"), s.refOf(doc, "bd-3"), s.refOf(doc, "bd-4")
		if src := s.read(".tektite/nodes/" + bug + ".md"); !strings.Contains(src, "parent: "+epic) {
			t.Errorf("parent missing:\n%s", src)
		}
		if blocker := s.tektite("show", bug, "--json"); !strings.Contains(blocker.stdout, `"blocks"`) || !strings.Contains(blocker.stdout, docs) {
			t.Errorf("blocks link missing: %s", blocker.stdout)
		}
		if derived := s.tektite("show", later, "--json"); !strings.Contains(derived.stdout, `"derived-from"`) || !strings.Contains(derived.stdout, bug) {
			t.Errorf("derived-from link missing: %s", derived.stdout)
		}
		if doc.Links != 2 {
			t.Errorf("links = %d", doc.Links)
		}
	})

	t.Run("Unknown beads fields are kept as frontmatter", func(t *testing.T) {
		s := beadsSandbox(t, beadsFixture)
		_, doc := s.migrate(t)
		if bug := s.read(".tektite/nodes/" + s.refOf(doc, "bd-2") + ".md"); !strings.Contains(bug, "estimate: 30") {
			t.Errorf("bug = %s", bug)
		}
	})

	t.Run("Beads dependencies that cannot be mapped warn", func(t *testing.T) {
		s := beadsSandbox(t, beadsFixture+"not json\n")
		got, doc := s.migrate(t)
		related, missing, badLine := false, false, false
		for _, w := range doc.Warnings {
			related = related || (w.Code == "TKT-W014" && strings.Contains(w.Message, "related"))
			missing = missing || (w.Code == "TKT-W015" && strings.Contains(w.Message, "bd-99"))
			badLine = badLine || (w.Code == "TKT-W015" && strings.Contains(w.Message, "not a JSON object"))
			if w.Severity != "warning" {
				t.Errorf("severity = %q", w.Severity)
			}
		}
		if got.code != 0 || !related || !missing || !badLine || doc.Created != 5 {
			t.Errorf("got = %+v warnings = %+v", got, doc.Warnings)
		}
	})

	t.Run("A dry run writes nothing", func(t *testing.T) {
		s := beadsSandbox(t, beadsFixture)
		before := countFiles(t, filepath.Join(s.root, ".tektite"))
		got, doc := s.migrate(t, "--dry-run")
		if got.code != 0 || doc.Created != 5 || doc.Links != 2 || len(doc.Items) != 5 || countFiles(t, filepath.Join(s.root, ".tektite")) != before {
			t.Errorf("got = %+v doc = %+v", got, doc)
		}
		text := s.tektite("migrate-beads", "--dry-run")
		if !strings.Contains(text.stdout, "would create bd-2 ticket Fix the bug") {
			t.Errorf("text = %q", text.stdout)
		}
	})

	t.Run("Importing again changes nothing", func(t *testing.T) {
		s := beadsSandbox(t, beadsFixture)
		s.migrate(t)
		before := countFiles(t, filepath.Join(s.root, ".tektite"))
		got, doc := s.migrate(t)
		if got.code != 0 || doc.Created != 0 || doc.Skipped != 5 || doc.Links != 0 || countFiles(t, filepath.Join(s.root, ".tektite")) != before {
			t.Errorf("got = %+v doc = %+v", got, doc)
		}
		for _, it := range doc.Items {
			if it.Action != "skip" || it.Ref == "" {
				t.Errorf("item = %+v", it)
			}
		}
	})

	t.Run("A missing source file", func(t *testing.T) {
		s := newSandbox(t)
		if got := s.tektite("migrate-beads"); got.code != exitNotFound || !strings.Contains(got.stderr, "issues.jsonl") {
			t.Errorf("got = %+v", got)
		}
	})
}
