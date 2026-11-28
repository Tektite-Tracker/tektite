package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/tektite-tracker/tektite/parse"
)

const blockBody = "Intro text.\n\n- [ ] handle timeouts\n- [ ] <!-- t:KEEP0001 --> already has one\n\n```\n- [ ] inside a fence\n```\n\n## Gaps\n\n- [x] missing retry\n\nClosing text."

func blocksOf(t *testing.T, s *sandbox) []parse.Block {
	t.Helper()
	n, _ := parse.ParseNode("auth-retry-3fa.md", []byte(s.read(".tektite/nodes/auth-retry-3fa.md")))
	return n.Blocks
}

func TestCLI_FmtBlockIDs(t *testing.T) {
	t.Run("Formatting assigns ids to checklist blocks", func(t *testing.T) {
		s := newSandbox(t)
		s.node("auth-retry-3fa", idA, "ticket", "", "", blockBody)
		res := s.tektite("fmt", "--json")
		if res.code != 0 || !strings.Contains(res.stdout, "assign block ids") {
			t.Fatalf("exit %d: %s%s", res.code, res.stdout, res.stderr)
		}
		got := s.read(".tektite/nodes/auth-retry-3fa.md")
		if !regexp.MustCompile(`(?m)^- \[ \] <!-- t:[0-9A-Z]{8} --> handle timeouts$`).MatchString(got) ||
			!regexp.MustCompile(`(?m)^- \[x\] <!-- g:[0-9A-Z]{8} --> missing retry$`).MatchString(got) {
			t.Errorf("ids were not assigned as expected:\n%s", got)
		}
		for _, want := range []string{"- [ ] <!-- t:KEEP0001 --> already has one", "```\n- [ ] inside a fence\n```", "Intro text.", "Closing text."} {
			if !strings.Contains(got, want) {
				t.Errorf("the body lost %q:\n%s", want, got)
			}
		}
		blocks := blocksOf(t, s)
		if len(blocks) != 3 {
			t.Fatalf("want 3 blocks, got %+v", blocks)
		}
		for _, b := range blocks {
			if b.ID == "" {
				t.Errorf("block %q has no id", b.Text)
			}
		}
		after := s.read(".tektite/nodes/auth-retry-3fa.md")
		if again := s.tektite("fmt", "--json"); again.code != 0 || strings.TrimSpace(again.stdout) != "[]" || s.read(".tektite/nodes/auth-retry-3fa.md") != after {
			t.Errorf("a second run must change nothing: %s", again.stdout)
		}
	})

	t.Run("A dry run shows block ids without writing them", func(t *testing.T) {
		s := newSandbox(t)
		s.node("auth-retry-3fa", idA, "ticket", "", "", "- [ ] handle timeouts")
		before := s.read(".tektite/nodes/auth-retry-3fa.md")
		res := s.tektite("fmt", "--dry-run")
		if res.code != 0 || !strings.Contains(res.stdout, "would update") || !strings.Contains(res.stdout, "assign block ids") {
			t.Fatalf("exit %d: %s%s", res.code, res.stdout, res.stderr)
		}
		if s.read(".tektite/nodes/auth-retry-3fa.md") != before {
			t.Errorf("the file must not be written")
		}
	})
}
