package main

import (
	"strings"
	"testing"
	"time"
)

func (s *sandbox) actorEvent(ref, id, typ, actor, at, extra string) {
	s.write(".tektite/events/"+ref+"/"+id+".md", "---\ntype: "+typ+"\nactor: "+actor+"\nat: "+at+"\n"+extra+"---\n")
}

func (s *sandbox) as(actor string, args ...string) result {
	s.env = map[string]string{"TKT_ACTOR": actor}
	return s.tektite(args...)
}

func claimSandbox(t *testing.T) *sandbox {
	s := newSandbox(t)
	s.node("auth-retry-3fa", idA, "ticket", "", "", "x")
	return s
}

func TestCLI_Claims(t *testing.T) {
	t.Run("Claiming an open node", func(t *testing.T) {
		s := claimSandbox(t)
		got := s.as("agent:claude-opus/s-4f2a", "claim", "auth-retry-3fa")
		if got.code != 0 {
			t.Fatalf("result = %+v", got)
		}
		files := eventFiles(t, s, "auth-retry-3fa")
		src := s.read(".tektite/events/auth-retry-3fa/" + files[0])
		if !strings.Contains(src, "type: claim") || !strings.Contains(src, "ttl: 30m0s") {
			t.Errorf("event =\n%s", src)
		}
		var doc map[string]any
		decode(t, s.tektite("show", "auth-retry-3fa", "--json").stdout, &doc)
		claim, _ := doc["claim"].(map[string]any)
		if claim["holder"] != "agent:claude-opus/s-4f2a" || claim["live"] != true || claim["expires_at"] != "2026-11-27T12:30:00Z" {
			t.Errorf("claim = %v", claim)
		}
	})

	t.Run("A claim blocks other actors while the lease is live", func(t *testing.T) {
		s := claimSandbox(t)
		s.actorEvent("auth-retry-3fa", e1, "claim", "agent:a", "2026-11-27T11:50:00Z", "ttl: 30m\n")
		got := s.as("agent:b", "claim", "auth-retry-3fa")
		if got.code != exitConflict || !strings.Contains(got.stderr, "agent:a") || !strings.Contains(got.stderr, "2026-11-27T12:20:00Z") {
			t.Errorf("result = %+v", got)
		}
		if n := len(eventFiles(t, s, "auth-retry-3fa")); n != 1 {
			t.Errorf("events = %d", n)
		}
		var doc map[string]any
		decode(t, s.as("agent:b", "claim", "auth-retry-3fa", "--json").stdout, &doc)
		if doc["code"] != float64(4) {
			t.Errorf("doc = %v", doc)
		}
	})

	t.Run("A heartbeat extends the lease", func(t *testing.T) {
		s := claimSandbox(t)
		s.actorEvent("auth-retry-3fa", e1, "claim", "agent:a", "2026-11-27T11:35:00Z", "ttl: 30m\n")
		if got := s.as("agent:a", "heartbeat", "auth-retry-3fa"); got.code != 0 {
			t.Fatalf("result = %+v", got)
		}
		var doc map[string]any
		decode(t, s.tektite("show", "auth-retry-3fa", "--json").stdout, &doc)
		if c, _ := doc["claim"].(map[string]any); c["expires_at"] != "2026-11-27T12:30:00Z" {
			t.Errorf("claim = %v", c)
		}
	})

	t.Run("An expired lease can be taken over", func(t *testing.T) {
		s := claimSandbox(t)
		s.actorEvent("auth-retry-3fa", e1, "claim", "agent:a", "2026-11-27T11:20:00Z", "ttl: 30m\n")
		if got := s.as("agent:b", "claim", "auth-retry-3fa"); got.code != 0 {
			t.Fatalf("result = %+v", got)
		}
		if got := s.as("agent:a", "heartbeat", "auth-retry-3fa"); got.code != exitConflict {
			t.Errorf("former holder heartbeat: %+v", got)
		}
		var doc map[string]any
		decode(t, s.tektite("show", "auth-retry-3fa", "--json").stdout, &doc)
		if c, _ := doc["claim"].(map[string]any); c["holder"] != "agent:b" {
			t.Errorf("claim = %v", c)
		}
	})

	t.Run("Only the holder can heartbeat or release", func(t *testing.T) {
		s := claimSandbox(t)
		s.actorEvent("auth-retry-3fa", e1, "claim", "agent:a", "2026-11-27T11:50:00Z", "ttl: 30m\n")
		for _, verb := range []string{"heartbeat", "release"} {
			if got := s.as("agent:b", verb, "auth-retry-3fa"); got.code != exitConflict {
				t.Errorf("%s: %+v", verb, got)
			}
		}
		if got := s.as("agent:b", "release", "ghost-000"); got.code != exitNotFound {
			t.Errorf("missing node: %+v", got)
		}
		if n := len(eventFiles(t, s, "auth-retry-3fa")); n != 1 {
			t.Errorf("events = %d", n)
		}
		unclaimed := claimSandbox(t)
		if got := unclaimed.as("agent:a", "release", "auth-retry-3fa"); got.code != exitConflict {
			t.Errorf("release of an unclaimed node: %+v", got)
		}
	})

	t.Run("Releasing frees the node immediately", func(t *testing.T) {
		s := claimSandbox(t)
		s.actorEvent("auth-retry-3fa", e1, "claim", "agent:a", "2026-11-27T11:50:00Z", "ttl: 30m\n")
		if got := s.as("agent:a", "release", "auth-retry-3fa"); got.code != 0 {
			t.Fatalf("result = %+v", got)
		}
		var doc map[string]any
		decode(t, s.tektite("show", "auth-retry-3fa", "--json").stdout, &doc)
		if doc["claim"] != nil {
			t.Errorf("claim = %v", doc["claim"])
		}
		s.now = s.now.Add(time.Second)
		if got := s.as("agent:b", "claim", "auth-retry-3fa"); got.code != 0 {
			t.Errorf("claim after release: %+v", got)
		}
	})

	t.Run("A claim can set its own ttl", func(t *testing.T) {
		s := claimSandbox(t)
		if got := s.as("agent:a", "claim", "auth-retry-3fa", "--ttl", "10m"); got.code != 0 {
			t.Fatalf("result = %+v", got)
		}
		if src := s.read(".tektite/events/auth-retry-3fa/" + eventFiles(t, s, "auth-retry-3fa")[0]); !strings.Contains(src, "ttl: 10m0s") {
			t.Errorf("event =\n%s", src)
		}
		for _, bad := range []string{"soon", "0s", "-5m"} {
			if got := s.as("agent:a", "claim", "auth-retry-3fa", "--ttl", bad); got.code != exitUsage {
				t.Errorf("ttl %q: %+v", bad, got)
			}
		}
	})

	t.Run("Show and ls display the current claim", func(t *testing.T) {
		s := claimSandbox(t)
		s.actorEvent("auth-retry-3fa", e1, "claim", "agent:a", "2026-11-27T11:50:00Z", "ttl: 30m\n")
		if got := s.tektite("show", "auth-retry-3fa"); !strings.Contains(got.stdout, "claimed by: agent:a (expires 2026-11-27T12:20:00Z)") {
			t.Errorf("show = %s", got.stdout)
		}
		if got := s.tektite("ls"); !strings.Contains(got.stdout, "agent:a") {
			t.Errorf("ls = %s", got.stdout)
		}
		var list []map[string]any
		decode(t, s.tektite("ls", "--json").stdout, &list)
		if c, _ := list[0]["claim"].(map[string]any); c["holder"] != "agent:a" || c["live"] != true {
			t.Errorf("list = %v", list)
		}
		s.now = s.now.Add(time.Hour)
		if got := s.tektite("show", "auth-retry-3fa"); !strings.Contains(got.stdout, "(expired 2026-11-27T12:20:00Z)") {
			t.Errorf("show = %s", got.stdout)
		}
	})
}

func nextSandbox(t *testing.T) *sandbox {
	s := newSandbox(t)
	s.node("low-one-3f1", "01HZ3K8Q5W2X9N4T7V6B1C0D01", "ticket", "", "priority: low\n", "x")
	s.node("high-old-3f2", "01HZ3K8Q5W2X9N4T7V6B1C0D02", "ticket", "", "priority: high\n", "x")
	s.node("high-new-3f3", "01HZ3K8Q5W2X9N4T7V6B1C0D03", "ticket", "", "priority: high\n", "x")
	s.node("num-five-3f4", "01HZ3K8Q5W2X9N4T7V6B1C0D04", "ticket", "", "priority: 5\n", "x")
	s.node("a-plan-3f5", "01HZ3K8Q5W2X9N4T7V6B1C0D05", "plan", "", "priority: 9\n", "x")
	return s
}

func nextRefs(t *testing.T, s *sandbox, args ...string) []string {
	t.Helper()
	got := s.tektite(append([]string{"next", "--json"}, args...)...)
	var items []map[string]any
	if got.code == 0 {
		decode(t, got.stdout, &items)
	}
	out := []string{}
	for _, it := range items {
		out = append(out, it["ref"].(string))
	}
	return out
}

func TestCLI_Next(t *testing.T) {
	t.Run("Next returns the best available node", func(t *testing.T) {
		s := nextSandbox(t)
		got := nextRefs(t, s)
		if len(got) != 1 || got[0] != "num-five-3f4" {
			t.Errorf("next = %v", got)
		}
		if got := s.tektite("next"); got.code != 0 || !strings.HasPrefix(got.stdout, "num-five-3f4") {
			t.Errorf("text = %+v", got)
		}
	})

	t.Run("Next ranks by the priority field then by age", func(t *testing.T) {
		s := nextSandbox(t)
		got := strings.Join(nextRefs(t, s, "--limit", "9"), " ")
		if got != "num-five-3f4 high-old-3f2 high-new-3f3 low-one-3f1" {
			t.Errorf("order = %s", got)
		}
		if got := strings.Join(nextRefs(t, s, "--limit", "1", "--kind", ""), " "); got != "a-plan-3f5" {
			t.Errorf("any kind = %s", got)
		}
	})

	t.Run("Next skips claimed, blocked, and gated nodes", func(t *testing.T) {
		s := nextSandbox(t)
		s.actorEvent("num-five-3f4", e1, "claim", "agent:a", "2026-11-27T11:50:00Z", "ttl: 30m\n")
		s.actorEvent("high-old-3f2", e1, "set-status", "agent:a", "2026-11-27T11:50:00Z", "status: blocked\n")
		got := strings.Join(nextRefs(t, s, "--limit", "9"), " ")
		if got != "high-new-3f3 low-one-3f1" {
			t.Errorf("order = %s", got)
		}
	})

	t.Run("An expired claim does not hide a node from next", func(t *testing.T) {
		s := nextSandbox(t)
		s.actorEvent("num-five-3f4", e1, "claim", "agent:a", "2026-11-27T10:00:00Z", "ttl: 30m\n")
		if got := nextRefs(t, s); len(got) != 1 || got[0] != "num-five-3f4" {
			t.Errorf("next = %v", got)
		}
	})

	t.Run("A blocked node is not offered by `tektite next`", func(t *testing.T) {
		s := nextSandbox(t)
		s.node("blocker-3f6", "01HZ3K8Q5W2X9N4T7V6B1C0D06", "ticket", "", "blocks: [num-five-3f4]\n", "x")
		for _, ref := range nextRefs(t, s, "--limit", "9") {
			if ref == "num-five-3f4" {
				t.Errorf("blocked node offered")
			}
		}
		s.tektite("unlink", "blocker-3f6", "blocks", "num-five-3f4", "--actor", "human:x")
		s.tektite("link", "low-one-3f1", "blocked-by", "high-old-3f2", "--actor", "human:x")
		got := strings.Join(nextRefs(t, s, "--limit", "9"), " ")
		if strings.Contains(got, "low-one-3f1") || !strings.Contains(got, "num-five-3f4") {
			t.Errorf("order = %s", got)
		}
	})

	t.Run("Finishing the blocker releases the node", func(t *testing.T) {
		s := nextSandbox(t)
		s.node("blocker-3f6", "01HZ3K8Q5W2X9N4T7V6B1C0D06", "ticket", "", "blocks: [num-five-3f4]\n", "x")
		s.actorEvent("blocker-3f6", e1, "set-status", "agent:a", "2026-11-27T11:00:00Z", "status: done\n")
		if got := nextRefs(t, s); len(got) != 1 || got[0] != "num-five-3f4" {
			t.Errorf("next = %v", got)
		}
	})

	t.Run("Nothing to do", func(t *testing.T) {
		s := newSandbox(t)
		got := s.tektite("next")
		if got.code != exitNotFound || !strings.Contains(got.stderr, "nothing is available") {
			t.Errorf("result = %+v", got)
		}
	})

	t.Run("Next can claim in one step", func(t *testing.T) {
		s := nextSandbox(t)
		got := s.as("agent:a", "next", "--claim", "--json")
		if got.code != 0 {
			t.Fatalf("result = %+v", got)
		}
		var doc map[string]any
		decode(t, s.tektite("show", "num-five-3f4", "--json").stdout, &doc)
		if c, _ := doc["claim"].(map[string]any); c["holder"] != "agent:a" {
			t.Errorf("claim = %v", doc["claim"])
		}
		again := s.as("agent:b", "next", "--claim")
		if again.code != 0 || !strings.HasPrefix(again.stdout, "high-old-3f2") {
			t.Errorf("second agent: %+v", again)
		}
	})
}
