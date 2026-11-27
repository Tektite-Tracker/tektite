package main

import (
	"strings"
	"testing"
)

type gapsDoc struct {
	Gaps []struct {
		Block    string `json:"block"`
		Closed   bool   `json:"closed"`
		Evidence string `json:"evidence"`
	} `json:"gaps"`
}

func gapSandbox(t *testing.T, exitCode string) *sandbox {
	s := evidenceSandbox(t)
	s.node("auth-retry-3fa", idA, "ticket", "", "", "- [ ] <!-- g:01HZ3KB2 --> missing retry on 429\n- [ ] <!-- t:01HZ3KB3 --> plain item\n- [ ] <!-- g:01HZ3KB4 --> second gap")
	s.node("retry-banner-shot-5de", idB, "evidence", "auth-retry-3fa", "evidences: [auth-retry-3fa]\nclaim: retries on 429\nprovenance:\n  actor: human:jesus\n  commit: 3f2a9c1\n  exit_code: "+exitCode+"\n", "")
	return s
}

func TestCLI_CloseGap(t *testing.T) {
	t.Run("Evidence can close a gap block", func(t *testing.T) {
		s := gapSandbox(t, "0")
		res := s.tkt("close-gap", "auth-retry-3fa#01HZ3KB2", "--evidence", "retry-banner-shot-5de", "--json")
		if res.code != 0 {
			t.Fatalf("exit %d: %s", res.code, res.stderr)
		}
		var doc appended
		decode(t, res.stdout, &doc)
		if doc.Type != "close" {
			t.Fatalf("want a close event, got %q", doc.Type)
		}
		event := s.read(doc.Path)
		for _, want := range []string{"block: 01HZ3KB2", "evidence: retry-banner-shot-5de"} {
			if !strings.Contains(event, want) {
				t.Errorf("close event lacks %q:\n%s", want, event)
			}
		}
		var shown gapsDoc
		decode(t, s.tkt("show", "auth-retry-3fa", "--json").stdout, &shown)
		states := map[string]bool{}
		for _, g := range shown.Gaps {
			states[g.Block] = g.Closed
			if g.Block == "01HZ3KB2" && g.Evidence != "retry-banner-shot-5de" {
				t.Errorf("the closed gap should name its evidence, got %q", g.Evidence)
			}
		}
		if !states["01HZ3KB2"] || states["01HZ3KB4"] {
			t.Errorf("only the first gap should be closed, got %v", states)
		}
		if text := s.tkt("show", "auth-retry-3fa").stdout; !strings.Contains(text, "gap 01HZ3KB2: missing retry on 429 (closed by retry-banner-shot-5de)") {
			t.Errorf("text output should show the gap as closed:\n%s", text)
		}
	})

	t.Run("Failing evidence cannot close a gap block", func(t *testing.T) {
		s := gapSandbox(t, "1")
		res := s.tkt("close-gap", "auth-retry-3fa#01HZ3KB2", "--evidence", "retry-banner-shot-5de")
		if res.code != 2 {
			t.Fatalf("want exit 2, got %d: %s", res.code, res.stderr)
		}
		if files := eventFiles(t, s, "auth-retry-3fa"); len(files) != 0 {
			t.Errorf("no event should be appended, found %v", files)
		}
	})

	t.Run("Only an open gap block of the node can be closed", func(t *testing.T) {
		s := gapSandbox(t, "0")
		if res := s.tkt("close-gap", "auth-retry-3fa#01HZ3KB2", "--evidence", "retry-banner-shot-5de"); res.code != 0 {
			t.Fatalf("first close: exit %d: %s", res.code, res.stderr)
		}
		before := len(eventFiles(t, s, "auth-retry-3fa"))
		for _, block := range []string{"01HZ3KB2", "01HZ3KB3", "01HZ3KBZ"} {
			res := s.tkt("close-gap", "auth-retry-3fa#"+block, "--evidence", "retry-banner-shot-5de")
			if res.code != 2 {
				t.Errorf("%s: want exit 2, got %d: %s", block, res.code, res.stderr)
			}
		}
		if after := len(eventFiles(t, s, "auth-retry-3fa")); after != before {
			t.Errorf("no event should be appended, %d became %d", before, after)
		}
	})
}
