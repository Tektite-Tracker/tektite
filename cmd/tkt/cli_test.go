package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tektite-tracker/tektite/internal/store"
)

type result struct {
	code   int
	stdout string
	stderr string
}

type sandbox struct {
	t    *testing.T
	root string
	rng  io.Reader
	now  time.Time

	interactive bool
	editor      string
	env         map[string]string
	git         string
}

func newSandbox(t *testing.T) *sandbox {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".tektite"), 0o755); err != nil {
		t.Fatal(err)
	}
	return &sandbox{t: t, root: root, rng: rand.New(rand.NewSource(7)), now: time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)}
}

func (s *sandbox) write(rel, content string) {
	s.t.Helper()
	path := filepath.Join(s.root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		s.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		s.t.Fatal(err)
	}
}

func (s *sandbox) read(rel string) string {
	s.t.Helper()
	data, err := os.ReadFile(filepath.Join(s.root, filepath.FromSlash(rel)))
	if err != nil {
		s.t.Fatal(err)
	}
	return string(data)
}

func (s *sandbox) tkt(args ...string) result {
	return s.tktIn(s.root, "", args...)
}

func (s *sandbox) tktIn(cwd, stdin string, args ...string) result {
	s.t.Helper()
	var out, errw bytes.Buffer
	code := run(args, env{
		in: strings.NewReader(stdin), out: &out, err: &errw, cwd: cwd,
		now: func() time.Time { return s.now }, rand: s.rng, interactive: s.interactive,
		getenv: func(k string) string {
			if k == "EDITOR" {
				return s.editor
			}
			return s.env[k]
		},
		gitUser: func() string { return s.git },
	})
	return result{code: code, stdout: out.String(), stderr: errw.String()}
}

func (s *sandbox) node(ref, id, kind, parent, extra, body string) {
	s.write(".tektite/nodes/"+ref+".md", "---\nid: "+id+"\nkind: "+kind+"\ntitle: Title of "+ref+"\nparent: "+parent+"\n"+extra+"---\n"+body+"\n")
}

func (s *sandbox) event(ref, id, typ, at, extra string) {
	s.write(".tektite/events/"+ref+"/"+id+".md", "---\ntype: "+typ+"\nactor: human:jesus\nat: "+at+"\n"+extra+"---\n")
}

const (
	idA = "01HZ3K8Q5W2X9N4T7V6B1C0D3F"
	idB = "01HZ3K8Q5W2X9N4T7V6B1C0D9E"
	idC = "01HZ3K8Q5W2X9N4T7V6B1C0D7C"
	e1  = "01HZ3M0A1B2C3D4E5F6G7H8J91"
)

func decode(t *testing.T, s string, v any) {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(s))
	if err := dec.Decode(v); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, s)
	}
	if dec.More() {
		t.Fatalf("stdout holds more than one JSON document:\n%s", s)
	}
}

func TestCLI_ExitCodes(t *testing.T) {
	t.Run("Exit codes", func(t *testing.T) {
		s := newSandbox(t)
		s.node("auth-retry-3f1", idA, "ticket", "", "", "x")
		s.node("a-a3fa", idB, "ticket", "", "", "x")
		s.node("b-b3fa", idC, "ticket", "", "", "x")
		cases := []struct {
			name string
			args []string
			want int
		}{
			{"success", []string{"ls"}, exitOK},
			{"invalid usage", []string{"show"}, exitUsage},
			{"unknown command", []string{"frobnicate"}, exitUsage},
			{"unknown flag", []string{"ls", "--nope"}, exitUsage},
			{"too short a hash", []string{"show", "fa"}, exitUsage},
			{"ambiguous input", []string{"show", "3fa"}, exitUsage},
			{"a node that is not found", []string{"show", "ghost-000"}, exitNotFound},
		}
		for _, c := range cases {
			if got := s.tkt(c.args...); got.code != c.want {
				t.Errorf("%s: code = %d, want %d (stderr %q)", c.name, got.code, c.want, got.stderr)
			}
		}
		s.write(".tektite/nodes/bad-9e1.md", "---\ntitle: [x\n---\n")
		if got := s.tkt("check"); got.code != exitError {
			t.Errorf("check with errors: code = %d", got.code)
		}
		for err, want := range map[error]int{store.ErrExists: exitConflict, store.ErrNoRepo: exitNotFound, errors.New("boom"): exitError} {
			if got := classify(err).code; got != want {
				t.Errorf("%v: code = %d, want %d", err, got, want)
			}
		}
		if exitPolicy != 5 {
			t.Error("policy violations exit with 5")
		}
	})

	t.Run("Unknown commands and flags are usage errors", func(t *testing.T) {
		s := newSandbox(t)
		for _, args := range [][]string{{"frobnicate"}, {"ls", "--nope"}, {"ls", "extra"}} {
			got := s.tkt(args...)
			if got.code != exitUsage || got.stdout != "" || got.stderr == "" {
				t.Errorf("%v: %+v", args, got)
			}
		}
	})

	t.Run("Errors go to stderr, data goes to stdout", func(t *testing.T) {
		s := newSandbox(t)
		got := s.tkt("show", "ghost-000")
		if got.code != exitNotFound || got.stdout != "" || !strings.Contains(got.stderr, "ghost-000") || !strings.Contains(got.stderr, "hint:") {
			t.Errorf("result = %+v", got)
		}
	})
}

func TestCLI_Output(t *testing.T) {
	setup := func(t *testing.T) *sandbox {
		s := newSandbox(t)
		s.node("auth-hardening-7c1", idC, "plan", "", "", "The plan.")
		s.node("auth-retry-3fa", idA, "ticket", "auth-hardening-7c1", "labels: [net]\n", "Back off on 429, see [[auth-hardening-7c1]] and [[ghost-000]].")
		s.event("auth-retry-3fa", e1, "set-status", "2026-11-27T10:00:00Z", "status: active\n")
		return s
	}

	t.Run("Human text is the default", func(t *testing.T) {
		s := setup(t)
		got := s.tkt("show", "auth-retry-3fa")
		for _, want := range []string{"auth-retry-3fa  ticket  active", "Title of auth-retry-3fa", "parent: auth-hardening-7c1", "under: auth-hardening-7c1", "Back off on 429", "human:jesus  set-status", "ghost-000 (unresolved)"} {
			if !strings.Contains(got.stdout, want) {
				t.Errorf("missing %q in\n%s", want, got.stdout)
			}
		}
		if got.code != 0 || strings.Contains(got.stdout, "\x1b[") {
			t.Errorf("result = %+v", got)
		}
	})

	t.Run("JSON output is stable", func(t *testing.T) {
		s := setup(t)
		got := s.tkt("show", "3fa", "--json")
		var doc map[string]any
		decode(t, got.stdout, &doc)
		for _, k := range []string{"ref", "id", "kind", "title", "parent", "status", "path", "fields", "body", "ancestors", "children", "links", "history", "diagnostics"} {
			if _, ok := doc[k]; !ok {
				t.Errorf("missing field %q", k)
			}
		}
		if doc["ref"] != "auth-retry-3fa" || doc["status"] != "active" || doc["parent"] != "auth-hardening-7c1" {
			t.Errorf("doc = %v", doc)
		}
		var list []map[string]any
		decode(t, s.tkt("ls", "--json").stdout, &list)
		if len(list) != 2 || list[1]["status"] != "active" {
			t.Errorf("list = %v", list)
		}
		var empty []any
		decode(t, newSandbox(t).tkt("ls", "--json").stdout, &empty)
		if empty == nil || len(empty) != 0 {
			t.Errorf("empty list = %v", empty)
		}
	})

	t.Run("JSON errors", func(t *testing.T) {
		s := setup(t)
		got := s.tkt("show", "ghost-000", "--json")
		var doc map[string]any
		decode(t, got.stdout, &doc)
		if got.code != 3 || doc["error"] == "" || doc["code"] != float64(3) || doc["hint"] == "" {
			t.Errorf("result = %+v doc = %v", got, doc)
		}
		bad := s.tkt("ls", "--nope", "--json")
		decode(t, bad.stdout, &doc)
		if bad.code != 2 || doc["code"] != float64(2) {
			t.Errorf("usage error: %+v", bad)
		}
	})

	t.Run("No colour or prompts when not a terminal", func(t *testing.T) {
		s := setup(t)
		for _, args := range [][]string{{"ls"}, {"show", "3fa"}, {"check"}, {"new", "ticket", "Plain"}} {
			if got := s.tkt(args...); strings.Contains(got.stdout+got.stderr, "\x1b") || got.code != 0 {
				t.Errorf("%v: %+v", args, got)
			}
		}
	})
}

func TestCLI_Repository(t *testing.T) {
	t.Run("The root is found by walking upwards", func(t *testing.T) {
		s := newSandbox(t)
		s.node("auth-retry-3fa", idA, "ticket", "", "", "x")
		deep := filepath.Join(s.root, "src", "deep")
		if err := os.MkdirAll(deep, 0o755); err != nil {
			t.Fatal(err)
		}
		got := s.tktIn(deep, "", "ls")
		if got.code != 0 || !strings.Contains(got.stdout, "auth-retry-3fa") {
			t.Errorf("result = %+v", got)
		}
	})

	t.Run("No repository", func(t *testing.T) {
		s := newSandbox(t)
		got := s.tktIn(t.TempDir(), "", "ls")
		if got.code != exitNotFound || !strings.Contains(got.stderr, "tkt init") {
			t.Errorf("result = %+v", got)
		}
	})

	t.Run("An explicit root overrides discovery", func(t *testing.T) {
		s := newSandbox(t)
		s.node("auth-retry-3fa", idA, "ticket", "", "", "x")
		got := s.tktIn(t.TempDir(), "", "--root", s.root, "ls")
		if got.code != 0 || !strings.Contains(got.stdout, "auth-retry-3fa") {
			t.Errorf("result = %+v", got)
		}
		if got := s.tktIn(t.TempDir(), "", "--root", t.TempDir(), "ls"); got.code != exitNotFound {
			t.Errorf("missing root: %+v", got)
		}
	})

	t.Run("Initialising", func(t *testing.T) {
		dir := t.TempDir()
		s := &sandbox{t: t, root: dir, rng: rand.New(rand.NewSource(1)), now: time.Now()}
		first := s.tkt("init")
		if first.code != 0 || !strings.Contains(first.stdout, "initialised") {
			t.Fatalf("first = %+v", first)
		}
		for _, d := range []string{"nodes", "events", "blobs"} {
			if info, err := os.Stat(filepath.Join(dir, ".tektite", d)); err != nil || !info.IsDir() {
				t.Errorf("%s missing: %v", d, err)
			}
		}
		if _, err := os.Stat(filepath.Join(dir, ".tektite", "config")); err != nil {
			t.Errorf("config missing: %v", err)
		}
		s.write(".tektite/config", "auto_commit: true\n")
		second := s.tkt("init", "--json")
		var doc map[string]any
		decode(t, second.stdout, &doc)
		if second.code != 0 || doc["created"] != false || s.read(".tektite/config") != "auto_commit: true\n" {
			t.Errorf("second = %+v", second)
		}
	})
}

func TestCLI_New(t *testing.T) {
	t.Run("Creating a node", func(t *testing.T) {
		s := newSandbox(t)
		s.node("retry-plan-9de", idC, "plan", "", "", "plan")
		got := s.tkt("new", "ticket", "Add retry on 429", "--parent", "retry-plan-9de")
		ref := strings.TrimSpace(got.stdout)
		if got.code != 0 || !strings.HasPrefix(ref, "add-retry-on-429-") {
			t.Fatalf("result = %+v", got)
		}
		src := s.read(".tektite/nodes/" + ref + ".md")
		id := ""
		for _, line := range strings.Split(src, "\n") {
			if v, ok := strings.CutPrefix(line, "id: "); ok {
				id = v
			}
		}
		if len(id) != 26 || !strings.HasSuffix(strings.ToLower(id), strings.TrimPrefix(ref, "add-retry-on-429-")) {
			t.Errorf("id = %q ref = %q", id, ref)
		}
		for _, want := range []string{"kind: ticket\n", "title: Add retry on 429\n", "parent: retry-plan-9de\n"} {
			if !strings.Contains(src, want) {
				t.Errorf("missing %q in\n%s", want, src)
			}
		}
		if again := s.tkt("check"); again.code != 0 || !strings.Contains(again.stdout, "0 errors") {
			t.Errorf("check = %+v", again)
		}
		var doc map[string]any
		decode(t, s.tkt("new", "ticket", "Another", "--json").stdout, &doc)
		if doc["ref"] == "" || doc["path"] == "" || doc["id"] == "" {
			t.Errorf("doc = %v", doc)
		}
	})

	t.Run("A new node stores its parent as a full reference", func(t *testing.T) {
		s := newSandbox(t)
		s.node("retry-plan-9de", idC, "plan", "", "", "plan")
		got := s.tkt("new", "ticket", "Add retry", "--parent", "9de")
		ref := strings.TrimSpace(got.stdout)
		if got.code != 0 || !strings.Contains(s.read(".tektite/nodes/"+ref+".md"), "parent: retry-plan-9de\n") {
			t.Errorf("result = %+v", got)
		}
		before, _ := os.ReadDir(filepath.Join(s.root, ".tektite", "nodes"))
		bad := s.tkt("new", "ticket", "Orphan", "--parent", "ghost-000")
		after, _ := os.ReadDir(filepath.Join(s.root, ".tektite", "nodes"))
		if bad.code != exitNotFound || len(before) != len(after) {
			t.Errorf("bad parent: %+v", bad)
		}
	})

	t.Run("The body can come from stdin or an editor", func(t *testing.T) {
		s := newSandbox(t)
		got := s.tktIn(s.root, "text from stdin\nsecond line\n", "new", "ticket", "Add retry", "--body", "-")
		ref := strings.TrimSpace(got.stdout)
		if got.code != 0 || !strings.HasSuffix(s.read(".tektite/nodes/"+ref+".md"), "---\ntext from stdin\nsecond line\n") {
			t.Errorf("result = %+v\n%s", got, s.read(".tektite/nodes/"+ref+".md"))
		}

		script := filepath.Join(t.TempDir(), "editor.sh")
		if err := os.WriteFile(script, []byte("#!/bin/sh\n{ cat \"$1\"; printf 'from editor\\n'; } > \"$1.new\" && mv \"$1.new\" \"$1\"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		s.interactive, s.editor = true, script
		edited := s.tkt("new", "ticket", "Edited")
		eref := strings.TrimSpace(edited.stdout)
		if edited.code != 0 || !strings.HasSuffix(s.read(".tektite/nodes/"+eref+".md"), "from editor\n") || !strings.Contains(s.read(".tektite/nodes/"+eref+".md"), "### Context") {
			t.Errorf("editor result = %+v\n%s", edited, s.read(".tektite/nodes/"+eref+".md"))
		}
		explicit := s.tkt("new", "ticket", "Explicit", "--body", "given")
		xref := strings.TrimSpace(explicit.stdout)
		if strings.Contains(s.read(".tektite/nodes/"+xref+".md"), "from editor") {
			t.Error("--body must skip the editor")
		}
		s.interactive = false
		piped := s.tkt("new", "ticket", "Piped")
		pref := strings.TrimSpace(piped.stdout)
		if strings.Contains(s.read(".tektite/nodes/"+pref+".md"), "from editor") {
			t.Error("a non-terminal must not open the editor")
		}
	})

	t.Run("The template seeds a new body", func(t *testing.T) {
		s := newSandbox(t)
		s.write(".tektite/schemas/ticket.md", "---\nkind: ticket\n---\n## Template\n### Context\n### Acceptance criteria\n")
		ref := strings.TrimSpace(s.tkt("new", "ticket", "Add retry").stdout)
		if !strings.HasSuffix(s.read(".tektite/nodes/"+ref+".md"), "---\n### Context\n### Acceptance criteria\n") {
			t.Errorf("node:\n%s", s.read(".tektite/nodes/"+ref+".md"))
		}
		if err := os.Remove(filepath.Join(s.root, ".tektite", "schemas", "ticket.md")); err != nil {
			t.Fatal(err)
		}
		builtIn := strings.TrimSpace(s.tkt("new", "ticket", "Built in").stdout)
		if !strings.Contains(s.read(".tektite/nodes/"+builtIn+".md"), "### Acceptance criteria") {
			t.Errorf("built-in template missing:\n%s", s.read(".tektite/nodes/"+builtIn+".md"))
		}
		custom := strings.TrimSpace(s.tkt("new", "retro", "Friday").stdout)
		if !strings.HasSuffix(s.read(".tektite/nodes/"+custom+".md"), "---\n") {
			t.Errorf("custom kind must have an empty body:\n%s", s.read(".tektite/nodes/"+custom+".md"))
		}
	})

	t.Run("Explicit body overrides the template", func(t *testing.T) {
		s := newSandbox(t)
		s.write(".tektite/schemas/ticket.md", "---\nkind: ticket\n---\n## Template\n### Context\n")
		ref := strings.TrimSpace(s.tkt("new", "ticket", "Add retry", "--body", "short").stdout)
		src := s.read(".tektite/nodes/" + ref + ".md")
		if !strings.HasSuffix(src, "---\nshort\n") || strings.Contains(src, "Context") {
			t.Errorf("node:\n%s", src)
		}
	})

	t.Run("A required field is requested at creation", func(t *testing.T) {
		s := newSandbox(t)
		s.write(".tektite/schemas/ticket.md", "---\nkind: ticket\nrequired: [estimate]\n---\n")
		got := s.tkt("new", "ticket", "Add retry")
		if got.code != 0 || strings.TrimSpace(got.stdout) == "" || !strings.Contains(got.stderr, `"estimate" is required`) || !strings.Contains(got.stderr, "--set estimate=") {
			t.Errorf("result = %+v", got)
		}
		var doc map[string]any
		decode(t, s.tkt("new", "ticket", "Again", "--json").stdout, &doc)
		if m, _ := doc["missing_required"].([]any); len(m) != 1 || m[0] != "estimate" {
			t.Errorf("doc = %v", doc)
		}
		ok := s.tkt("new", "ticket", "With estimate", "--set", "estimate=3")
		if ok.stderr != "" {
			t.Errorf("no note expected: %+v", ok)
		}
	})

	t.Run("Setting fields at creation", func(t *testing.T) {
		s := newSandbox(t)
		ref := strings.TrimSpace(s.tkt("new", "ticket", "Add retry", "--set", "estimate=3", "--set", "labels=[net, auth]", "--set", "owner=team a").stdout)
		src := s.read(".tektite/nodes/" + ref + ".md")
		for _, want := range []string{"estimate: 3\n", "labels:\n  - net\n  - auth\n", "owner: team a\n"} {
			if !strings.Contains(src, want) {
				t.Errorf("missing %q in\n%s", want, src)
			}
		}
		for _, bad := range []string{"estimate", "id=x", "9bad=1", "title=x"} {
			if got := s.tkt("new", "ticket", "X", "--set", bad); got.code != exitUsage {
				t.Errorf("--set %s: %+v", bad, got)
			}
		}
		if got := s.tkt("new", "ticket", "  "); got.code != exitUsage {
			t.Errorf("empty title: %+v", got)
		}
	})

	t.Run("A title with no ASCII letters or digits gets a hash-only reference", func(t *testing.T) {
		s := newSandbox(t)
		ref := strings.TrimSpace(s.tkt("new", "note", "日本語").stdout)
		if len(ref) < 3 || strings.Contains(ref, "-") || strings.Contains(ref, "node") {
			t.Errorf("ref = %q", ref)
		}
		if got := s.tkt("show", ref); got.code != 0 || !strings.Contains(got.stdout, "日本語") {
			t.Errorf("show = %+v", got)
		}
	})
}

func TestCLI_List(t *testing.T) {
	t.Run("Listing with filters", func(t *testing.T) {
		s := newSandbox(t)
		s.node("auth-hardening-7c1", idC, "plan", "", "", "p")
		s.node("auth-retry-3fa", idA, "ticket", "auth-hardening-7c1", "", "t")
		s.node("auth-timeout-9e1", idB, "ticket", "auth-hardening-7c1", "", "t")
		s.node("other-ticket-5d2", "01HZ3K8Q5W2X9N4T7V6B1C0D5D", "ticket", "", "", "t")
		s.event("auth-timeout-9e1", e1, "set-status", "2026-11-27T10:00:00Z", "status: done\n")
		refs := func(args ...string) []string {
			var list []map[string]string
			decode(t, s.tkt(append([]string{"ls", "--json"}, args...)...).stdout, &list)
			out := []string{}
			for _, it := range list {
				out = append(out, it["ref"])
			}
			return out
		}
		cases := []struct {
			args []string
			want string
		}{
			{nil, "auth-hardening-7c1,auth-retry-3fa,auth-timeout-9e1,other-ticket-5d2"},
			{[]string{"--kind", "ticket"}, "auth-retry-3fa,auth-timeout-9e1,other-ticket-5d2"},
			{[]string{"--status", "open"}, "auth-hardening-7c1,auth-retry-3fa,other-ticket-5d2"},
			{[]string{"--status", "done"}, "auth-timeout-9e1"},
			{[]string{"--under", "7c1"}, "auth-retry-3fa,auth-timeout-9e1"},
			{[]string{"--kind", "ticket", "--status", "open", "--under", "auth-hardening-7c1"}, "auth-retry-3fa"},
		}
		for _, c := range cases {
			if got := strings.Join(refs(c.args...), ","); got != c.want {
				t.Errorf("%v: got %s, want %s", c.args, got, c.want)
			}
		}
		if got := s.tkt("ls", "--under", "ghost-000"); got.code != exitNotFound {
			t.Errorf("unknown --under: %+v", got)
		}
	})

	t.Run("Duplicate ids are reported and both nodes stay readable", func(t *testing.T) {
		s := newSandbox(t)
		s.node("one-d3f", idA, "spec", "", "", "x")
		s.node("two-9e1", idA, "spec", "", "", "y")
		ls := s.tkt("ls")
		if !strings.Contains(ls.stdout, "one-d3f") || !strings.Contains(ls.stdout, "two-9e1") {
			t.Errorf("ls = %+v", ls)
		}
		chk := s.tkt("check")
		if chk.code != exitError || !strings.Contains(chk.stdout, "TKT-E101") {
			t.Errorf("check = %+v", chk)
		}
	})

	t.Run("Renaming a title does not change identity or the file name", func(t *testing.T) {
		s := newSandbox(t)
		ref := strings.TrimSpace(s.tkt("new", "ticket", "Add retry on 429").stdout)
		rel := ".tektite/nodes/" + ref + ".md"
		before := s.read(rel)
		s.write(rel, strings.Replace(before, "title: Add retry on 429", "title: Add backoff on 429", 1))
		got := s.tkt("show", ref)
		if !strings.Contains(got.stdout, "Add backoff on 429") || !strings.HasPrefix(got.stdout, ref+"  ") {
			t.Errorf("show = %+v", got)
		}
		if fm := s.tkt("fmt"); fm.code != 0 {
			t.Errorf("fmt = %+v", fm)
		}
		entries, _ := os.ReadDir(filepath.Join(s.root, ".tektite", "nodes"))
		after := s.read(rel)
		idOf := func(src string) string { return strings.SplitN(strings.SplitN(src, "id: ", 2)[1], "\n", 2)[0] }
		if len(entries) != 1 || entries[0].Name() != ref+".md" || idOf(before) != idOf(after) {
			t.Errorf("entries = %v\n%s", entries, after)
		}
		if again := s.tkt("show", ref[len(ref)-3:]); !strings.HasPrefix(again.stdout, ref+"  ") {
			t.Errorf("hash lookup = %+v", again)
		}
	})
}

func TestCLI_Check(t *testing.T) {
	t.Run("Checking the repository", func(t *testing.T) {
		s := newSandbox(t)
		s.node("tk-d3f", idA, "ticket", "ghost-000", "blocks: [nowhere-111]\n", "x")
		got := s.tkt("check")
		if got.code != 0 {
			t.Fatalf("warnings only must exit 0: %+v", got)
		}
		for _, want := range []string{".tektite/nodes/tk-d3f.md: warning TKT-W103", "hint: point parent", "0 errors, 2 warnings, 0 notes"} {
			if !strings.Contains(got.stdout, want) {
				t.Errorf("missing %q in\n%s", want, got.stdout)
			}
		}
		s.write(".tektite/nodes/bad-9e1.md", "---\ntitle: [x\n---\n")
		bad := s.tkt("check")
		if bad.code != exitError || !strings.Contains(bad.stdout, "bad-9e1.md:") || !strings.Contains(bad.stdout, "error TKT-E001") || bad.stderr != "" {
			t.Errorf("bad = %+v", bad)
		}
	})

	t.Run("Notes alone do not fail the check", func(t *testing.T) {
		s := newSandbox(t)
		s.write(".tektite/nodes/plain-3fa.md", "# Heading\ntext\n")
		s.write(".tektite/nodes/partial-9e1.md", "---\ntitle: Partial\n---\nbody\n")
		got := s.tkt("check")
		if got.code != 0 || !strings.Contains(got.stdout, "note TKT-W002") || !strings.Contains(got.stdout, "0 errors, 0 warnings, 3 notes") {
			t.Errorf("text = %+v", got)
		}
		js := s.tkt("check", "--json")
		var rep struct {
			Problems []struct {
				Code     string `json:"code"`
				Severity string `json:"severity"`
			} `json:"problems"`
			Notes int `json:"notes"`
		}
		decode(t, js.stdout, &rep)
		if js.code != 0 || rep.Notes != 3 || len(rep.Problems) != 3 {
			t.Fatalf("json = %+v rep = %+v", js, rep)
		}
		for _, p := range rep.Problems {
			if p.Severity != "note" {
				t.Errorf("problem = %+v", p)
			}
		}
	})

	t.Run("Check output as JSON", func(t *testing.T) {
		s := newSandbox(t)
		s.node("tk-d3f", idA, "ticket", "ghost-000", "", "x")
		s.write(".tektite/nodes/bad-9e1.md", "---\ntitle: [x\n---\n")
		got := s.tkt("check", "--json")
		var rep struct {
			Problems []map[string]any `json:"problems"`
			Errors   int              `json:"errors"`
			Warnings int              `json:"warnings"`
		}
		decode(t, got.stdout, &rep)
		if got.code != exitError || rep.Errors != 1 || rep.Warnings != 1 || len(rep.Problems) != 3 {
			t.Fatalf("result = %+v rep = %+v", got, rep)
		}
		for _, p := range rep.Problems {
			for _, k := range []string{"severity", "code", "path", "line", "message"} {
				if _, ok := p[k]; !ok {
					t.Errorf("problem %v lacks %q", p, k)
				}
			}
		}
	})
}

func TestCLI_Fmt(t *testing.T) {
	const noID = "---\nkind: ticket\ntitle: Messy\nparent: \"\"\nreviewed_by:   jesus\nlabels: [a,   b]\n---\n\n# Body  \n  keep   this\n"

	t.Run("A missing id is assigned by tkt fmt", func(t *testing.T) {
		s := newSandbox(t)
		s.write(".tektite/nodes/messy-3fa.md", noID)
		got := s.tkt("fmt")
		src := s.read(".tektite/nodes/messy-3fa.md")
		if got.code != 0 || !strings.Contains(got.stdout, "updated .tektite/nodes/messy-3fa.md: assign id") {
			t.Fatalf("result = %+v", got)
		}
		if !strings.HasPrefix(src, "---\nid: 01") || !strings.HasSuffix(src, "---\n\n# Body  \n  keep   this\n") {
			t.Errorf("node:\n%s", src)
		}
		if chk := s.tkt("check"); strings.Contains(chk.stdout, "TKT-W002") {
			t.Errorf("id still missing: %s", chk.stdout)
		}
	})

	t.Run("Unknown frontmatter fields are preserved", func(t *testing.T) {
		s := newSandbox(t)
		s.write(".tektite/nodes/messy-3fa.md", noID)
		s.tkt("fmt")
		src := s.read(".tektite/nodes/messy-3fa.md")
		for _, want := range []string{"reviewed_by: jesus\n", "labels: [a, b]\n"} {
			if !strings.Contains(src, want) {
				t.Errorf("missing %q in\n%s", want, src)
			}
		}
		if i, j := strings.Index(src, "reviewed_by"), strings.Index(src, "labels"); i > j || strings.Index(src, "parent") > i {
			t.Errorf("field order changed:\n%s", src)
		}
	})

	t.Run("Formatting is idempotent and skips broken files", func(t *testing.T) {
		s := newSandbox(t)
		s.write(".tektite/nodes/messy-3fa.md", noID)
		s.write(".tektite/nodes/plain-9e1.md", "# Just a heading\ntext\n")
		s.write(".tektite/nodes/broken-7c1.md", "---\ntitle: [x\n---\nbody\n")
		first := s.tkt("fmt")
		if !strings.Contains(first.stdout, "skipped .tektite/nodes/broken-7c1.md") || !strings.Contains(first.stdout, "add frontmatter") {
			t.Errorf("first = %+v", first)
		}
		if s.read(".tektite/nodes/broken-7c1.md") != "---\ntitle: [x\n---\nbody\n" {
			t.Error("broken file must stay untouched")
		}
		snapshot := s.read(".tektite/nodes/messy-3fa.md") + s.read(".tektite/nodes/plain-9e1.md")
		second := s.tkt("fmt")
		if strings.Contains(second.stdout, "updated") || snapshot != s.read(".tektite/nodes/messy-3fa.md")+s.read(".tektite/nodes/plain-9e1.md") {
			t.Errorf("second = %+v", second)
		}
		plain := s.read(".tektite/nodes/plain-9e1.md")
		if !strings.Contains(plain, "kind: note\ntitle: Just a heading\n") || !strings.HasSuffix(plain, "---\n# Just a heading\ntext\n") {
			t.Errorf("plain:\n%s", plain)
		}
	})

	t.Run("Formatting keeps the values, styles and comments of fields it does not own", func(t *testing.T) {
		s := newSandbox(t)
		s.write(".tektite/nodes/dated-3fa.md", "---\ntitle: Invented\ndate: 2024-01-05\ncreated: 2024-01-05 10:30\n# keep this comment\nquoted: 'single'\nzip: \"007\"\ntags:\n  - alpha\n  - beta\nmulti: |\n  line one\n  line two\nsummary: \"A long invented summary that goes well past eighty characters so that any line folding would be visible here\"\n---\nbody\n")
		got := s.tkt("fmt")
		src := s.read(".tektite/nodes/dated-3fa.md")
		if got.code != 0 || strings.Contains(got.stdout, "skipped") {
			t.Fatalf("result = %+v", got)
		}
		for _, want := range []string{"date: 2024-01-05\n", "created: 2024-01-05 10:30\n", "# keep this comment\n", "quoted: 'single'\n", "zip: \"007\"\n", "tags:\n  - alpha\n  - beta\n", "multi: |\n  line one\n  line two\n", "summary: \"A long invented summary that goes well past eighty characters so that any line folding would be visible here\"\n"} {
			if !strings.Contains(src, want) {
				t.Errorf("missing %q in\n%s", want, src)
			}
		}
		if !strings.HasPrefix(src, "---\nid: 01") || !strings.HasSuffix(src, "---\nbody\n") {
			t.Errorf("node:\n%s", src)
		}
		again := s.tkt("fmt")
		if strings.Contains(again.stdout, "updated") || s.read(".tektite/nodes/dated-3fa.md") != src {
			t.Errorf("second run changed the file: %+v", again)
		}
	})

	t.Run("A file whose values would change is skipped and left untouched", func(t *testing.T) {
		s := newSandbox(t)
		const dangling = "---\ntitle: &t\ncopy: *t\n---\nbody\n"
		s.write(".tektite/nodes/dangling-3fa.md", dangling)
		got := s.tkt("fmt")
		if got.code != 0 || !strings.Contains(got.stdout, "skipped .tektite/nodes/dangling-3fa.md") || s.read(".tektite/nodes/dangling-3fa.md") != dangling {
			t.Errorf("result = %+v", got)
		}
	})

	t.Run("Formatting", func(t *testing.T) {
		s := newSandbox(t)
		s.write(".tektite/nodes/messy-3fa.md", noID)
		dry := s.tkt("fmt", "--dry-run")
		if dry.code != 0 || !strings.Contains(dry.stdout, "would update .tektite/nodes/messy-3fa.md") || s.read(".tektite/nodes/messy-3fa.md") != noID {
			t.Errorf("dry run = %+v", dry)
		}
		var items []map[string]any
		decode(t, s.tkt("fmt", "--dry-run", "--json").stdout, &items)
		if len(items) != 1 || items[0]["written"] != false {
			t.Errorf("items = %v", items)
		}
		s.tkt("fmt")
		if s.read(".tektite/nodes/messy-3fa.md") == noID {
			t.Error("fmt must write")
		}
	})
}

func TestCLI_Schema(t *testing.T) {
	t.Run("The built-in kinds", func(t *testing.T) {
		s := newSandbox(t)
		s.write(".tektite/schemas/ticket.md", "---\nkind: ticket\nrequired: [estimate]\n---\n")
		got := s.tkt("schema", "ls")
		want := map[string]string{"decision": "built-in", "evidence": "built-in", "gap": "built-in", "plan": "built-in", "spec": "built-in", "ticket": "local"}
		rows := map[string]string{}
		for _, line := range strings.Split(strings.TrimSpace(got.stdout), "\n") {
			f := strings.Fields(line)
			rows[f[0]] = f[1]
		}
		if got.code != 0 || len(rows) != len(want) {
			t.Fatalf("result = %+v", got)
		}
		for k, v := range want {
			if rows[k] != v {
				t.Errorf("%s: source %q, want %q", k, rows[k], v)
			}
		}
		var list []map[string]any
		decode(t, s.tkt("schema", "ls", "--json").stdout, &list)
		if len(list) != 6 || list[5]["kind"] != "ticket" || list[5]["source"] != "local" {
			t.Errorf("list = %v", list)
		}
	})

	t.Run("Unknown keys in a schema are preserved", func(t *testing.T) {
		s := newSandbox(t)
		const schema = "---\nkind:   ticket\nowner: team-a\nrequired: [estimate]\n---\n## Template\n### Context\n"
		s.write(".tektite/schemas/ticket.md", schema)
		s.node("tk-d3f", idA, "ticket", "", "", "x")
		if got := s.tkt("fmt"); got.code != 0 {
			t.Fatalf("fmt = %+v", got)
		}
		if s.read(".tektite/schemas/ticket.md") != schema {
			t.Errorf("schema changed:\n%s", s.read(".tektite/schemas/ticket.md"))
		}
		var list []map[string]any
		decode(t, s.tkt("schema", "ls", "--json").stdout, &list)
		extra, _ := list[5]["extra"].(map[string]any)
		if extra["owner"] != "team-a" {
			t.Errorf("extra = %v", list[5])
		}
	})
}
