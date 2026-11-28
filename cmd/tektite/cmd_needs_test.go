package main

import (
	"strings"
	"testing"
)

func needsRefs(t *testing.T, s *sandbox, kind string) []string {
	t.Helper()
	var list []map[string]any
	got := s.tektite("ls", "--json", "--needs", kind)
	if got.code != 0 {
		t.Fatalf("ls --needs %s = %+v", kind, got)
	}
	decode(t, got.stdout, &list)
	out := []string{}
	for _, it := range list {
		out = append(out, it["ref"].(string))
	}
	return out
}

func TestCLI_ListNeeds(t *testing.T) {
	t.Run("Listing what needs approval", func(t *testing.T) {
		s := policySandbox(t, "x", gateApproval)
		s.node("other-plan-2cd", idB, "plan", "", "", "p")
		s.actorEvent("other-plan-2cd", e1, "approve", "human:jesus", "2026-11-27T11:00:00Z", "revision: 1\n")
		if got := strings.Join(needsRefs(t, s, "approval"), ","); got != "auth-plan-1ab" {
			t.Errorf("got %s", got)
		}
	})

	t.Run("Listing what needs an override", func(t *testing.T) {
		s := policySandbox(t, "x", gateEvidence)
		s.node("auth-ok-4aa", idB, "ticket", "auth-plan-1ab", "", "t")
		s.evidenceFor("auth-ok-4aa", "01HZ3K8Q5W2X9N4T7V6B1C0DE2")
		s.event("auth-retry-3fa", e1, "set-status", "2026-11-27T10:00:00Z", "status: review\n")
		s.event("auth-ok-4aa", e2, "set-status", "2026-11-27T10:00:00Z", "status: review\n")
		if got := strings.Join(needsRefs(t, s, "override"), ","); got != "auth-retry-3fa" {
			t.Errorf("got %s", got)
		}
	})

	t.Run("Listing what needs review", func(t *testing.T) {
		s := policySandbox(t, "x")
		s.event("auth-retry-3fa", e1, "set-status", "2026-11-27T10:00:00Z", "status: review\n")
		if got := strings.Join(needsRefs(t, s, "review"), ","); got != "auth-retry-3fa" {
			t.Errorf("got %s", got)
		}
	})

	t.Run("An unknown needs value is a usage error", func(t *testing.T) {
		s := policySandbox(t, "x")
		if got := s.tektite("ls", "--needs", "lunch"); got.code != exitUsage {
			t.Errorf("result = %+v", got)
		}
		if got := s.tektite("ls", "--json", "--needs", "approval"); got.code != 0 || strings.TrimSpace(got.stdout) != "[]" {
			t.Errorf("empty = %+v", got)
		}
	})
}
