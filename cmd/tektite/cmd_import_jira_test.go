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

	t.Run("Importing a Jira JSON export", func(t *testing.T) {
		s := newSandbox(t)
		s.write("jira.json", jiraJSONFixture)
		got := s.tektite("import", "jira", "--json", "--from", s.root+"/jira.json")
		var doc migrateDoc
		if got.code != 0 {
			t.Fatalf("got = %+v", got)
		}
		decode(t, got.stdout, &doc)
		want := map[string]string{"J-1": "open", "J-2": "review", "J-3": "dropped"}
		for _, it := range doc.Items {
			if it.Status != want[it.SourceID] {
				t.Errorf("%s status = %s, want %s", it.SourceID, it.Status, want[it.SourceID])
			}
		}
		story := s.read(".tektite/nodes/" + s.refOf(doc, "J-2") + ".md")
		epic := s.read(".tektite/nodes/" + s.refOf(doc, "J-1") + ".md")
		for _, want := range []string{"parent: " + s.refOf(doc, "J-1"), "assignee: Sam Lee", "- ui", "priority: medium", "source_id: J-2"} {
			if !strings.Contains(story, want) {
				t.Errorf("story lacks %q:\n%s", want, story)
			}
		}
		if !strings.Contains(epic, "kind: plan") {
			t.Errorf("epic = %s", epic)
		}
		if ev := s.eventsOf(s.refOf(doc, "J-2")); !strings.Contains(ev, "rel: blocks") || !strings.Contains(ev, `at: "2024-03-27T16:00:00Z"`) {
			t.Errorf("story events = %s", ev)
		}
		if ev := s.eventsOf(s.refOf(doc, "J-3")); !strings.Contains(ev, `at: "2024-03-28T10:00:00Z"`) {
			t.Errorf("bug events = %s", ev)
		}
	})

	t.Run("Jira rich text becomes plain text", func(t *testing.T) {
		s := newSandbox(t)
		s.write("jira.json", jiraJSONFixture)
		got := s.tektite("import", "jira", "--json", "--from", s.root+"/jira.json")
		var doc migrateDoc
		decode(t, got.stdout, &doc)
		story := s.read(".tektite/nodes/" + s.refOf(doc, "J-2") + ".md")
		if !strings.Contains(story, "Filter by tag\n") || !strings.Contains(story, "- first\n- second") {
			t.Fatalf("story = %s", story)
		}
		if ev := s.eventsOf(s.refOf(doc, "J-2")); !strings.Contains(ev, "Lee: Looks fine") {
			t.Fatalf("events = %s", ev)
		}
	})

	t.Run("A JSON file that is not a Jira export is refused", func(t *testing.T) {
		s := newSandbox(t)
		s.write("jira.json", `{"hello":"world"}`)
		if got := s.tektite("import", "jira", "--from", s.root+"/jira.json"); got.code != exitUsage {
			t.Fatalf("got = %+v", got)
		}
	})
}

const jiraJSONFixture = `{"issues":[
{"key":"J-1","fields":{"summary":"Search revamp","issuetype":{"name":"Epic"},"status":{"name":"Backlog Grooming","statusCategory":{"name":"To Do"}},"priority":{"name":"High"},"created":"2024-03-27T09:00:00.000+0000"}},
{"key":"J-2","fields":{"summary":"Add filters","issuetype":{"name":"Story"},"status":{"name":"In Review","statusCategory":{"name":"In Progress"}},"priority":{"name":"Medium"},"assignee":{"displayName":"Sam Lee"},"labels":["ui"],"parent":{"key":"J-1"},"created":"2024-03-27T15:12:00.000+0000",
"description":{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Filter by tag"}]},{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"first"}]}]},{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"second"}]}]}]}]},
"issuelinks":[{"type":{"name":"Blocks"},"outwardIssue":{"key":"J-3"}}],
"comment":{"comments":[{"author":{"displayName":"Lee"},"body":"Looks fine","created":"2024-03-27T16:00:00.000+0000"}]}}},
{"key":"J-3","fields":{"summary":"Drop legacy","issuetype":{"name":"Bug"},"status":{"name":"Won't Do","statusCategory":{"name":"Done"}},"priority":{"name":"Low"},"resolution":{"name":"Won't Do"},"created":"2024-03-28T08:00:00.000+0000","resolutiondate":"2024-03-28T10:00:00.000+0000"}}
]}`

func TestCLI_ImportJiraMarks(t *testing.T) {
	t.Run("Jira rich text keeps bold, italic, code and links", func(t *testing.T) {
		s := newSandbox(t)
		s.write("jira.json", `{"issues":[{"key":"J-1","fields":{"summary":"Marks","issuetype":{"name":"Task"},"status":{"name":"Done","statusCategory":{"name":"Done"}},"priority":{"name":"Low"},"description":{"type":"doc","content":[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Title"}]},{"type":"paragraph","content":[{"type":"text","text":"bold","marks":[{"type":"strong"}]},{"type":"text","text":" and "},{"type":"text","text":"it","marks":[{"type":"em"}]},{"type":"text","text":" and "},{"type":"text","text":"x()","marks":[{"type":"code"}]},{"type":"text","text":" and "},{"type":"text","text":"site","marks":[{"type":"link","attrs":{"href":"https://e.test"}}]}]}]}}}]}`)
		got := s.tektite("import", "jira", "--json", "--yes", "--from", s.root+"/jira.json")
		var doc migrateDoc
		decode(t, got.stdout, &doc)
		body := s.read(".tektite/nodes/" + s.refOf(doc, "J-1") + ".md")
		if !strings.Contains(body, "## Title") || !strings.Contains(body, "**bold** and *it* and `x()` and [site](https://e.test)") {
			t.Fatalf("body = %s", body)
		}
	})
}
