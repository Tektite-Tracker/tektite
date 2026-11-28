package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type checkDoc struct {
	Problems []struct {
		Code    string `json:"code"`
		Path    string `json:"path"`
		Message string `json:"message"`
	} `json:"problems"`
}

func staleCodes(t *testing.T, s *sandbox, args ...string) []string {
	t.Helper()
	res := s.tektite(append([]string{"check", "--json"}, args...)...)
	var doc checkDoc
	decode(t, res.stdout, &doc)
	out := []string{}
	for _, p := range doc.Problems {
		if p.Code == "TKT-W115" {
			out = append(out, p.Message)
		}
	}
	return out
}

func TestCLI_Stale(t *testing.T) {
	t.Run("Evidence can be checked for staleness", func(t *testing.T) {
		s := evidenceSandbox(t)
		prov := "provenance:\n  actor: human:jesus\n  commit: 3f2a9c1\n"
		s.node("recorded-shot-5de", idB, "evidence", "auth-retry-3fa", "evidences: [auth-retry-3fa]\nclaim: recorded\nfiles: [src/retry.go]\n"+prov, "")
		s.node("untouched-shot-7aa", idC, "evidence", "auth-retry-3fa", "evidences: [auth-retry-3fa]\nclaim: untouched\n"+prov, "")
		s.node("no-commit-shot-9bb", e1, "evidence", "auth-retry-3fa", "evidences: [auth-retry-3fa]\nclaim: no commit\nprovenance:\n  actor: human:jesus\n", "")
		calls := map[string][]string{}
		s.commits = func(since string, paths []string) []string {
			calls[since] = append(calls[since], strings.Join(paths, ","))
			if slices.Equal(paths, []string{"src/retry.go"}) {
				return []string{"a1b2c3d", "e4f5a6b"}
			}
			return nil
		}

		if got := staleCodes(t, s); len(got) != 0 || len(calls) != 0 {
			t.Fatalf("check without --stale must not read history, got %v and %v", got, calls)
		}
		got := staleCodes(t, s, "--stale")
		if len(got) != 1 || !strings.Contains(got[0], "recorded-shot-5de") || !strings.Contains(got[0], "a1b2c3d, e4f5a6b") || !strings.Contains(got[0], "2 later commits") {
			t.Fatalf("want one stale report naming the evidence and its commits, got %v", got)
		}
		if !slices.Contains(calls["3f2a9c1"], "src/retry.go") || !slices.Contains(calls["3f2a9c1"], "") || len(calls) != 1 {
			t.Errorf("related files should be the listed files, or none, and only evidence with a commit is looked up: %v", calls)
		}
	})
}

func gcSandbox(t *testing.T) (*sandbox, string, string) {
	s := evidenceSandbox(t)
	kept, lost := strings.Repeat("a", 64)+".txt", strings.Repeat("b", 64)+".txt"
	s.write(".tektite/blobs/"+kept, "kept")
	s.write(".tektite/blobs/"+lost, "lost")
	s.node("retry-shot-5de", idB, "evidence", "auth-retry-3fa", "evidences: [auth-retry-3fa]\nclaim: shot\nattachments: [blobs/"+kept+"]\nprovenance:\n  commit: 3f2a9c1\n", "")
	return s, kept, lost
}

func exists(s *sandbox, name string) bool {
	_, err := os.Stat(filepath.Join(s.root, ".tektite", "blobs", name))
	return err == nil
}

func TestCLI_GC(t *testing.T) {
	t.Run("Unreferenced blobs are removed by gc", func(t *testing.T) {
		s, kept, lost := gcSandbox(t)
		res := s.tektite("gc", "--blobs", "--dry-run", "--json")
		var doc gcResult
		decode(t, res.stdout, &doc)
		if res.code != 0 || !doc.DryRun || !slices.Equal(doc.Removed, []string{lost}) || !exists(s, lost) {
			t.Fatalf("dry run should list %s and keep it: %+v exit %d", lost, doc, res.code)
		}
		res = s.tektite("gc", "--blobs")
		if res.code != 0 || !strings.Contains(res.stdout, "removed blobs/"+lost) {
			t.Fatalf("exit %d: %s%s", res.code, res.stdout, res.stderr)
		}
		if exists(s, lost) || !exists(s, kept) {
			t.Errorf("only the unreferenced blob should be gone: lost=%v kept=%v", exists(s, lost), exists(s, kept))
		}
	})

	t.Run("Gc needs a target", func(t *testing.T) {
		s, _, lost := gcSandbox(t)
		res := s.tektite("gc")
		if res.code != 2 || !exists(s, lost) {
			t.Errorf("want exit 2 and nothing removed, got %d, blob present %v", res.code, exists(s, lost))
		}
	})
}
