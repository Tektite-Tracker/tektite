package service

import (
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tektite-tracker/tektite/internal/store"
)

type stubSource struct {
	issues []*importIssue
}

func (*stubSource) name() string              { return "stub" }
func (*stubSource) fixed() bool               { return false }
func (*stubSource) defaultPath(string) string { return "" }
func (s *stubSource) read(string, *ImportResult) (*importData, error) {
	return &importData{issues: s.issues}, nil
}

func stubIssue(id, status string) *importIssue {
	return &importIssue{id: id, title: "Issue " + id, rawStatus: status, line: 1}
}

func stubService(t *testing.T, issues ...*importIssue) (*Service, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".tektite"), 0o755); err != nil {
		t.Fatal(err)
	}
	repo, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	importSources["stub"] = &stubSource{issues: issues}
	t.Cleanup(func() { delete(importSources, "stub") })
	svc := &Service{Repo: repo, Now: func() time.Time { return time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC) }, Rand: rand.New(rand.NewSource(1)), Getenv: func(string) string { return "" }}
	return svc, root
}

func nodeCount(t *testing.T, root string) int {
	t.Helper()
	entries, _ := os.ReadDir(filepath.Join(root, ".tektite", "nodes"))
	return len(entries)
}

func isUsage(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Kind == KindUsage
}

func TestImportMapping(t *testing.T) {
	t.Run("Values are mapped by category, timestamps and names", func(t *testing.T) {
		at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		cat := stubIssue("1", "Ready for QA")
		cat.category = "In Progress"
		cancelled := stubIssue("2", "Parked")
		cancelled.canceledAt = at
		finished := stubIssue("3", "Shipped?")
		finished.completedAt = at
		named := stubIssue("4", "Backlog")
		started := stubIssue("5", "Working")
		started.startedAt = at
		wrongName := stubIssue("6", "Done-ish")
		wrongName.category = "To Do"
		data := &importData{issues: []*importIssue{cat, cancelled, finished, named, started, wrongName}}
		want := map[string]string{"Ready for QA": "review", "Parked": "dropped", "Shipped?": "done", "Backlog": "open", "Working": "active", "Done-ish": "open"}
		for _, v := range scanFacets(data) {
			if v.facet != facetStatus {
				continue
			}
			got, _, certain := propose(v)
			if !certain || got != want[v.value] {
				t.Errorf("%q = %q certain=%v, want %q", v.value, got, certain, want[v.value])
			}
		}
	})

	t.Run("Only ambiguous values are asked about", func(t *testing.T) {
		svc, _ := stubService(t, stubIssue("1", "Backlog"), stubIssue("2", "Zebra"))
		asked := []string{}
		res, err := svc.Import(ImportInput{Source: "stub", Path: "x", DryRun: true, Ask: func(q ImportQuestion) (string, error) {
			asked = append(asked, q.Value)
			return "blocked", nil
		}})
		if err != nil || len(asked) != 1 || asked[0] != "Zebra" || res.Mapping.Status["Zebra"] != "blocked" || res.Mapping.Status["Backlog"] != "open" {
			t.Fatalf("asked = %v, res = %+v, err = %v", asked, res, err)
		}
		_, err = svc.Import(ImportInput{Source: "stub", Path: "x", DryRun: true, Ask: func(ImportQuestion) (string, error) { return "bogus", nil }})
		if !isUsage(err) {
			t.Fatalf("an answer outside the options must be a usage error, got %v", err)
		}
	})

	t.Run("Accepting the proposal with --yes", func(t *testing.T) {
		started := stubIssue("1", "Zebra")
		started.startedAt = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		other := stubIssue("2", "Zebra")
		other.startedAt = started.startedAt
		third := stubIssue("3", "Zebra")
		third.startedAt = started.startedAt
		extra := stubIssue("4", "Zebra")
		svc, _ := stubService(t, started, other, third, extra)
		res, err := svc.Import(ImportInput{Source: "stub", Path: "x", Yes: true, DryRun: true})
		if err != nil || res.Mapping.Status["Zebra"] != "active" {
			t.Fatalf("res = %+v, err = %v", res, err)
		}
	})

	t.Run("The confirmed mapping is saved and reused", func(t *testing.T) {
		svc, root := stubService(t, stubIssue("1", "Backlog"), stubIssue("2", "Zebra"))
		if _, err := svc.Import(ImportInput{Source: "stub", Path: "x", Ask: func(ImportQuestion) (string, error) { return "review", nil }}); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(root, ".tektite", "import", "stub.yaml"))
		if err != nil || !strings.Contains(string(data), "Zebra: review") {
			t.Fatalf("saved mapping = %q, err = %v", data, err)
		}
		res, err := svc.Import(ImportInput{Source: "stub", Path: "x"})
		if err != nil || res.Skipped != 2 || res.MappingNew {
			t.Fatalf("res = %+v, err = %v", res, err)
		}
	})

	t.Run("A new value in a later export is asked about alone", func(t *testing.T) {
		svc, root := stubService(t, stubIssue("1", "Zebra"))
		if _, err := svc.Import(ImportInput{Source: "stub", Path: "x", Ask: func(ImportQuestion) (string, error) { return "review", nil }}); err != nil {
			t.Fatal(err)
		}
		importSources["stub"] = &stubSource{issues: []*importIssue{stubIssue("1", "Zebra"), stubIssue("2", "Quokka")}}
		asked := []string{}
		if _, err := svc.Import(ImportInput{Source: "stub", Path: "x", Ask: func(q ImportQuestion) (string, error) {
			asked = append(asked, q.Value)
			return "active", nil
		}}); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(filepath.Join(root, ".tektite", "import", "stub.yaml"))
		if len(asked) != 1 || asked[0] != "Quokka" || !strings.Contains(string(data), "Zebra: review") || !strings.Contains(string(data), "Quokka: active") {
			t.Fatalf("asked = %v, saved = %q", asked, data)
		}
	})

	t.Run("A mapping file can be supplied", func(t *testing.T) {
		svc, root := stubService(t, stubIssue("1", "Zebra"))
		file := filepath.Join(t.TempDir(), "m.yaml")
		if err := os.WriteFile(file, []byte("status:\n  Zebra: blocked\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		res, err := svc.Import(ImportInput{Source: "stub", Path: "x", MapFile: file, Ask: func(ImportQuestion) (string, error) { t.Fatal("must not ask"); return "", nil }})
		if err != nil || res.Items[0].Status != "blocked" {
			t.Fatalf("res = %+v, err = %v", res, err)
		}
		if _, err := os.Stat(filepath.Join(root, ".tektite", "import", "stub.yaml")); err == nil {
			t.Fatal("a supplied mapping must not be saved")
		}
	})

	t.Run("A dry run prints the proposed mapping", func(t *testing.T) {
		svc, root := stubService(t, stubIssue("1", "Backlog"))
		res, err := svc.Import(ImportInput{Source: "stub", Path: "x", DryRun: true})
		if err != nil || res.Mapping.Status["Backlog"] != "open" || res.Created != 1 {
			t.Fatalf("res = %+v, err = %v", res, err)
		}
		if _, err := os.Stat(filepath.Join(root, ".tektite", "import")); err == nil || nodeCount(t, root) != 0 {
			t.Fatal("a dry run must not write the mapping or any node")
		}
	})

	t.Run("No terminal and no saved mapping fails", func(t *testing.T) {
		svc, root := stubService(t, stubIssue("1", "Zebra"), stubIssue("2", "Quokka"))
		_, err := svc.Import(ImportInput{Source: "stub", Path: "x"})
		if !isUsage(err) || !strings.Contains(err.Error(), `status "Zebra"`) || !strings.Contains(err.Error(), `status "Quokka"`) || nodeCount(t, root) != 0 {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("Unknown values are never mapped to open", func(t *testing.T) {
		svc, root := stubService(t, stubIssue("1", "Zebra"))
		_, err := svc.Import(ImportInput{Source: "stub", Path: "x", Yes: true})
		if !isUsage(err) || nodeCount(t, root) != 0 {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("A won't do resolution drops a done status", func(t *testing.T) {
		at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		wont := stubIssue("1", "Archived")
		wont.category = "Done"
		wont.canceledAt = at
		shipped := stubIssue("2", "Closed Out")
		shipped.category = "Done"
		shipped.completedAt = at
		for _, v := range scanFacets(&importData{issues: []*importIssue{wont, shipped}}) {
			want := map[string]string{"Archived": "dropped", "Closed Out": "done"}[v.value]
			if got, _, certain := propose(v); !certain || got != want {
				t.Errorf("%q = %q certain=%v, want %q", v.value, got, certain, want)
			}
		}
	})

	t.Run("Spanish status names are recognised by keyword", func(t *testing.T) {
		want := map[string]string{"Por hacer": "open", "En curso": "active", "Hecho": "done", "Descartado": "dropped", "En revisión": "review", "Bloqueado": "blocked"}
		for name, target := range want {
			if got := keywordStatus(name); got != target {
				t.Errorf("%q = %q, want %q", name, got, target)
			}
		}
	})
}
