package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type attachDoc struct {
	Evidence string   `json:"evidence"`
	Blob     string   `json:"blob"`
	Hash     string   `json:"hash"`
	Stored   bool     `json:"stored"`
	Parent   string   `json:"parent"`
	Event    string   `json:"event"`
	Commit   string   `json:"commit"`
	Warnings []string `json:"warnings"`
}

type showEvidence struct {
	Evidence []struct {
		Ref         string   `json:"ref"`
		Claim       string   `json:"claim"`
		Commit      string   `json:"commit"`
		Dirty       bool     `json:"dirty"`
		Command     string   `json:"command"`
		ExitCode    *int     `json:"exit_code"`
		Attachments []string `json:"attachments"`
	} `json:"evidence"`
	History []struct {
		Type string `json:"type"`
	} `json:"history"`
}

func sha(data string) string {
	sum := sha256.Sum256([]byte(data))
	return hex.EncodeToString(sum[:])
}

func evidenceSandbox(t *testing.T) *sandbox {
	s := newSandbox(t)
	s.node("auth-retry-3fa", idA, "ticket", "", "", "x")
	s.head = "3f2a9c1d4e5f60718293a4b5c6d7e8f901234567"
	s.env = map[string]string{"TKT_ACTOR": "agent:claude-opus/s-4f2a"}
	return s
}

func (s *sandbox) attach(extra ...string) (attachDoc, result) {
	s.t.Helper()
	res := s.tektite(append([]string{"attach-evidence", "auth-retry-3fa", "--json"}, extra...)...)
	var doc attachDoc
	if res.code == 0 {
		decode(s.t, res.stdout, &doc)
	}
	return doc, res
}

func (s *sandbox) file(name, content string) string {
	s.write(name, content)
	return filepath.Join(s.root, name)
}

func TestCLI_Evidence(t *testing.T) {
	t.Run("Storing an attachment", func(t *testing.T) {
		s := evidenceSandbox(t)
		path := s.file("shot.png", "png bytes")
		doc, res := s.attach("--file", path, "--claim", "retry banner shows")
		if res.code != 0 {
			t.Fatalf("result = %+v", res)
		}
		blob := ".tektite/blobs/" + sha("png bytes") + ".png"
		if doc.Blob != blob || s.read(blob) != "png bytes" || !doc.Stored {
			t.Errorf("doc = %+v", doc)
		}
		src := s.read(".tektite/nodes/" + doc.Evidence + ".md")
		for _, want := range []string{"kind: evidence", "parent: auth-retry-3fa", "claim: retry banner shows", "blobs/" + sha("png bytes") + ".png", "commit: 3f2a9c1\n"} {
			if !strings.Contains(src, want) {
				t.Errorf("missing %q in\n%s", want, src)
			}
		}
	})

	t.Run("Attaching evidence to a node", func(t *testing.T) {
		s := evidenceSandbox(t)
		doc, _ := s.attach("--file", s.file("shot.png", "x"), "--claim", "retry banner shows")
		var show showEvidence
		decode(t, s.tektite("show", "auth-retry-3fa", "--json").stdout, &show)
		if len(show.Evidence) != 1 || show.Evidence[0].Ref != doc.Evidence || show.Evidence[0].Claim != "retry banner shows" {
			t.Errorf("evidence = %+v", show.Evidence)
		}
		attached := false
		for _, h := range show.History {
			attached = attached || h.Type == "attach"
		}
		if !attached {
			t.Errorf("no attach event in %+v", show.History)
		}
		if out := s.tektite("show", "auth-retry-3fa"); !strings.Contains(out.stdout, "evidence: "+doc.Evidence+"  retry banner shows  @3f2a9c1") {
			t.Errorf("show = %s", out.stdout)
		}
	})

	t.Run("Identical files are stored once", func(t *testing.T) {
		s := evidenceSandbox(t)
		path := s.file("shot.png", "same bytes")
		first, _ := s.attach("--file", path, "--claim", "first claim")
		second, _ := s.attach("--file", path, "--claim", "second claim")
		entries, _ := os.ReadDir(filepath.Join(s.root, ".tektite", "blobs"))
		if len(entries) != 1 || first.Blob != second.Blob || first.Evidence == second.Evidence || !first.Stored || second.Stored {
			t.Errorf("entries = %v first = %+v second = %+v", entries, first, second)
		}
	})

	t.Run("A blob never changes", func(t *testing.T) {
		s := evidenceSandbox(t)
		path := s.file("shot.png", "original")
		doc, _ := s.attach("--file", path, "--claim", "a claim")
		before, _ := os.Stat(filepath.Join(s.root, doc.Blob))
		s.write("shot.png", "original")
		s.attach("--file", path, "--claim", "again")
		s.tektite("check")
		s.tektite("context", "auth-retry-3fa")
		after, _ := os.Stat(filepath.Join(s.root, doc.Blob))
		if !before.ModTime().Equal(after.ModTime()) || s.read(doc.Blob) != "original" {
			t.Errorf("blob was rewritten")
		}
	})

	t.Run("A tampered blob is detected", func(t *testing.T) {
		s := evidenceSandbox(t)
		doc, _ := s.attach("--file", s.file("log.txt", "honest log"), "--claim", "a claim")
		s.write(doc.Blob, "edited log")
		res := s.tektite("check")
		if res.code != exitError || !strings.Contains(res.stdout, "error TKT-E103") || !strings.Contains(res.stdout, sha("honest log")) || !strings.Contains(res.stdout, doc.Evidence) {
			t.Errorf("check = %+v", res)
		}
	})

	t.Run("A missing blob is reported", func(t *testing.T) {
		s := evidenceSandbox(t)
		doc, _ := s.attach("--file", s.file("log.txt", "log"), "--claim", "a claim")
		if err := os.Remove(filepath.Join(s.root, doc.Blob)); err != nil {
			t.Fatal(err)
		}
		res := s.tektite("check")
		if res.code != 0 || !strings.Contains(res.stdout, "warning TKT-W114") || !strings.Contains(res.stdout, doc.Evidence+".md") || !strings.Contains(res.stdout, doc.Blob) {
			t.Errorf("check = %+v", res)
		}
		if show := s.tektite("show", doc.Evidence); show.code != 0 {
			t.Errorf("show = %+v", show)
		}
	})

	t.Run("Large files are warned about", func(t *testing.T) {
		s := evidenceSandbox(t)
		s.write(".tektite/config", "blob_cap_mb: 1\n")
		path := s.file("big.bin", strings.Repeat("x", 2<<20))
		res := s.tektite("attach-evidence", "auth-retry-3fa", "--file", path, "--claim", "big one")
		if res.code != exitUsage || !strings.Contains(res.stderr, "warning:") || !strings.Contains(res.stderr, "--allow-large") {
			t.Fatalf("result = %+v", res)
		}
		if entries, _ := os.ReadDir(filepath.Join(s.root, ".tektite", "blobs")); len(entries) != 0 {
			t.Errorf("blob stored despite refusal: %v", entries)
		}
		doc, ok := s.attach("--file", path, "--claim", "big one", "--allow-large")
		if ok.code != 0 || len(doc.Warnings) == 0 || !strings.Contains(doc.Warnings[0], "cap") {
			t.Errorf("result = %+v doc = %+v", ok, doc)
		}
	})

	t.Run("Unused blobs are listed, not deleted", func(t *testing.T) {
		s := evidenceSandbox(t)
		stray := ".tektite/blobs/" + sha("stray") + ".txt"
		s.write(stray, "stray")
		res := s.tektite("check")
		if res.code != 0 || !strings.Contains(res.stdout, "note TKT-N103") || !strings.Contains(res.stdout, sha("stray")) || s.read(stray) != "stray" {
			t.Errorf("check = %+v", res)
		}
	})

	t.Run("Provenance is captured with the evidence", func(t *testing.T) {
		s := evidenceSandbox(t)
		s.attach("--file", s.file("out.txt", "ok"), "--claim", "tests pass", "--command", "go test ./...", "--exit-code", "0")
		var show showEvidence
		decode(t, s.tektite("show", "auth-retry-3fa", "--json").stdout, &show)
		ev := show.Evidence[0]
		if ev.Command != "go test ./..." || ev.ExitCode == nil || *ev.ExitCode != 0 || ev.Commit != "3f2a9c1" || ev.Claim != "tests pass" {
			t.Errorf("evidence = %+v", ev)
		}
		src := s.read(".tektite/nodes/" + ev.Ref + ".md")
		if !strings.Contains(src, "actor: agent:claude-opus/s-4f2a") || !strings.Contains(src, "at: ") {
			t.Errorf("provenance missing actor or time:\n%s", src)
		}
	})

	t.Run("The commit SHA is captured from the repository", func(t *testing.T) {
		s := evidenceSandbox(t)
		doc, _ := s.attach("--file", s.file("a.txt", "a"), "--claim", "a claim")
		if doc.Commit != "3f2a9c1" || len(doc.Warnings) != 0 {
			t.Errorf("doc = %+v", doc)
		}
	})

	t.Run("A dirty tree is recorded", func(t *testing.T) {
		s := evidenceSandbox(t)
		s.dirty = true
		doc, _ := s.attach("--file", s.file("a.txt", "a"), "--claim", "a claim")
		if src := s.read(".tektite/nodes/" + doc.Evidence + ".md"); !strings.Contains(src, "dirty: true") {
			t.Errorf("source = %s", src)
		}
	})

	t.Run("Evidence without a commit SHA is flagged", func(t *testing.T) {
		s := evidenceSandbox(t)
		s.head = ""
		doc, res := s.attach("--file", s.file("a.txt", "a"), "--claim", "a claim")
		if res.code != 0 || len(doc.Warnings) != 1 || !strings.Contains(doc.Warnings[0], "TKT-W113") {
			t.Errorf("result = %+v doc = %+v", res, doc)
		}
		out := s.tektite("check")
		if out.code != 0 || !strings.Contains(out.stdout, "warning TKT-W113") || !strings.Contains(out.stdout, doc.Evidence+".md") || !strings.Contains(out.stdout, "code version") {
			t.Errorf("check = %+v", out)
		}
	})

	t.Run("Evidence outside a repository has no commit", func(t *testing.T) {
		s := evidenceSandbox(t)
		s.head = ""
		doc, _ := s.attach("--file", s.file("a.txt", "a"), "--claim", "a claim")
		if src := s.read(".tektite/nodes/" + doc.Evidence + ".md"); strings.Contains(src, "commit:") {
			t.Errorf("source = %s", src)
		}
	})

	t.Run("Evidence can be attached from stdin", func(t *testing.T) {
		s := evidenceSandbox(t)
		res := s.tektiteIn(s.root, "go test output\nok\n", "attach-evidence", "auth-retry-3fa", "--file", "-", "--claim", "tests pass", "--command", "go test ./...", "--json")
		var doc attachDoc
		decode(t, res.stdout, &doc)
		if res.code != 0 || !strings.HasSuffix(doc.Blob, ".txt") || s.read(doc.Blob) != "go test output\nok\n" {
			t.Errorf("result = %+v", res)
		}
	})

	t.Run("Attaching needs a claim and a file", func(t *testing.T) {
		s := evidenceSandbox(t)
		if res := s.tektite("attach-evidence", "auth-retry-3fa", "--file", s.file("a.txt", "a")); res.code != exitUsage {
			t.Errorf("no claim = %+v", res)
		}
		if res := s.tektite("attach-evidence", "auth-retry-3fa", "--claim", "x"); res.code != exitUsage {
			t.Errorf("no file = %+v", res)
		}
		if res := s.tektite("attach-evidence", "ghost-000", "--file", s.file("a.txt", "a"), "--claim", "x"); res.code != exitNotFound {
			t.Errorf("unknown node = %+v", res)
		}
	})

	t.Run("Evidence attaches to a node through its parent and an evidences link", func(t *testing.T) {
		s := evidenceSandbox(t)
		s.node("by-parent-5de", idB, "evidence", "auth-retry-3fa", "claim: via parent\n", "x")
		s.node("by-link-5df", idC, "evidence", "", "evidences: [auth-retry-3fa#01HZ3KB2]\nclaim: via link\n", "x")
		var show showEvidence
		decode(t, s.tektite("show", "auth-retry-3fa", "--json").stdout, &show)
		got := []string{}
		for _, e := range show.Evidence {
			got = append(got, e.Ref)
		}
		if len(got) != 2 {
			t.Errorf("evidence = %v", got)
		}
		var links showLinks
		decode(t, s.tektite("show", "auth-retry-3fa", "--json").stdout, &links)
		found := false
		for _, b := range links.Backlinks {
			found = found || (b.Rel == "evidenced-by" && b.Ref == "by-link-5df")
		}
		if !found {
			t.Errorf("backlinks = %+v", links.Backlinks)
		}
	})

	t.Run("Evidence in the context bundle uses real attachments, recent first", func(t *testing.T) {
		s := evidenceSandbox(t)
		s.now = s.now.Add(-2 * 3600e9)
		s.attach("--file", s.file("old.txt", "old log"), "--claim", "older claim", "--command", "make old", "--exit-code", "1")
		s.now = s.now.Add(2 * 3600e9)
		s.attach("--file", s.file("new.png", "binary"), "--claim", "newer claim")
		doc, _ := s.context("auth-retry-3fa")
		if len(doc.Evidence) != 2 || !strings.Contains(doc.Evidence[0].Text, "Claim: newer claim") || !strings.Contains(doc.Evidence[1].Text, "Claim: older claim") {
			t.Fatalf("evidence = %+v", doc.Evidence)
		}
		newer := doc.Evidence[0].Text
		if !strings.Contains(newer, "blobs/"+sha("binary")+".png (6 bytes, content not included)") || strings.Contains(newer, "binary\n") {
			t.Errorf("text = %s", newer)
		}
		if older := doc.Evidence[1].Text; !strings.Contains(older, "Command: make old") || !strings.Contains(older, "Exit code: 1") {
			t.Errorf("text = %s", older)
		}
	})
}
