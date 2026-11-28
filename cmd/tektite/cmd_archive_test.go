package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var archiveSeq int

func nid() string {
	archiveSeq++
	return fmt.Sprintf("01HZ3K8Q5W2X9N4T7V6B1C%04d", archiveSeq)
}

func eid() string {
	archiveSeq++
	return fmt.Sprintf("01HZ3M0A1B2C3D4E5F6G%06d", archiveSeq)
}

func (s *sandbox) closed(ref, id, status, at string) {
	s.node(ref, id, "ticket", "", "", "x")
	s.event(ref, eid(), "set-status", at, "status: "+status+"\n")
}

func (s *sandbox) exists(rel string) bool {
	_, err := os.Stat(filepath.Join(s.root, filepath.FromSlash(rel)))
	return err == nil
}

type archiveDoc struct {
	Archived []struct {
		Ref    string   `json:"ref"`
		From   string   `json:"from"`
		To     string   `json:"to"`
		Events int      `json:"events"`
		Blobs  []string `json:"blobs"`
	} `json:"archived"`
	Skipped []struct {
		Ref    string `json:"ref"`
		Reason string `json:"reason"`
	} `json:"skipped"`
	DryRun bool `json:"dry_run"`
}

func blobName(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:]) + ".txt"
}

func compact(s string) string {
	return strings.Join(strings.Fields(s), "")
}

func archivedRefs(t *testing.T, res result) []string {
	t.Helper()
	var doc archiveDoc
	decode(t, res.stdout, &doc)
	out := []string{}
	for _, n := range doc.Archived {
		out = append(out, n.Ref)
	}
	return out
}

func TestCLI_Archive(t *testing.T) {
	const at = "2026-11-20T10:00:00Z"
	t.Run("Archiving moves a closed node with its events and blobs", func(t *testing.T) {
		s := newSandbox(t)
		blob := blobName("kept")
		s.write(".tektite/blobs/"+blob, "kept")
		s.closed("done-a-aa1", nid(), "done", at)
		s.node("shot-a-bb2", nid(), "evidence", "done-a-aa1", "evidences: [done-a-aa1]\nclaim: shot\nattachments: [blobs/"+blob+"]\nprovenance:\n  commit: 3f2a9c1\n", "")
		res := s.tektite("archive", "done-a-aa1", "--json")
		if res.code != 0 {
			t.Fatalf("%+v", res)
		}
		var doc archiveDoc
		decode(t, res.stdout, &doc)
		if len(doc.Archived) != 2 || doc.Archived[0].Ref != "done-a-aa1" || doc.Archived[0].Events != 1 || doc.Archived[1].Ref != "shot-a-bb2" || len(doc.Archived[1].Blobs) != 1 {
			t.Fatalf("doc = %+v", doc)
		}
		for _, p := range []string{".tektite/archive/nodes/done-a-aa1.md", ".tektite/archive/nodes/shot-a-bb2.md", ".tektite/archive/events/done-a-aa1", ".tektite/archive/blobs/" + blob} {
			if !s.exists(p) {
				t.Errorf("missing %s", p)
			}
		}
		for _, p := range []string{".tektite/nodes/done-a-aa1.md", ".tektite/nodes/shot-a-bb2.md", ".tektite/events/done-a-aa1", ".tektite/blobs/" + blob} {
			if s.exists(p) {
				t.Errorf("still at %s", p)
			}
		}
	})

	t.Run("A blob that an active node still uses stays in place", func(t *testing.T) {
		s := newSandbox(t)
		blob := blobName("shared")
		s.write(".tektite/blobs/"+blob, "shared")
		s.closed("done-a-aa1", nid(), "done", at)
		s.node("open-a-cc3", nid(), "ticket", "", "", "x")
		ev := "evidences: [%s]\nclaim: shot\nattachments: [blobs/" + blob + "]\nprovenance:\n  commit: 3f2a9c1\n"
		s.node("shot-a-bb2", nid(), "evidence", "done-a-aa1", fmt.Sprintf(ev, "done-a-aa1"), "")
		s.node("shot-b-dd4", nid(), "evidence", "open-a-cc3", fmt.Sprintf(ev, "open-a-cc3"), "")
		if res := s.tektite("archive", "done-a-aa1"); res.code != 0 {
			t.Fatalf("%+v", res)
		}
		if !s.exists(".tektite/blobs/"+blob) || s.exists(".tektite/archive/blobs/"+blob) {
			t.Error("shared blob must stay in .tektite/blobs/")
		}
		if res := s.tektite("check", "--archived"); res.code != 0 || !strings.Contains(res.stdout, "0 errors, 0 warnings") {
			t.Errorf("check = %+v", res)
		}
	})

	t.Run("An open node cannot be archived", func(t *testing.T) {
		s := newSandbox(t)
		for i, st := range []string{"open", "active", "blocked", "review"} {
			ref := fmt.Sprintf("t-%s-a%d1", st, i)
			s.closed(ref, nid(), st, at)
			res := s.tektite("archive", ref)
			if res.code != exitConflict || !strings.Contains(res.stderr, "only done or dropped nodes can be archived") {
				t.Fatalf("%s: %+v", st, res)
			}
			if !s.exists(".tektite/nodes/" + ref + ".md") {
				t.Errorf("%s moved", st)
			}
		}
	})

	t.Run("A node with open children cannot be archived", func(t *testing.T) {
		s := newSandbox(t)
		s.closed("plan-a-aa1", nid(), "done", at)
		s.node("kid-a-bb2", nid(), "ticket", "plan-a-aa1", "", "x")
		res := s.tektite("archive", "plan-a-aa1")
		if res.code != exitConflict || !strings.Contains(res.stderr, "kid-a-bb2") || !s.exists(".tektite/nodes/plan-a-aa1.md") {
			t.Fatalf("%+v", res)
		}
	})

	t.Run("A node that blocks an open node cannot be archived", func(t *testing.T) {
		s := newSandbox(t)
		s.closed("blocker-aa1", nid(), "done", at)
		s.node("waiting-bb2", nid(), "ticket", "", "blocked-by: [blocker-aa1]\n", "x")
		res := s.tektite("archive", "blocker-aa1")
		if res.code != exitConflict || !strings.Contains(res.stderr, "waiting-bb2") || !s.exists(".tektite/nodes/blocker-aa1.md") {
			t.Fatalf("%+v", res)
		}
	})

	t.Run("Archiving several nodes is all or nothing", func(t *testing.T) {
		s := newSandbox(t)
		s.closed("done-a-aa1", nid(), "done", at)
		s.closed("open-a-bb2", nid(), "open", at)
		res := s.tektite("archive", "done-a-aa1", "open-a-bb2")
		if res.code != exitConflict || !s.exists(".tektite/nodes/done-a-aa1.md") || s.exists(".tektite/archive") {
			t.Fatalf("%+v", res)
		}
	})

	t.Run("An archived node is not archived again", func(t *testing.T) {
		s := newSandbox(t)
		s.closed("done-a-aa1", nid(), "done", at)
		s.tektite("archive", "done-a-aa1")
		res := s.tektite("archive", "done-a-aa1")
		if res.code != exitConflict || !strings.Contains(res.stderr, "already archived") {
			t.Fatalf("%+v", res)
		}
	})

	t.Run("Archiving by age takes closed nodes whose last event is old enough", func(t *testing.T) {
		s := newSandbox(t)
		s.closed("old-done-aa1", nid(), "done", "2026-10-10T10:00:00Z")
		s.closed("old-drop-bb2", nid(), "dropped", "2026-10-10T10:00:00Z")
		s.closed("new-done-cc3", nid(), "done", "2026-11-17T10:00:00Z")
		s.closed("old-open-dd4", nid(), "open", "2026-10-10T10:00:00Z")
		got := archivedRefs(t, s.tektite("archive", "--done", "--older-than", "30d", "--json"))
		if strings.Join(got, ",") != "old-done-aa1,old-drop-bb2" {
			t.Fatalf("archived %v", got)
		}
		for _, unit := range []string{"4w", "720h", "1h30m"} {
			if res := s.tektite("archive", "--done", "--older-than", unit, "--dry-run"); res.code != 0 {
				t.Errorf("%s: %+v", unit, res)
			}
		}
		if got := archivedRefs(t, s.tektite("archive", "--done", "--dry-run", "--json")); strings.Join(got, ",") != "new-done-cc3" {
			t.Errorf("without --older-than every closed node is selected, got %v", got)
		}
	})

	t.Run("Bulk archiving skips nodes that are not safe to move", func(t *testing.T) {
		s := newSandbox(t)
		old := "2026-10-10T10:00:00Z"
		s.closed("parent-a-aa1", nid(), "done", old)
		s.node("kid-a-bb2", nid(), "ticket", "parent-a-aa1", "", "x")
		s.closed("blocker-cc3", nid(), "done", old)
		s.node("waiting-dd4", nid(), "ticket", "", "blocked-by: [blocker-cc3]\n", "x")
		s.closed("plain-ee5", nid(), "done", old)
		res := s.tektite("archive", "--done", "--older-than", "30d", "--json")
		if res.code != 0 {
			t.Fatalf("%+v", res)
		}
		var doc archiveDoc
		decode(t, res.stdout, &doc)
		if len(doc.Archived) != 1 || doc.Archived[0].Ref != "plain-ee5" || len(doc.Skipped) != 2 || doc.Skipped[0].Reason == "" {
			t.Fatalf("doc = %+v", doc)
		}
		if text := s.tektite("archive", "--done"); !strings.Contains(text.stdout, "skipped") {
			t.Errorf("plain = %s", text.stdout)
		}
	})

	t.Run("An archive request must name nodes or select them", func(t *testing.T) {
		s := newSandbox(t)
		s.closed("done-a-aa1", nid(), "done", at)
		for _, args := range [][]string{{"archive"}, {"archive", "--older-than", "30d"}, {"archive", "done-a-aa1", "--done"}} {
			if res := s.tektite(args...); res.code != exitUsage {
				t.Errorf("%v: %+v", args, res)
			}
		}
	})

	t.Run("An unknown duration is a usage error", func(t *testing.T) {
		s := newSandbox(t)
		s.closed("done-a-aa1", nid(), "done", at)
		for _, v := range []string{"soon", "-5d", "30"} {
			if res := s.tektite("archive", "--done", "--older-than", v); res.code != exitUsage {
				t.Errorf("%s: %+v", v, res)
			}
		}
	})

	t.Run("A dry run lists what would move", func(t *testing.T) {
		s := newSandbox(t)
		s.closed("done-a-aa1", nid(), "done", at)
		res := s.tektite("archive", "done-a-aa1", "--dry-run", "--json")
		var doc archiveDoc
		decode(t, res.stdout, &doc)
		if res.code != 0 || !doc.DryRun || len(doc.Archived) != 1 || !s.exists(".tektite/nodes/done-a-aa1.md") || s.exists(".tektite/archive") {
			t.Fatalf("%+v %+v", res, doc)
		}
		if plain := s.tektite("archive", "done-a-aa1", "--dry-run"); !strings.Contains(plain.stdout, "would archive done-a-aa1") {
			t.Errorf("plain = %s", plain.stdout)
		}
	})

	t.Run("Empty results are empty lists", func(t *testing.T) {
		s := newSandbox(t)
		res := s.tektite("archive", "--done", "--older-than", "30d", "--json")
		if res.code != 0 || !strings.Contains(compact(res.stdout), `"archived":[]`) || !strings.Contains(compact(res.stdout), `"skipped":[]`) {
			t.Fatalf("%+v", res)
		}
		if res := s.tektite("unarchive", "--help"); res.code != 0 {
			t.Fatalf("%+v", res)
		}
	})
}

func archivedSandbox(t *testing.T) *sandbox {
	t.Helper()
	s := newSandbox(t)
	s.closed("done-a-aa1", nid(), "done", "2026-11-20T10:00:00Z")
	s.node("live-b-bb2", nid(), "ticket", "done-a-aa1", "blocked-by: [done-a-aa1]\nderived-from: [done-a-aa1]\n", "x")
	s.event("live-b-bb2", eid(), "set-status", "2026-11-21T10:00:00Z", "status: done\n")
	s.node("open-c-cc3", nid(), "ticket", "", "derived-from: [done-a-aa1]\n", "x")
	if res := s.tektite("archive", "done-a-aa1"); res.code != 0 {
		t.Fatalf("%+v", res)
	}
	return s
}

func TestCLI_ArchivedNodes(t *testing.T) {
	t.Run("Show and references resolve archived nodes", func(t *testing.T) {
		s := archivedSandbox(t)
		for _, q := range []string{"done-a-aa1", "aa1", "old-slug-aa1"} {
			res := s.tektite("show", q)
			if res.code != 0 || !strings.Contains(res.stdout, "archived: yes") || !strings.Contains(res.stdout, "done-a-aa1") {
				t.Fatalf("%s: %+v", q, res)
			}
		}
		var doc map[string]any
		decode(t, s.tektite("show", "aa1", "--json").stdout, &doc)
		if doc["archived"] != true || doc["status"] != "done" {
			t.Errorf("doc = %v", doc)
		}
		if res := s.tektite("show", "open-c-cc3", "--json"); strings.Contains(compact(res.stdout), `"archived":true`) {
			t.Errorf("active node marked archived: %s", res.stdout)
		}
	})

	t.Run("Archived nodes are left out of ls unless asked", func(t *testing.T) {
		s := archivedSandbox(t)
		refs := func(args ...string) string {
			var list []struct {
				Ref      string `json:"ref"`
				Archived bool   `json:"archived"`
			}
			decode(t, s.tektite(append([]string{"ls", "--json"}, args...)...).stdout, &list)
			out := []string{}
			for _, it := range list {
				tag := it.Ref
				if it.Archived {
					tag += "*"
				}
				out = append(out, tag)
			}
			return strings.Join(out, ",")
		}
		if got := refs(); got != "live-b-bb2,open-c-cc3" {
			t.Errorf("default = %s", got)
		}
		for _, flag := range []string{"--archived", "--all"} {
			if got := refs(flag); got != "done-a-aa1*,live-b-bb2,open-c-cc3" {
				t.Errorf("%s = %s", flag, got)
			}
		}
		if got := refs("--archived", "--status", "done"); got != "done-a-aa1*,live-b-bb2" {
			t.Errorf("combined = %s", got)
		}
		if out := s.tektite("ls", "--archived").stdout; !strings.Contains(out, "(archived)") {
			t.Errorf("plain = %s", out)
		}
	})

	t.Run("Archived nodes are left out of next and context", func(t *testing.T) {
		s := archivedSandbox(t)
		if res := s.tektite("next", "--json"); strings.Contains(res.stdout, "done-a-aa1") {
			t.Errorf("next = %s", res.stdout)
		}
		if res := s.tektite("context", "open-c-cc3"); res.code != 0 || strings.Contains(res.stdout, "done-a-aa1") {
			t.Errorf("context = %+v", res)
		}
		for _, flag := range []string{"--archived", "--all"} {
			if res := s.tektite("context", "open-c-cc3", flag); res.code != 0 || !strings.Contains(res.stdout, "done-a-aa1") {
				t.Errorf("context %s = %+v", flag, res)
			}
		}
		if res := s.tektite("context", "done-a-aa1"); res.code != 0 || !strings.Contains(res.stdout, "done-a-aa1") {
			t.Errorf("context of an archived node = %+v", res)
		}
	})

	t.Run("Check passes with archived nodes and does not break links to them", func(t *testing.T) {
		s := archivedSandbox(t)
		for _, args := range [][]string{{"check"}, {"check", "--archived"}, {"check", "--all"}} {
			res := s.tektite(append(args, "--json")...)
			var doc checkDoc
			decode(t, res.stdout, &doc)
			if res.code != 0 || len(doc.Problems) != 0 {
				t.Errorf("%v: %+v", args, res)
			}
		}
		s.write(".tektite/archive/nodes/broken-z-zz9.md", "---\nid: "+nid()+"\nkind: ticket\ntitle: T\nparent: ghost-000\n---\nx\n")
		if res := s.tektite("check", "--json"); strings.Contains(res.stdout, "broken-z-zz9") {
			t.Errorf("check without --archived looked at archived nodes: %s", res.stdout)
		}
		if res := s.tektite("check", "--archived", "--json"); !strings.Contains(res.stdout, "broken-z-zz9") {
			t.Errorf("check --archived must look at archived nodes: %s", res.stdout)
		}
	})

	t.Run("Archived nodes cannot be written to", func(t *testing.T) {
		s := archivedSandbox(t)
		res := s.tektite("note", "done-a-aa1", "text")
		if res.code != exitConflict || !strings.Contains(res.stderr, "unarchive") {
			t.Fatalf("%+v", res)
		}
		if s.exists(".tektite/events/done-a-aa1") {
			t.Error("an event was written for an archived node")
		}
	})

	t.Run("Unarchiving restores the node, its events and blobs", func(t *testing.T) {
		s := newSandbox(t)
		blob := blobName("kept")
		s.write(".tektite/blobs/"+blob, "kept")
		s.closed("done-a-aa1", nid(), "done", "2026-11-20T10:00:00Z")
		s.node("shot-a-bb2", nid(), "evidence", "done-a-aa1", "evidences: [done-a-aa1]\nclaim: shot\nattachments: [blobs/"+blob+"]\nprovenance:\n  commit: 3f2a9c1\n", "")
		s.tektite("archive", "done-a-aa1")
		res := s.tektite("unarchive", "done-a-aa1", "--json")
		if res.code != 0 || !strings.Contains(res.stdout, `"unarchived"`) {
			t.Fatalf("%+v", res)
		}
		for _, p := range []string{".tektite/nodes/done-a-aa1.md", ".tektite/nodes/shot-a-bb2.md", ".tektite/events/done-a-aa1", ".tektite/blobs/" + blob} {
			if !s.exists(p) {
				t.Errorf("missing %s", p)
			}
		}
		if s.exists(".tektite/archive/nodes/done-a-aa1.md") || s.exists(".tektite/archive/blobs/"+blob) {
			t.Error("archive still holds files")
		}
		if out := s.tektite("ls").stdout; !strings.Contains(out, "done-a-aa1") {
			t.Errorf("ls = %s", out)
		}
		if res := s.tektite("note", "done-a-aa1", "back"); res.code != 0 {
			t.Errorf("note = %+v", res)
		}
	})

	t.Run("Unarchiving a node that is not archived is a conflict", func(t *testing.T) {
		s := newSandbox(t)
		s.closed("done-a-aa1", nid(), "done", "2026-11-20T10:00:00Z")
		if res := s.tektite("unarchive", "done-a-aa1"); res.code != exitConflict {
			t.Fatalf("%+v", res)
		}
	})

	t.Run("Unarchive supports a dry run", func(t *testing.T) {
		s := archivedSandbox(t)
		res := s.tektite("unarchive", "done-a-aa1", "--dry-run", "--json")
		if res.code != 0 || !strings.Contains(compact(res.stdout), `"dry_run":true`) || !s.exists(".tektite/archive/nodes/done-a-aa1.md") {
			t.Fatalf("%+v", res)
		}
	})
}
