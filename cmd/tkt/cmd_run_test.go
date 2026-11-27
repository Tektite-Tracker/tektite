package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func (s *sandbox) attachRun(command string, extra ...string) (attachDoc, result) {
	s.t.Helper()
	args := append([]string{"attach-evidence", "auth-retry-3fa", "--json", "--claim", "the command passes", "--run", command}, extra...)
	res := s.tkt(args...)
	var doc attachDoc
	if res.code == 0 {
		decode(s.t, res.stdout, &doc)
	}
	return doc, res
}

func (s *sandbox) evidenceNode(ref string) string {
	s.t.Helper()
	return s.read(".tektite/nodes/" + ref + ".md")
}

func TestCLI_AttachRun(t *testing.T) {
	t.Run("A command output is stored as an inline fenced block", func(t *testing.T) {
		s := evidenceSandbox(t)
		doc, res := s.attachRun("echo hello; exit 3")
		if res.code != 0 {
			t.Fatalf("exit %d: %s", res.code, res.stderr)
		}
		node := s.evidenceNode(doc.Evidence)
		for _, want := range []string{"```\nhello\n```", "exit_code: 3", "command: echo hello; exit 3"} {
			if !strings.Contains(node, want) {
				t.Errorf("evidence node lacks %q:\n%s", want, node)
			}
		}
		if doc.Blob != "" || doc.Stored {
			t.Errorf("an inline output should not store a blob, got %q stored=%v", doc.Blob, doc.Stored)
		}
	})

	t.Run("Long output is stored as a blob and summarised", func(t *testing.T) {
		s := evidenceSandbox(t)
		doc, res := s.attachRun("seq 1 2000")
		if res.code != 0 {
			t.Fatalf("exit %d: %s", res.code, res.stderr)
		}
		if !doc.Stored || doc.Blob == "" {
			t.Fatalf("expected a stored blob, got %+v", doc)
		}
		full, err := os.ReadFile(filepath.Join(s.root, filepath.FromSlash(doc.Blob)))
		if err != nil || !strings.HasPrefix(string(full), "1\n2\n") || !strings.HasSuffix(string(full), "2000\n") {
			t.Fatalf("blob does not hold the full output: %v", err)
		}
		node := s.evidenceNode(doc.Evidence)
		if !strings.Contains(node, "1981\n") || !strings.Contains(node, "2000\n") || strings.Contains(node, "\n1980\n") {
			t.Errorf("body should hold exactly the last 20 lines:\n%s", node)
		}
		if !strings.Contains(node, "blobs/"+filepath.Base(doc.Blob)) {
			t.Errorf("body should name the blob path:\n%s", node)
		}
	})

	t.Run("Secrets are not captured from the environment", func(t *testing.T) {
		t.Setenv("TKT_TEST_SECRET", "hunter2-secret")
		s := evidenceSandbox(t)
		doc, res := s.attachRun("echo done")
		if res.code != 0 {
			t.Fatalf("exit %d: %s", res.code, res.stderr)
		}
		node := s.evidenceNode(doc.Evidence)
		if strings.Contains(node, "hunter2-secret") || strings.Contains(node, "env") {
			t.Errorf("the environment leaked into the evidence node:\n%s", node)
		}
	})

	t.Run("Run cannot be combined with a file or a command", func(t *testing.T) {
		s := evidenceSandbox(t)
		for _, extra := range [][]string{{"--file", "out.txt"}, {"--command", "other"}} {
			_, res := s.attachRun("echo hi", extra...)
			if res.code != 2 {
				t.Errorf("%v: want exit 2, got %d", extra, res.code)
			}
		}
		if entries, _ := os.ReadDir(filepath.Join(s.root, ".tektite", "nodes")); len(entries) != 1 {
			t.Errorf("no evidence node should be created, found %d nodes", len(entries))
		}
	})
}

func TestCLI_Rerun(t *testing.T) {
	t.Run("Evidence can be re-run", func(t *testing.T) {
		s := evidenceSandbox(t)
		first, res := s.attachRun("echo again")
		if res.code != 0 {
			t.Fatalf("exit %d: %s", res.code, res.stderr)
		}
		res = s.tkt("rerun", first.Evidence, "--json")
		if res.code != 0 {
			t.Fatalf("exit %d: %s", res.code, res.stderr)
		}
		var second attachDoc
		decode(t, res.stdout, &second)
		if second.Evidence == first.Evidence || second.Parent != "auth-retry-3fa" {
			t.Fatalf("expected a new node under the same parent, got %+v", second)
		}
		node := s.evidenceNode(second.Evidence)
		for _, want := range []string{"supersedes:", first.Evidence, "claim: the command passes", "```\nagain\n```"} {
			if !strings.Contains(node, want) {
				t.Errorf("new evidence lacks %q:\n%s", want, node)
			}
		}
	})

	t.Run("Evidence without a command cannot be re-run", func(t *testing.T) {
		s := evidenceSandbox(t)
		res := s.tktIn(s.root, "some output", "attach-evidence", "auth-retry-3fa", "--file", "-", "--claim", "it works", "--json")
		var doc attachDoc
		decode(t, res.stdout, &doc)
		before, _ := os.ReadDir(filepath.Join(s.root, ".tektite", "nodes"))
		res = s.tkt("rerun", doc.Evidence)
		if res.code != 2 {
			t.Fatalf("want exit 2, got %d: %s", res.code, res.stderr)
		}
		after, _ := os.ReadDir(filepath.Join(s.root, ".tektite", "nodes"))
		if len(after) != len(before) {
			t.Errorf("a node was created")
		}
	})
}
