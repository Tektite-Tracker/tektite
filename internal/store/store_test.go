package store_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tektite-tracker/tektite/internal/node"
	"github.com/tektite-tracker/tektite/internal/store"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func nodeSrc(id, kind, title, parent string) string {
	return "---\nid: " + id + "\nkind: " + kind + "\ntitle: " + title + "\nparent: " + parent + "\n---\nbody\n"
}

func eventSrc(typ, at string) string {
	return "---\ntype: " + typ + "\nactor: human:jesus\nat: " + at + "\n---\n"
}

func repo(t *testing.T) (*store.Repo, string) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".tektite"), 0o755); err != nil {
		t.Fatal(err)
	}
	r, err := store.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return r, r.Root
}

const (
	idA = "01HZ3K8Q5W2X9N4T7V6B1C0D3F"
	idB = "01HZ3K8Q5W2X9N4T7V6B1C0D9E"
	idC = "01HZ3K8Q5W2X9N4T7V6B1C0D7C"
	idD = "01HZ3K8Q5W2X9N4T7V6B1C0D5D"
)

func refs(s *store.Snapshot) []string {
	out := []string{}
	for _, n := range s.Nodes {
		out = append(out, n.Ref)
	}
	return out
}

func TestDiscover(t *testing.T) {
	t.Run("The root is found by walking upwards", func(t *testing.T) {
		root := t.TempDir()
		deep := filepath.Join(root, "a", "b", "c")
		if err := os.MkdirAll(deep, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "a", ".tektite"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, ".tektite"), 0o755); err != nil {
			t.Fatal(err)
		}
		r, err := store.Discover(deep)
		if err != nil {
			t.Fatal(err)
		}
		want, _ := filepath.EvalSymlinks(filepath.Join(root, "a"))
		got, _ := filepath.EvalSymlinks(r.Root)
		if got != want {
			t.Errorf("root = %q, want nearest %q", got, want)
		}
	})

	t.Run("No repository", func(t *testing.T) {
		if _, err := store.Discover(t.TempDir()); !errors.Is(err, store.ErrNoRepo) {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("An explicit root overrides discovery", func(t *testing.T) {
		r, _ := repo(t)
		other, err := store.Open(r.Root)
		if err != nil || other.Root != r.Root {
			t.Errorf("open = %+v err=%v", other, err)
		}
		if _, err := store.Open(t.TempDir()); !errors.Is(err, store.ErrNoRepo) {
			t.Errorf("err = %v", err)
		}
	})
}

func TestLoad(t *testing.T) {
	t.Run("Nodes and events are loaded from disk", func(t *testing.T) {
		r, root := repo(t)
		write(t, root, ".tektite/nodes/b-9e.md", nodeSrc(idB, "ticket", "B", ""))
		write(t, root, ".tektite/nodes/a-d3f.md", nodeSrc(idA, "spec", "A", ""))
		write(t, root, ".tektite/events/a-d3f/01HZ3M0A1B2C3D4E5F6G7H8J91.md", eventSrc("set-status\nstatus: active", "2026-11-27T10:00:00Z"))
		write(t, root, ".tektite/events/a-d3f/bad.md", "no frontmatter")
		s, err := r.Load()
		if err != nil {
			t.Fatal(err)
		}
		if got := refs(s); !reflect.DeepEqual(got, []string{"a-d3f", "b-9e"}) {
			t.Errorf("refs = %v", got)
		}
		if len(s.Events["a-d3f"]) != 2 || len(s.EventsOf("a-d3f")) != 2 || len(s.Events["b-9e"]) != 0 {
			t.Errorf("events = %+v", s.Events)
		}
		if s.Nodes[0].Path != ".tektite/nodes/a-d3f.md" {
			t.Errorf("path = %q", s.Nodes[0].Path)
		}
		bad := 0
		for _, f := range s.Events["a-d3f"] {
			if !f.Event.Valid && len(f.Diags) > 0 {
				bad++
			}
		}
		if bad != 1 {
			t.Errorf("invalid events with diagnostics = %d", bad)
		}
	})

	t.Run("Only visible markdown files are read", func(t *testing.T) {
		r, root := repo(t)
		write(t, root, ".tektite/nodes/a-d3f.md", nodeSrc(idA, "spec", "A", ""))
		write(t, root, ".tektite/nodes/.tmp-123", "x")
		write(t, root, ".tektite/nodes/.hidden.md", nodeSrc(idB, "spec", "B", ""))
		write(t, root, ".tektite/nodes/notes.txt", "x")
		write(t, root, ".tektite/events/.git/x.md", "x")
		s, err := r.Load()
		if err != nil {
			t.Fatal(err)
		}
		if got := refs(s); !reflect.DeepEqual(got, []string{"a-d3f"}) || len(s.Orphans) != 0 {
			t.Errorf("refs = %v orphans = %v", got, s.Orphans)
		}
	})

	t.Run("A repository without nodes or events is empty", func(t *testing.T) {
		r, _ := repo(t)
		s, err := r.Load()
		if err != nil || len(s.Nodes) != 0 || len(s.Tree.Roots) != 0 {
			t.Errorf("snapshot = %+v err=%v", s, err)
		}
	})

	t.Run("Duplicate ids are reported and both nodes stay readable", func(t *testing.T) {
		r, root := repo(t)
		write(t, root, ".tektite/nodes/one-d3f.md", nodeSrc(idA, "spec", "One", ""))
		write(t, root, ".tektite/nodes/two-9e1.md", nodeSrc(idA, "spec", "Two", ""))
		write(t, root, ".tektite/nodes/three-7c1.md", nodeSrc(idB, "spec", "Three", ""))
		s, err := r.Load()
		if err != nil {
			t.Fatal(err)
		}
		if len(s.Nodes) != 3 {
			t.Errorf("nodes = %v", refs(s))
		}
		want := map[string][]string{idA: {"one-d3f", "two-9e1"}}
		if got := s.DuplicateIDs(); !reflect.DeepEqual(got, want) {
			t.Errorf("duplicates = %v", got)
		}
	})

	t.Run("A reference resolves by unique hash suffix", func(t *testing.T) {
		r, root := repo(t)
		write(t, root, ".tektite/nodes/auth-retry-3fa.md", nodeSrc(idA, "ticket", "Retry", ""))
		write(t, root, ".tektite/nodes/auth-hardening-7c1.md", nodeSrc(idB, "plan", "Hardening", ""))
		s, _ := r.Load()
		got, err := s.Resolve("3fa")
		if err != nil || got.Ref != "auth-retry-3fa" {
			t.Errorf("resolve = %+v err=%v", got, err)
		}
		if _, err := s.Resolve("fa"); !errors.Is(err, node.ErrTooShort) {
			t.Errorf("err = %v", err)
		}
		if _, err := s.Resolve("zzz"); !errors.Is(err, node.ErrNotFound) {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("A parent reference resolves by hash suffix", func(t *testing.T) {
		r, root := repo(t)
		write(t, root, ".tektite/nodes/retry-plan-9de.md", nodeSrc(idA, "plan", "Plan", ""))
		write(t, root, ".tektite/nodes/ticket-3fa.md", nodeSrc(idB, "ticket", "T", "old-plan-name-9de"))
		s, _ := r.Load()
		tk, _ := s.Get("ticket-3fa")
		if tk.ParentRef != "retry-plan-9de" || tk.ParentErr != nil || s.Tree.Parents["ticket-3fa"] != "retry-plan-9de" {
			t.Errorf("ticket = %+v", tk)
		}
	})

	t.Run("An unresolved parent is kept and the node becomes a root", func(t *testing.T) {
		r, root := repo(t)
		write(t, root, ".tektite/nodes/ticket-3fa.md", nodeSrc(idA, "ticket", "T", "ghost-000"))
		s, _ := r.Load()
		tk, _ := s.Get("ticket-3fa")
		if !errors.Is(tk.ParentErr, node.ErrNotFound) || tk.ParentRef != "" || tk.Node.Parent != "ghost-000" {
			t.Errorf("ticket = %+v", tk)
		}
		if !reflect.DeepEqual(s.Tree.Roots, []string{"ticket-3fa"}) {
			t.Errorf("roots = %v", s.Tree.Roots)
		}
	})

	t.Run("Events resolve to their node by hash when the directory slug is stale", func(t *testing.T) {
		r, root := repo(t)
		write(t, root, ".tektite/nodes/auth-retry-3fa.md", nodeSrc(idA, "ticket", "T", ""))
		write(t, root, ".tektite/events/old-slug-3fa/01HZ3M0A1B2C3D4E5F6G7H8J91.md", eventSrc("note", "2026-11-27T10:00:00Z"))
		s, _ := r.Load()
		if len(s.Events["auth-retry-3fa"]) != 1 || len(s.Orphans) != 0 {
			t.Errorf("events = %v orphans = %v", s.Events, s.Orphans)
		}
	})

	t.Run("An event for a missing node is kept and reported", func(t *testing.T) {
		r, root := repo(t)
		write(t, root, ".tektite/nodes/auth-retry-3fa.md", nodeSrc(idA, "ticket", "T", ""))
		write(t, root, ".tektite/events/ghost-000/01HZ3M0A1B2C3D4E5F6G7H8J91.md", eventSrc("note", "2026-11-27T10:00:00Z"))
		s, _ := r.Load()
		if len(s.Orphans) != 1 || s.Orphans[0].Dir != ".tektite/events/ghost-000" || len(s.Orphans[0].Events) != 1 {
			t.Errorf("orphans = %+v", s.Orphans)
		}
		if len(s.Nodes) != 1 || len(s.Events) != 0 {
			t.Errorf("other nodes affected: %v %v", refs(s), s.Events)
		}
	})
}

func TestTree(t *testing.T) {
	load := func(t *testing.T) *store.Snapshot {
		t.Helper()
		r, root := repo(t)
		write(t, root, ".tektite/nodes/spec-d3f.md", nodeSrc(idA, "spec", "S", ""))
		write(t, root, ".tektite/nodes/plan-9e1.md", nodeSrc(idB, "plan", "P", "spec-d3f"))
		write(t, root, ".tektite/nodes/tb-7c1.md", nodeSrc(idC, "ticket", "B", "plan-9e1"))
		write(t, root, ".tektite/nodes/ta-5d2.md", nodeSrc(idD, "ticket", "A", "plan-9e1"))
		s, err := r.Load()
		if err != nil {
			t.Fatal(err)
		}
		return s
	}

	t.Run("Nodes nest to any depth", func(t *testing.T) {
		s := load(t)
		got, cyc := s.Tree.Ancestors("tb-7c1")
		if cyc || !reflect.DeepEqual(got, []string{"plan-9e1", "spec-d3f"}) {
			t.Errorf("ancestors = %v cycle=%v", got, cyc)
		}
	})

	t.Run("Descendants are listed in tree order", func(t *testing.T) {
		s := load(t)
		want := []store.Visit{{Ref: "plan-9e1", Depth: 1}, {Ref: "ta-5d2", Depth: 2}, {Ref: "tb-7c1", Depth: 2}}
		if got := s.Tree.Descendants("spec-d3f"); !reflect.DeepEqual(got, want) {
			t.Errorf("descendants = %v", got)
		}
	})

	t.Run("A cycle in parents is reported and not followed", func(t *testing.T) {
		r, root := repo(t)
		write(t, root, ".tektite/nodes/a-d3f.md", nodeSrc(idA, "plan", "A", "b-9e1"))
		write(t, root, ".tektite/nodes/b-9e1.md", nodeSrc(idB, "plan", "B", "a-d3f"))
		write(t, root, ".tektite/nodes/c-7c1.md", nodeSrc(idC, "plan", "C", ""))
		s, err := r.Load()
		if err != nil {
			t.Fatal(err)
		}
		if want := [][]string{{"a-d3f", "b-9e1"}}; !reflect.DeepEqual(s.Tree.Cycles, want) {
			t.Errorf("cycles = %v", s.Tree.Cycles)
		}
		if len(s.Tree.Descendants("a-d3f")) != 1 {
			t.Errorf("descendants = %v", s.Tree.Descendants("a-d3f"))
		}
		if _, ok := s.Get("a-d3f"); !ok {
			t.Error("cyclic node must stay readable")
		}
	})
}

func TestWriteNode(t *testing.T) {
	t.Run("A new node file is written atomically", func(t *testing.T) {
		r, root := repo(t)
		path, err := r.WriteNode("add-retry-3fa", []byte("hello"))
		if err != nil || path != ".tektite/nodes/add-retry-3fa.md" {
			t.Fatalf("path=%q err=%v", path, err)
		}
		got, err := os.ReadFile(filepath.Join(root, ".tektite", "nodes", "add-retry-3fa.md"))
		if err != nil || string(got) != "hello" {
			t.Errorf("content=%q err=%v", got, err)
		}
		entries, _ := os.ReadDir(r.NodesDir())
		if len(entries) != 1 {
			t.Errorf("leftover files: %v", entries)
		}
	})

	t.Run("An existing node file is never overwritten", func(t *testing.T) {
		r, root := repo(t)
		write(t, root, ".tektite/nodes/add-retry-3fa.md", "original")
		_, err := r.WriteNode("add-retry-3fa", []byte("new"))
		if !errors.Is(err, store.ErrExists) {
			t.Fatalf("err = %v", err)
		}
		got, _ := os.ReadFile(filepath.Join(root, ".tektite", "nodes", "add-retry-3fa.md"))
		if string(got) != "original" {
			t.Errorf("content = %q", got)
		}
		entries, _ := os.ReadDir(r.NodesDir())
		if len(entries) != 1 {
			t.Errorf("leftover files: %v", entries)
		}
	})

	t.Run("An unsafe reference is rejected", func(t *testing.T) {
		r, root := repo(t)
		for _, ref := range []string{"", "../x", "a/b", `a\b`, ".hidden"} {
			if _, err := r.WriteNode(ref, []byte("x")); !errors.Is(err, store.ErrBadRef) {
				t.Errorf("%q: err = %v", ref, err)
			}
		}
		if _, err := os.Stat(filepath.Join(root, ".tektite", "nodes")); !os.IsNotExist(err) {
			t.Errorf("nodes dir must not be created: %v", err)
		}
	})
}

func TestWriteEvent(t *testing.T) {
	t.Run("A new event file is never overwritten", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, ".tektite/nodes/a-3fa.md", nodeSrc("01HZ3K8Q5W2X9N4T7V6B1C0D3F", "ticket", "A", ""))
		r, err := store.Open(root)
		if err != nil {
			t.Fatal(err)
		}
		id := "01HZ3M0A1B2C3D4E5F6G7H8J91"
		path, err := r.WriteEvent("a-3fa", id, []byte("first"))
		if err != nil || path != ".tektite/events/a-3fa/"+id+".md" {
			t.Fatalf("path = %q err = %v", path, err)
		}
		if _, err := r.WriteEvent("a-3fa", id, []byte("second")); !errors.Is(err, store.ErrExists) {
			t.Errorf("err = %v", err)
		}
		data, err := os.ReadFile(filepath.Join(root, path))
		if err != nil || string(data) != "first" {
			t.Errorf("data = %q err = %v", data, err)
		}
		entries, _ := os.ReadDir(filepath.Join(root, ".tektite/events/a-3fa"))
		if len(entries) != 1 {
			t.Errorf("leftover files: %v", entries)
		}
		if _, err := r.WriteEvent("../x", id, nil); !errors.Is(err, store.ErrBadRef) {
			t.Errorf("bad ref: %v", err)
		}
	})
}
