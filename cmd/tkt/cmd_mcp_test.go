package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type rpcMsg struct {
	ID     any            `json:"id"`
	Result map[string]any `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (s *sandbox) mcp(flags []string, msgs ...string) []rpcMsg {
	s.t.Helper()
	res := s.tktIn(s.root, strings.Join(msgs, "\n")+"\n", append([]string{"mcp"}, flags...)...)
	if res.code != 0 {
		s.t.Fatalf("mcp exit %d: %s", res.code, res.stderr)
	}
	out := []rpcMsg{}
	for _, line := range strings.Split(strings.TrimSpace(res.stdout), "\n") {
		if line == "" {
			continue
		}
		var m rpcMsg
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			s.t.Fatalf("reply %q is not JSON: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

func rpcCall(id int, tool string, args map[string]any) string {
	params, _ := json.Marshal(map[string]any{"name": tool, "arguments": args})
	msg, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": "tools/call", "params": json.RawMessage(params)})
	return string(msg)
}

func toolText(t *testing.T, m rpcMsg) (string, bool) {
	t.Helper()
	if m.Error != nil {
		t.Fatalf("protocol error: %+v", m.Error)
	}
	content, _ := m.Result["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("want one content item, got %v", m.Result)
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	isError, _ := m.Result["isError"].(bool)
	return text, isError
}

func (s *sandbox) callTool(flags []string, tool string, args map[string]any) (string, bool) {
	s.t.Helper()
	replies := s.mcp(flags, rpcCall(1, tool, args))
	if len(replies) != 1 {
		s.t.Fatalf("want one reply, got %d", len(replies))
	}
	return toolText(s.t, replies[0])
}

func TestCLI_MCP(t *testing.T) {
	t.Run("Initialising a session", func(t *testing.T) {
		s := eventSandbox(t)
		replies := s.mcp(nil,
			`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`,
			`{"jsonrpc":"2.0","method":"notifications/initialized"}`)
		if len(replies) != 1 {
			t.Fatalf("only initialize should be answered, got %d replies", len(replies))
		}
		info, _ := replies[0].Result["serverInfo"].(map[string]any)
		caps, _ := replies[0].Result["capabilities"].(map[string]any)
		if info["name"] != "tkt" || info["version"] == "" || caps["tools"] == nil || replies[0].Result["protocolVersion"] != "2025-03-26" {
			t.Errorf("unexpected initialize result: %v", replies[0].Result)
		}
	})

	t.Run("Listing the tools", func(t *testing.T) {
		s := eventSandbox(t)
		replies := s.mcp(nil, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
		tools, _ := replies[0].Result["tools"].([]any)
		have := map[string]bool{}
		for _, raw := range tools {
			tool := raw.(map[string]any)
			schema, _ := tool["inputSchema"].(map[string]any)
			if tool["name"] == "" || tool["description"] == "" || schema["type"] != "object" {
				t.Errorf("incomplete tool: %v", tool)
			}
			have[tool["name"].(string)] = true
		}
		for _, want := range []string{"show", "ls", "next", "context", "check", "new", "set_status", "note", "link", "unlink", "claim", "heartbeat", "release", "approve", "revise", "attach_evidence", "rerun", "close_gap", "policy_explain"} {
			if !have["tkt_"+want] {
				t.Errorf("tool tkt_%s is missing", want)
			}
		}
	})

	t.Run("Unknown methods and malformed input are rejected", func(t *testing.T) {
		s := eventSandbox(t)
		replies := s.mcp(nil, `{"jsonrpc":"2.0","id":1,"method":"nope"}`, `not json`, `{"jsonrpc":"2.0","id":3,"method":"ping"}`)
		if len(replies) != 3 || replies[0].Error == nil || replies[0].Error.Code != -32601 || replies[1].Error == nil || replies[1].Error.Code != -32700 || replies[2].Error != nil {
			t.Errorf("unexpected replies: %+v", replies)
		}
	})

	t.Run("Reading a node through a tool", func(t *testing.T) {
		s := eventSandbox(t)
		text, isError := s.callTool(nil, "tkt_show", map[string]any{"ref": "auth-retry-3fa"})
		cli := s.tkt("show", "auth-retry-3fa", "--json")
		if isError || strings.TrimSpace(text) != strings.TrimSpace(cli.stdout) {
			t.Errorf("tool result differs from the CLI:\n%s\n%s", text, cli.stdout)
		}
	})

	t.Run("Writing through a tool appends the same event as the CLI", func(t *testing.T) {
		s := eventSandbox(t)
		text, isError := s.callTool(nil, "tkt_set_status", map[string]any{"ref": "auth-retry-3fa", "status": "review", "actor": "agent:claude"})
		var doc struct {
			Event, Type, Actor, Path string
		}
		if isError || json.Unmarshal([]byte(text), &doc) != nil || doc.Type != "set-status" || doc.Actor != "agent:claude" || doc.Event == "" {
			t.Fatalf("unexpected result (error=%v): %s", isError, text)
		}
		if event := s.read(doc.Path); !strings.Contains(event, "type: set-status") || !strings.Contains(event, "status: review") {
			t.Errorf("unexpected event:\n%s", event)
		}
	})

	t.Run("A refused write is a tool error with its kind and hint", func(t *testing.T) {
		s := policySandbox(t, "x", gateEvidence)
		text, isError := s.callTool(nil, "tkt_set_status", map[string]any{"ref": "auth-retry-3fa", "status": "done", "actor": "agent:claude"})
		var doc struct {
			Kind, Hint string
			Gates      []struct{ Name string }
		}
		if err := json.Unmarshal([]byte(text), &doc); err != nil || !isError || doc.Kind != "policy" || doc.Hint == "" || len(doc.Gates) != 1 {
			t.Fatalf("unexpected result (error=%v): %s", isError, text)
		}
		if files := eventFiles(t, s, "auth-retry-3fa"); len(files) != 0 {
			t.Errorf("no event should be appended, found %v", files)
		}
	})

	t.Run("Warnings are returned as diagnostics", func(t *testing.T) {
		s := newSandbox(t)
		s.node("auth-retry-3fa", idA, "ticket", "", "", "x")
		text, isError := s.callTool(nil, "tkt_note", map[string]any{"ref": "auth-retry-3fa", "text": "hello"})
		var doc struct {
			Warnings []struct{ Code, Severity string }
		}
		if err := json.Unmarshal([]byte(text), &doc); err != nil || isError || len(doc.Warnings) != 1 || doc.Warnings[0].Code != "TKT-W006" || doc.Warnings[0].Severity != "warning" {
			t.Errorf("unexpected result (error=%v): %s", isError, text)
		}
	})

	t.Run("Evidence can be attached from text", func(t *testing.T) {
		s := evidenceSandbox(t)
		text, isError := s.callTool(nil, "tkt_attach_evidence", map[string]any{"ref": "auth-retry-3fa", "claim": "it works", "text": "all green"})
		var doc attachDoc
		if err := json.Unmarshal([]byte(text), &doc); err != nil || isError || doc.Evidence == "" || !doc.Stored || doc.Hash != sha("all green") {
			t.Fatalf("unexpected result (error=%v): %s", isError, text)
		}
		if _, err := os.Stat(filepath.Join(s.root, filepath.FromSlash(doc.Blob))); err != nil {
			t.Errorf("the blob should exist: %v", err)
		}
	})

	t.Run("Commands are not run unless the server allows it", func(t *testing.T) {
		s := evidenceSandbox(t)
		before, _ := os.ReadDir(filepath.Join(s.root, ".tektite", "nodes"))
		for tool, args := range map[string]map[string]any{
			"tkt_attach_evidence": {"ref": "auth-retry-3fa", "claim": "x", "run": "echo hi"},
			"tkt_rerun":           {"ref": "auth-retry-3fa"},
		} {
			text, isError := s.callTool(nil, tool, args)
			if !isError || !strings.Contains(text, "disabled") {
				t.Errorf("%s should be refused, got error=%v: %s", tool, isError, text)
			}
		}
		if after, _ := os.ReadDir(filepath.Join(s.root, ".tektite", "nodes")); len(after) != len(before) {
			t.Errorf("nothing should be written")
		}
		text, isError := s.callTool([]string{"--allow-run"}, "tkt_attach_evidence", map[string]any{"ref": "auth-retry-3fa", "claim": "x", "run": "echo hi"})
		if isError || !strings.Contains(text, "evidence") {
			t.Errorf("--allow-run should enable it, got error=%v: %s", isError, text)
		}
	})
}
