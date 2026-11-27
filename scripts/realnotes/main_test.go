package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRealNotes(t *testing.T) {
	t.Run("No path means the validation is skipped", func(t *testing.T) {
		var out, errw bytes.Buffer
		if code := run(nil, &out, &errw); code != 0 || !strings.Contains(out.String(), "skipping") {
			t.Errorf("code=%d out=%q", code, out.String())
		}
	})

	t.Run("A folder of notes is summarised without leaking content", func(t *testing.T) {
		bin := filepath.Join(t.TempDir(), "tkt")
		build := exec.Command("go", "build", "-o", bin, "github.com/tektite-tracker/tektite/cmd/tkt")
		if msg, err := build.CombinedOutput(); err != nil {
			t.Fatalf("build: %v\n%s", err, msg)
		}
		dir := t.TempDir()
		write := func(rel, content string) {
			path := filepath.Join(dir, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		write("Zeta Secret Name.md", "# Zebra Heading\nsee [[Quokka Target]] and [[ghost-000]]\n- [ ] task\n")
		write("sub/plain.md", "---\nid: 01HZ3K8Q5W2X9N4T7V6B1C0D3F\nkind: ticket\ntitle: Marmot\nparent: \"\"\n---\nbody\n")
		write("broken.md", "---\ntitle: [oops\n---\ntext\n")
		write(".obsidian/ignored.md", "# hidden\n")
		write("notes.txt", "not markdown")

		var out, errw bytes.Buffer
		if code := run([]string{"--tkt", bin, dir}, &out, &errw); code != 0 {
			t.Fatalf("code=%d err=%s", code, errw.String())
		}
		got := out.String()
		for _, want := range []string{"files: 3", "skipped hidden dirs: 1", "parser panics: 0", "TKT-E001: 1", "ls nodes: 3", "check exit 1: 1", "wikilink shaped like a title or path: 1", "show exit 0: 3"} {
			if !strings.Contains(got, want) {
				t.Errorf("missing %q in\n%s", want, got)
			}
		}
		for _, secret := range []string{"Zeta", "Secret", "Zebra", "Quokka", "Marmot", "broken", "plain", "sub", dir} {
			if strings.Contains(got, secret) {
				t.Errorf("output leaks %q", secret)
			}
		}
		left, _ := filepath.Glob(filepath.Join(os.TempDir(), "realnotes-*"))
		if len(left) != 0 {
			t.Errorf("temporary copies left behind: %d", len(left))
		}
	})
}
