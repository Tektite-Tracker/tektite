package main

import (
	"strings"
	"testing"
)

const jiraHeader = "Summary,Issue key,Issue id,Issue Type,Status,Priority,Resolution,Assignee,Reporter,Created,Updated,Resolved,Description,Labels,Labels,Sprint,Σ Time Spent,Parent,Parent key,Status Category,Outward issue link (Blocks),Outward issue link (Relates),Outward issue link (Duplicate),Outward issue link (Problem/Incident),Comment\n"

const jiraFixture = jiraHeader +
	`Search revamp,J-1,10001,Epic,Backlog Grooming,High,,,Pat,27/Mar/24 9:00 AM,01/Apr/24 9:00 AM,,"The big one",,,Sprint 1,0,,,To Do,,,,,` + "\n" +
	`Add filters,J-2,10002,Story,In Review,Medium,,Sam,Pat,27/Mar/24 3:12 PM,01/Apr/24 9:00 AM,,"Filter by tag",ui,api,Sprint 1,60,10001,,In Progress,J-3,J-4,,,27/Mar/24 4:00 PM;acct1;Looks fine` + "\n" +
	`Drop legacy,J-3,10003,Bug,Won't Do,Low,Won't Do,,Pat,28/03/2024 14:44,01/Apr/24 9:00 AM,28/Mar/24 10:00 AM,,,,,,,J-1,Done,,,J-2,J-4,` + "\n" +
	`Ship it,J-4,10004,Task,Done,Highest,Done,,Pat,28/03/2024 15:00,01/Apr/24 9:00 AM,29/Mar/24 11:30 AM,,,,,,,J-9,Done,,J-2,,,` + "\n"

func jiraSandbox(t *testing.T, src string) *sandbox {
	s := newSandbox(t)
	s.write("jira.csv", src)
	return s
}

func (s *sandbox) importJira(t *testing.T, args ...string) (result, migrateDoc) {
	t.Helper()
	got := s.tektite(append([]string{"import", "jira", "--json", "--from", s.root + "/jira.csv"}, args...)...)
	var doc migrateDoc
	if got.code == 0 {
		decode(t, got.stdout, &doc)
	}
	return got, doc
}

func TestCLI_ImportJira(t *testing.T) {
	t.Run("An export of the wrong shape is refused by jira", func(t *testing.T) {
		s := jiraSandbox(t, "Name,State\nA,B\n")
		got, _ := s.importJira(t)
		if got.code != exitUsage || !strings.Contains(got.stderr, "Summary") {
			t.Fatalf("got = %+v", got)
		}
	})

	t.Run("Importing Jira issues", func(t *testing.T) {
		s := jiraSandbox(t, jiraFixture)
		got, doc := s.importJira(t)
		if got.code != 0 || doc.Created != 4 {
			t.Fatalf("got = %+v", got)
		}
		story := s.read(".tektite/nodes/" + s.refOf(doc, "J-2") + ".md")
		for _, want := range []string{"kind: ticket", "title: Add filters", "source: jira", "source_id: J-2", "priority: medium", "assignee: Sam", "Filter by tag", "- ui", "- api"} {
			if !strings.Contains(story, want) {
				t.Errorf("node lacks %q:\n%s", want, story)
			}
		}
	})

	t.Run("Jira statuses are mapped by status category", func(t *testing.T) {
		s := jiraSandbox(t, jiraFixture)
		_, doc := s.importJira(t)
		want := map[string]string{"J-1": "open", "J-2": "review", "J-3": "dropped", "J-4": "done"}
		for _, it := range doc.Items {
			if it.Status != want[it.SourceID] {
				t.Errorf("%s status = %s, want %s", it.SourceID, it.Status, want[it.SourceID])
			}
		}
	})

	t.Run("Jira epics become plans", func(t *testing.T) {
		s := jiraSandbox(t, jiraFixture)
		_, doc := s.importJira(t)
		epic := s.read(".tektite/nodes/" + s.refOf(doc, "J-1") + ".md")
		story := s.read(".tektite/nodes/" + s.refOf(doc, "J-2") + ".md")
		bug := s.read(".tektite/nodes/" + s.refOf(doc, "J-3") + ".md")
		if !strings.Contains(epic, "kind: plan") || !strings.Contains(story, "parent: "+s.refOf(doc, "J-1")) || !strings.Contains(bug, "parent: "+s.refOf(doc, "J-1")) {
			t.Fatalf("epic = %s\nstory = %s\nbug = %s", epic, story, bug)
		}
	})

	t.Run("Jira issue links become typed links", func(t *testing.T) {
		s := jiraSandbox(t, jiraFixture)
		_, doc := s.importJira(t)
		story, bug, task := s.refOf(doc, "J-2"), s.refOf(doc, "J-3"), s.refOf(doc, "J-4")
		events := s.eventsOf(story)
		if !strings.Contains(events, "rel: blocks") || !strings.Contains(events, "target: "+bug) {
			t.Fatalf("story events = %s", events)
		}
		if !strings.Contains(events, "rel: supersedes") || !strings.Contains(s.eventsOf(bug), "rel: problem-incident") {
			t.Fatalf("story events = %s\nbug events = %s", events, s.eventsOf(bug))
		}
		relates := strings.Count(s.eventsOf(story), "rel: relates-to") + strings.Count(s.eventsOf(task), "rel: relates-to") + strings.Count(s.eventsOf(bug), "rel: relates-to")
		if relates != 1 {
			t.Fatalf("relates-to links = %d, want one for the pair listed on both sides", relates)
		}
		odd := jiraSandbox(t, jiraHeader+"One,K-1,1,Task,Done,Low,,,,01/Jan/24 9:00 AM,,02/Jan/24 9:00 AM,,,,,,,,Done,,,,K-2,\nTwo,K-2,2,Task,Done,Low,,,,01/Jan/24 9:00 AM,,02/Jan/24 9:00 AM,,,,,,,,Done,,,,,\n")
		_, oddDoc := odd.importJira(t)
		if !strings.Contains(odd.eventsOf(odd.refOf(oddDoc, "K-1")), "rel: problem-incident") {
			t.Fatalf("events = %s", odd.eventsOf(odd.refOf(oddDoc, "K-1")))
		}
	})

	t.Run("Jira dates are read in the export's format", func(t *testing.T) {
		s := jiraSandbox(t, jiraFixture)
		_, doc := s.importJira(t)
		if !strings.Contains(s.eventsOf(s.refOf(doc, "J-3")), `at: "2024-03-28T10:00:00Z"`) {
			t.Fatalf("events = %s", s.eventsOf(s.refOf(doc, "J-3")))
		}
		amb := jiraSandbox(t, jiraHeader+"One,K-1,1,Task,Done,Low,,,,04-08-2025 14:44,,04-08-2025 15:00,,,,,,,,Done,,,,,\n")
		got, ambDoc := amb.importJira(t)
		warned := false
		for _, w := range ambDoc.Warnings {
			warned = warned || strings.Contains(w.Message, "day first")
		}
		if got.code != 0 || !warned || !strings.Contains(amb.eventsOf(amb.refOf(ambDoc, "K-1")), `at: "2025-08-04T15:00:00Z"`) {
			t.Fatalf("got = %+v, warnings = %+v", got, ambDoc.Warnings)
		}
	})

	t.Run("Jira resolved times and comments become the history", func(t *testing.T) {
		s := jiraSandbox(t, jiraFixture)
		_, doc := s.importJira(t)
		done := s.eventsOf(s.refOf(doc, "J-4"))
		commented := s.eventsOf(s.refOf(doc, "J-2"))
		if !strings.Contains(done, `at: "2024-03-29T11:30:00Z"`) || !strings.Contains(commented, `at: "2024-03-27T16:00:00Z"`) || !strings.Contains(commented, "Looks fine") {
			t.Fatalf("done = %s\ncommented = %s", done, commented)
		}
	})

	t.Run("Jira columns without a mapping are kept as frontmatter", func(t *testing.T) {
		s := jiraSandbox(t, jiraFixture)
		_, doc := s.importJira(t)
		story := s.read(".tektite/nodes/" + s.refOf(doc, "J-2") + ".md")
		if !strings.Contains(story, "sprint: Sprint 1") || !strings.Contains(story, "reporter: Pat") || strings.Contains(story, "updated") || strings.Contains(story, "time_spent") {
			t.Fatalf("node = %s", story)
		}
	})
}
