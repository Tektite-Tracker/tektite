package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	e2   = "01HZ3M0A1B2C3D4E5F6G7H8J92"
	e3   = "01HZ3M0A1B2C3D4E5F6G7H8J93"
	idP  = "01HZ3K8Q5W2X9N4T7V6B1C0DP1"
	idEv = "01HZ3K8Q5W2X9N4T7V6B1C0DE1"
)

const (
	gateApproval = "  - name: plans require human approval before work\n    rule: plan-approval\n"
	gateEvidence = "  - name: tickets require passing evidence before done\n    rule: passing-evidence\n    kinds: [ticket]\n"
	gateGaps     = "  - name: no open gaps before done\n    rule: no-open-gaps\n    kinds: [ticket]\n"
)

func (s *sandbox) policy(gates ...string) {
	s.write(".tektite/policy.md", "---\nversion: 1\ngates:\n"+strings.Join(gates, "")+"---\n")
}

func policySandbox(t *testing.T, body string, gates ...string) *sandbox {
	s := newSandbox(t)
	s.node("auth-plan-1ab", idP, "plan", "", "", "plan")
	s.node("auth-retry-3fa", idA, "ticket", "auth-plan-1ab", "", body)
	s.policy(gates...)
	return s
}

func (s *sandbox) evidence(exit string) {
	extra := "provenance:\n  actor: human:jesus\n"
	if exit != "" {
		extra += "  exit_code: " + exit + "\n"
	}
	s.node("shot-5de", idEv, "evidence", "auth-retry-3fa", extra, "proof")
}

func countEvents(t *testing.T, s *sandbox, ref string) int {
	t.Helper()
	return len(eventFiles(t, s, ref))
}

func TestCLI_Policy(t *testing.T) {
	agent := "agent:claude-opus/s-4f2a"

	t.Run("Loading the policy", func(t *testing.T) {
		s := policySandbox(t, "x", gateApproval)
		got := s.as(agent, "claim", "auth-retry-3fa")
		if got.code != exitPolicy {
			t.Errorf("result = %+v", got)
		}
	})

	t.Run("No policy means no gates", func(t *testing.T) {
		s := policySandbox(t, "x", gateApproval)
		if err := os.Remove(filepath.Join(s.root, ".tektite", "policy.md")); err != nil {
			t.Fatal(err)
		}
		if got := s.as(agent, "claim", "auth-retry-3fa"); got.code != 0 {
			t.Errorf("claim = %+v", got)
		}
		if got := s.as(agent, "set-status", "auth-retry-3fa", "done"); got.code != 0 {
			t.Errorf("set-status = %+v", got)
		}
	})

	t.Run("A malformed policy is reported, not enforced half-way", func(t *testing.T) {
		s := policySandbox(t, "x", gateApproval)
		s.write(".tektite/policy.md", "---\nversion: 1\ngates: [oops\n---\n")
		chk := s.tektite("check")
		if chk.code != exitError || !strings.Contains(chk.stdout, "TKT-E104") || !strings.Contains(chk.stdout, ".tektite/policy.md") {
			t.Errorf("check = %+v", chk)
		}
		got := s.as(agent, "claim", "auth-retry-3fa")
		if got.code != 0 || !strings.Contains(got.stderr, "warning: TKT-E104") {
			t.Errorf("claim = %+v", got)
		}
	})

	t.Run("Unknown rules and keys make the policy malformed", func(t *testing.T) {
		for name, src := range map[string]string{
			"rule":    "---\nversion: 1\ngates:\n  - name: g\n    rule: made-up\n---\n",
			"key":     "---\nversion: 1\ngates:\n  - name: g\n    rule: no-open-gaps\n    label: [x]\n---\n",
			"top":     "---\nversion: 1\ngate: []\n---\n",
			"version": "---\nversion: 2\ngates: []\n---\n",
		} {
			s := policySandbox(t, "x", gateApproval)
			s.write(".tektite/policy.md", src)
			chk := s.tektite("check")
			if chk.code != exitError || !strings.Contains(chk.stdout, "TKT-E104") {
				t.Errorf("%s: check = %+v", name, chk)
			}
			if got := s.as(agent, "claim", "auth-retry-3fa"); got.code != 0 {
				t.Errorf("%s: claim = %+v", name, got)
			}
		}
	})

	t.Run("A plan needs human approval before agents claim its tickets", func(t *testing.T) {
		s := policySandbox(t, "x", gateApproval)
		got := s.as(agent, "claim", "auth-retry-3fa")
		if got.code != 5 || !strings.Contains(got.stderr, "auth-plan-1ab is not approved") || !strings.Contains(got.stderr, "plans require human approval before work") {
			t.Errorf("result = %+v", got)
		}
		if n := countEvents(t, s, "auth-retry-3fa"); n != 0 {
			t.Errorf("events = %d", n)
		}
	})

	t.Run("A human approval opens the gate", func(t *testing.T) {
		s := policySandbox(t, "x", gateApproval)
		s.actorEvent("auth-plan-1ab", e1, "approve", "human:jesus", "2026-11-27T11:00:00Z", "revision: 1\n")
		if got := s.as(agent, "claim", "auth-retry-3fa"); got.code != 0 {
			t.Errorf("result = %+v", got)
		}
	})

	t.Run("A human can claim under an unapproved plan", func(t *testing.T) {
		s := policySandbox(t, "x", gateApproval)
		if got := s.as("human:jesus", "claim", "auth-retry-3fa"); got.code != 0 {
			t.Errorf("result = %+v", got)
		}
	})

	t.Run("An approval by an agent does not count", func(t *testing.T) {
		s := policySandbox(t, "x", gateApproval)
		s.actorEvent("auth-plan-1ab", e1, "approve", agent, "2026-11-27T11:00:00Z", "revision: 1\n")
		got := s.as(agent, "claim", "auth-retry-3fa")
		if got.code != 5 || !strings.Contains(got.stderr, "not approved") || !strings.Contains(got.stderr, "agents do not count") {
			t.Errorf("result = %+v", got)
		}
	})

	t.Run("A new revision invalidates the approval", func(t *testing.T) {
		s := policySandbox(t, "x", gateApproval)
		s.actorEvent("auth-plan-1ab", e1, "revision", "human:jesus", "2026-11-27T10:00:00Z", "number: 1\n")
		s.actorEvent("auth-plan-1ab", e2, "revision", "human:jesus", "2026-11-27T10:30:00Z", "number: 2\n")
		s.actorEvent("auth-plan-1ab", e3, "approve", "human:jesus", "2026-11-27T10:40:00Z", "revision: 2\n")
		if got := s.as(agent, "claim", "auth-retry-3fa"); got.code != 0 {
			t.Fatalf("before = %+v", got)
		}
		s.actorEvent("auth-plan-1ab", "01HZ3M0A1B2C3D4E5F6G7H8J94", "revision", "human:jesus", "2026-11-27T11:00:00Z", "number: 3\n")
		s.now = s.now.Add(time.Hour)
		got := s.as("agent:other", "claim", "auth-retry-3fa")
		if got.code != 5 || !strings.Contains(got.stderr, "approval was for revision 2, the current revision is 3") {
			t.Errorf("after = %+v", got)
		}
	})

	t.Run("Approving records the current revision", func(t *testing.T) {
		s := policySandbox(t, "x", gateApproval)
		s.actorEvent("auth-plan-1ab", e1, "revision", "human:jesus", "2026-11-27T10:00:00Z", "number: 1\n")
		s.actorEvent("auth-plan-1ab", e2, "revision", "human:jesus", "2026-11-27T10:30:00Z", "number: 2\n")
		if got := s.as("human:jesus", "approve", "auth-plan-1ab"); got.code != 0 {
			t.Fatalf("result = %+v", got)
		}
		files := eventFiles(t, s, "auth-plan-1ab")
		src := s.read(".tektite/events/auth-plan-1ab/" + files[len(files)-1])
		if !strings.Contains(src, "type: approve") || !strings.Contains(src, "revision: 2") {
			t.Errorf("event =\n%s", src)
		}
		if got := s.as(agent, "claim", "auth-retry-3fa"); got.code != 0 {
			t.Errorf("claim = %+v", got)
		}
		if got := s.as("human:jesus", "approve", "auth-plan-1ab", "--revision", "1"); got.code != exitUsage {
			t.Errorf("stale revision = %+v", got)
		}
	})

	t.Run("Only a human can approve", func(t *testing.T) {
		s := policySandbox(t, "x", gateApproval)
		got := s.as(agent, "approve", "auth-plan-1ab")
		if got.code != 5 || !strings.Contains(got.stderr, "only a human can approve") {
			t.Errorf("result = %+v", got)
		}
		if n := countEvents(t, s, "auth-plan-1ab"); n != 0 {
			t.Errorf("events = %d", n)
		}
	})

	t.Run("Next does not offer a ticket the actor may not claim", func(t *testing.T) {
		s := policySandbox(t, "x", gateApproval)
		s.env = map[string]string{"TKT_ACTOR": agent}
		if got := s.tektite("next", "--json"); got.code != exitNotFound {
			t.Errorf("next = %+v", got)
		}
		s.actorEvent("auth-plan-1ab", e1, "approve", "human:jesus", "2026-11-27T11:00:00Z", "revision: 1\n")
		if got := nextRefsAs(t, s, agent); len(got) != 1 || got[0] != "auth-retry-3fa" {
			t.Errorf("approved = %v", got)
		}
	})

	t.Run("A policy can require evidence before done", func(t *testing.T) {
		s := policySandbox(t, "x", gateEvidence)
		got := s.as(agent, "set-status", "auth-retry-3fa", "done")
		if got.code != 5 || !strings.Contains(got.stderr, "tickets require passing evidence before done") {
			t.Errorf("result = %+v", got)
		}
		if n := countEvents(t, s, "auth-retry-3fa"); n != 0 {
			t.Errorf("events = %d", n)
		}
		s.evidence("0")
		if got := s.as(agent, "set-status", "auth-retry-3fa", "done"); got.code != 0 {
			t.Errorf("with evidence = %+v", got)
		}
	})

	t.Run("Failing evidence does not satisfy the policy", func(t *testing.T) {
		s := policySandbox(t, "x", gateEvidence)
		s.evidence("1")
		got := s.as(agent, "set-status", "auth-retry-3fa", "done")
		if got.code != 5 || !strings.Contains(got.stderr, "no passing evidence") {
			t.Errorf("result = %+v", got)
		}
	})

	t.Run("A ticket needs passing evidence before done", func(t *testing.T) {
		s := policySandbox(t, "x", gateEvidence)
		if got := s.as("human:jesus", "set-status", "auth-retry-3fa", "done"); got.code != 5 {
			t.Errorf("result = %+v", got)
		}
	})

	t.Run("Open gaps block completion", func(t *testing.T) {
		s := policySandbox(t, "- [ ] retry banner missing <!-- g:01HZ3KB2 -->\n", gateGaps)
		got := s.as(agent, "set-status", "auth-retry-3fa", "done")
		if got.code != 5 || !strings.Contains(got.stderr, "01HZ3KB2: retry banner missing") {
			t.Errorf("result = %+v", got)
		}
		s.actorEvent("auth-retry-3fa", e1, "close", "human:jesus", "2026-11-27T11:00:00Z", "block: 01HZ3KB2\n")
		if got := s.as(agent, "set-status", "auth-retry-3fa", "done"); got.code != 0 {
			t.Errorf("closed = %+v", got)
		}
	})

	t.Run("A human can override a gate with a reason", func(t *testing.T) {
		s := policySandbox(t, "x", gateEvidence)
		got := s.as("human:jesus", "set-status", "auth-retry-3fa", "done", "--override", "accepted risk")
		if got.code != 0 {
			t.Fatalf("result = %+v", got)
		}
		files := eventFiles(t, s, "auth-retry-3fa")
		if src := s.read(".tektite/events/auth-retry-3fa/" + files[0]); !strings.Contains(src, "override: accepted risk") {
			t.Errorf("event =\n%s", src)
		}
		var doc map[string]any
		decode(t, s.tektite("show", "auth-retry-3fa", "--json").stdout, &doc)
		hist, _ := doc["history"].([]any)
		if len(hist) != 1 || hist[0].(map[string]any)["override"] != "accepted risk" {
			t.Errorf("history = %v", hist)
		}
		if out := s.tektite("show", "auth-retry-3fa").stdout; !strings.Contains(out, "(override: accepted risk)") {
			t.Errorf("show =\n%s", out)
		}
		if chk := s.tektite("check"); chk.code != 0 {
			t.Errorf("check = %+v", chk)
		}
	})

	t.Run("An agent cannot override", func(t *testing.T) {
		s := policySandbox(t, "x", gateEvidence)
		got := s.as(agent, "set-status", "auth-retry-3fa", "done", "--override", "skip")
		if got.code != 5 || !strings.Contains(got.stderr, "an agent cannot override") {
			t.Errorf("result = %+v", got)
		}
		if n := countEvents(t, s, "auth-retry-3fa"); n != 0 {
			t.Errorf("events = %d", n)
		}
	})

	t.Run("A gate failure has a JSON shape", func(t *testing.T) {
		s := policySandbox(t, "- [ ] retry banner missing <!-- g:01HZ3KB2 -->\n", gateGaps)
		got := s.as(agent, "set-status", "auth-retry-3fa", "done", "--json")
		var doc map[string]any
		decode(t, got.stdout, &doc)
		gates, _ := doc["gates"].([]any)
		if got.code != 5 || doc["code"] != float64(5) || doc["hint"] == "" || len(gates) != 1 {
			t.Fatalf("doc = %v", doc)
		}
		g := gates[0].(map[string]any)
		items, _ := g["items"].([]any)
		if g["name"] != "no open gaps before done" || g["rule"] != "no-open-gaps" || g["reason"] == "" || len(items) != 1 {
			t.Errorf("gate = %v", g)
		}
	})

	t.Run("A gate applies only to listed kinds", func(t *testing.T) {
		s := policySandbox(t, "x", gateEvidence)
		s.node("loose-end-3fb", "01HZ3K8Q5W2X9N4T7V6B1C0D0Z", "gap", "auth-retry-3fa", "", "x")
		if got := s.as(agent, "set-status", "loose-end-3fb", "done"); got.code != 0 {
			t.Errorf("result = %+v", got)
		}
	})

	t.Run("A gate can be scoped by label", func(t *testing.T) {
		scoped := "  - name: security needs evidence\n    rule: passing-evidence\n    labels: [security]\n"
		s := policySandbox(t, "x", scoped)
		s.node("plain-one-3fc", "01HZ3K8Q5W2X9N4T7V6B1C0D0Y", "ticket", "", "", "x")
		if got := s.as(agent, "set-status", "plain-one-3fc", "done"); got.code != 0 {
			t.Errorf("unlabelled = %+v", got)
		}
		s.node("auth-retry-3fa", idA, "ticket", "auth-plan-1ab", "labels: [security, auth]\n", "x")
		if got := s.as(agent, "set-status", "auth-retry-3fa", "done"); got.code != 5 {
			t.Errorf("labelled = %+v", got)
		}
	})

	t.Run("check reports violations in existing data", func(t *testing.T) {
		s := policySandbox(t, "x", gateEvidence)
		s.actorEvent("auth-retry-3fa", e1, "set-status", "agent:a", "2026-11-27T11:00:00Z", "status: done\n")
		chk := s.tektite("check")
		for _, want := range []string{"TKT-E105", "auth-retry-3fa", e1, "tickets require passing evidence before done", "events/auth-retry-3fa/" + e1 + ".md"} {
			if chk.code != exitError || !strings.Contains(chk.stdout, want) {
				t.Fatalf("missing %q in %+v", want, chk)
			}
		}
	})

	t.Run("check reports an agent claim made before approval", func(t *testing.T) {
		s := policySandbox(t, "x", gateApproval)
		s.actorEvent("auth-retry-3fa", e1, "claim", agent, "2026-11-27T10:00:00Z", "ttl: 30m\n")
		s.actorEvent("auth-plan-1ab", e2, "approve", "human:jesus", "2026-11-27T11:00:00Z", "revision: 1\n")
		chk := s.tektite("check")
		if chk.code != exitError || !strings.Contains(chk.stdout, "TKT-E105") || !strings.Contains(chk.stdout, e1) {
			t.Errorf("early claim = %+v", chk)
		}
		s.actorEvent("auth-retry-3fa", e3, "release", agent, "2026-11-27T11:05:00Z", "")
		s.actorEvent("auth-retry-3fa", "01HZ3M0A1B2C3D4E5F6G7H8J95", "claim", agent, "2026-11-27T11:10:00Z", "ttl: 30m\n")
		chk = s.tektite("check", "--json")
		var rep struct{ Problems []map[string]any }
		decode(t, chk.stdout, &rep)
		if len(rep.Problems) != 1 || !strings.Contains(rep.Problems[0]["message"].(string), e1) {
			t.Errorf("problems = %v", rep.Problems)
		}
	})

	t.Run("check ignores an override by an agent", func(t *testing.T) {
		s := policySandbox(t, "x", gateEvidence)
		s.actorEvent("auth-retry-3fa", e1, "set-status", agent, "2026-11-27T11:00:00Z", "status: done\noverride: skip\n")
		if chk := s.tektite("check"); chk.code != exitError || !strings.Contains(chk.stdout, "TKT-E105") {
			t.Errorf("check = %+v", chk)
		}
	})

	t.Run("A gate can be explained", func(t *testing.T) {
		s := policySandbox(t, "- [ ] retry banner missing <!-- g:01HZ3KB2 -->\n", gateApproval, gateEvidence, gateGaps)
		s.evidence("0")
		got := s.tektite("policy", "explain", "auth-retry-3fa")
		if got.code != 0 {
			t.Fatalf("result = %+v", got)
		}
		for _, want := range []string{"plans require human approval before work  not met", "tickets require passing evidence before done  met", "no open gaps before done  not met", "01HZ3KB2: retry banner missing"} {
			if !strings.Contains(got.stdout, want) {
				t.Errorf("missing %q in\n%s", want, got.stdout)
			}
		}
		var doc map[string]any
		decode(t, s.tektite("policy", "explain", "auth-retry-3fa", "--json").stdout, &doc)
		if gates, _ := doc["gates"].([]any); len(gates) != 3 {
			t.Errorf("doc = %v", doc)
		}
	})

	t.Run("Explaining a node that no gate applies to", func(t *testing.T) {
		s := policySandbox(t, "x", gateEvidence)
		got := s.tektite("policy", "explain", "auth-plan-1ab")
		if got.code != 0 || !strings.Contains(got.stdout, "no gate applies") {
			t.Errorf("result = %+v", got)
		}
	})
}

func nextRefsAs(t *testing.T, s *sandbox, actor string) []string {
	t.Helper()
	s.env = map[string]string{"TKT_ACTOR": actor}
	return nextRefs(t, s)
}
