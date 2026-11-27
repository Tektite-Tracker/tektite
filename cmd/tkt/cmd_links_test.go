package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type showLinks struct {
	Links []struct {
		Rel    string `json:"rel"`
		Target string `json:"target"`
		Ref    string `json:"ref"`
	} `json:"links"`
	Backlinks []struct {
		Rel string `json:"rel"`
		Ref string `json:"ref"`
	} `json:"backlinks"`
}

func (s *sandbox) links(ref string) showLinks {
	s.t.Helper()
	var doc showLinks
	decode(s.t, s.tkt("show", ref, "--json").stdout, &doc)
	return doc
}

func linkSandbox(t *testing.T) *sandbox {
	s := newSandbox(t)
	s.node("ticket-a-3f1", "01HZ3K8Q5W2X9N4T7V6B1C0D01", "ticket", "", "implements: [plan-p-3f2]\n", "x")
	s.node("plan-p-3f2", "01HZ3K8Q5W2X9N4T7V6B1C0D02", "plan", "", "", "x")
	s.node("ticket-b-3f3", "01HZ3K8Q5W2X9N4T7V6B1C0D03", "ticket", "", "", "x")
	return s
}

func TestCLI_Links(t *testing.T) {
	t.Run("The folded link set is frontmatter plus events", func(t *testing.T) {
		s := linkSandbox(t)
		s.event("ticket-a-3f1", "01HZ3M0A1B2C3D4E5F6G7H8J91", "link", "2026-11-27T09:00:00Z", "rel: blocks\ntarget: ticket-b-3f3\n")
		s.event("ticket-a-3f1", "01HZ3M0A1B2C3D4E5F6G7H8J92", "unlink", "2026-11-27T10:00:00Z", "rel: implements\ntarget: plan-p-3f2\n")
		got := s.links("ticket-a-3f1")
		if len(got.Links) != 1 || got.Links[0].Rel != "blocks" || got.Links[0].Ref != "ticket-b-3f3" {
			t.Errorf("links = %+v", got.Links)
		}
	})

	t.Run("A late unlink removes only the matching link", func(t *testing.T) {
		s := linkSandbox(t)
		s.event("ticket-a-3f1", "01HZ3M0A1B2C3D4E5F6G7H8J91", "link", "2026-11-27T09:00:00Z", "rel: blocks\ntarget: ticket-b-3f3\n")
		s.event("ticket-a-3f1", "01HZ3M0A1B2C3D4E5F6G7H8J92", "unlink", "2026-11-27T10:00:00Z", "rel: blocks\ntarget: 3f3\n")
		s.event("ticket-a-3f1", "01HZ3M0A1B2C3D4E5F6G7H8J93", "unlink", "2026-11-27T11:00:00Z", "rel: mentions\ntarget: plan-p-3f2\n")
		got := s.links("ticket-a-3f1")
		if len(got.Links) != 1 || got.Links[0].Rel != "implements" {
			t.Errorf("links = %+v", got.Links)
		}
	})

	t.Run("A link has an inverse on its target", func(t *testing.T) {
		s := linkSandbox(t)
		got := s.links("plan-p-3f2")
		if len(got.Backlinks) != 1 || got.Backlinks[0].Rel != "implemented-by" || got.Backlinks[0].Ref != "ticket-a-3f1" {
			t.Errorf("backlinks = %+v", got.Backlinks)
		}
		if out := s.tkt("show", "plan-p-3f2"); !strings.Contains(out.stdout, "implemented-by: ticket-a-3f1") {
			t.Errorf("show = %s", out.stdout)
		}
	})

	t.Run("Backlinks require no edit to the target", func(t *testing.T) {
		s := linkSandbox(t)
		path := filepath.Join(s.root, ".tektite", "nodes", "plan-p-3f2.md")
		before, _ := os.ReadFile(path)
		s.links("plan-p-3f2")
		after, _ := os.ReadFile(path)
		if string(before) != string(after) {
			t.Errorf("target file changed")
		}
	})

	t.Run("Link targets accept any unambiguous reference", func(t *testing.T) {
		s := linkSandbox(t)
		s.node("ticket-c-3f4", "01HZ3K8Q5W2X9N4T7V6B1C0D04", "ticket", "", "", "see [[3f3]]")
		got := s.links("ticket-c-3f4")
		if len(got.Links) != 1 || got.Links[0].Ref != "ticket-b-3f3" {
			t.Errorf("links = %+v", got.Links)
		}
	})

	t.Run("A dropped blocker does not hold the node forever", func(t *testing.T) {
		s := linkSandbox(t)
		s.node("ticket-d-3f5", "01HZ3K8Q5W2X9N4T7V6B1C0D05", "ticket", "", "blocked-by: [ticket-b-3f3]\n", "x")
		s.event("ticket-b-3f3", "01HZ3M0A1B2C3D4E5F6G7H8J91", "set-status", "2026-11-27T09:00:00Z", "status: dropped\n")
		if got := nextRefs(t, s, "--limit", "9"); !strings.Contains(strings.Join(got, " "), "ticket-d-3f5") {
			t.Errorf("next = %v", got)
		}
		out := s.tkt("check")
		if !strings.Contains(out.stdout, "note TKT-N102") || !strings.Contains(out.stdout, "ticket-d-3f5.md") {
			t.Errorf("check = %s", out.stdout)
		}
	})

	t.Run("Mutually blocking nodes are not offered by next", func(t *testing.T) {
		s := newSandbox(t)
		s.node("ticket-a-3f1", "01HZ3K8Q5W2X9N4T7V6B1C0D01", "ticket", "", "blocks: [ticket-b-3f3]\n", "x")
		s.node("ticket-b-3f3", "01HZ3K8Q5W2X9N4T7V6B1C0D03", "ticket", "", "blocks: [ticket-a-3f1]\n", "x")
		if got := s.tkt("next"); got.code != exitNotFound {
			t.Errorf("next = %+v", got)
		}
	})

	t.Run("A link to a missing node", func(t *testing.T) {
		s := linkSandbox(t)
		s.node("ticket-e-3f6", "01HZ3K8Q5W2X9N4T7V6B1C0D06", "ticket", "", "evidences: [ghost-000]\n", "x")
		out := s.tkt("check")
		if out.code != 0 || !strings.Contains(out.stdout, "TKT-W101") || !strings.Contains(out.stdout, "evidences") || !strings.Contains(out.stdout, "ghost-000") {
			t.Errorf("check = %+v", out)
		}
		if show := s.tkt("show", "ticket-e-3f6"); show.code != 0 {
			t.Errorf("show = %+v", show)
		}
	})
}
