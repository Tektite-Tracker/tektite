package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	agentSnippetMarker = "<!-- tektite:agent-onboarding -->"
	agentSnippet       = agentSnippetMarker + "\n## Tektite\n\nThis repo tracks work with tektite. Run `tektite prime` for the agent workflow and exit codes, then `tektite next --claim --actor agent:<model>/<session>` to take a ticket.\n"
)

var agentFiles = []string{"AGENTS.md", "CLAUDE.md"}

type SnippetResult struct {
	Path    string `json:"path"`
	Written bool   `json:"written"`
	Created bool   `json:"created"`
}

func AgentSnippetTarget(dir string) (string, bool, error) {
	for _, name := range agentFiles {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err == nil && strings.Contains(string(b), agentSnippetMarker) {
			return name, true, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return "", false, fmt.Errorf("agent snippet: %w", err)
		}
	}
	for _, name := range agentFiles {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return name, false, nil
		}
	}
	return agentFiles[0], false, nil
}

func AddAgentSnippet(dir string) (*SnippetResult, error) {
	name, present, err := AgentSnippetTarget(dir)
	if err != nil {
		return nil, err
	}
	res := &SnippetResult{Path: name}
	if present {
		return res, nil
	}
	path := filepath.Join(dir, name)
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("agent snippet: %w", err)
	}
	res.Created = os.IsNotExist(err)
	content := string(existing)
	if content != "" {
		if !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		content += "\n"
	}
	content += agentSnippet
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return nil, fmt.Errorf("agent snippet: %w", err)
	}
	res.Written = true
	return res, nil
}
