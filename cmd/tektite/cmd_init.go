package main

import (
	"bufio"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/service"
	"github.com/tektite-tracker/tektite/internal/store"
)

type initResult struct {
	Created bool   `json:"created"`
	Path    string `json:"path"`

	AgentSnippet *service.SnippetResult `json:"agent_snippet,omitempty"`
}

func (a *app) initCmd() *cobra.Command {
	var addSnippet, skipSnippet bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create .tektite/ in the project directory",
		Args:  exactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			if addSnippet && skipSnippet {
				return usage("--agent-snippet and --no-agent-snippet cannot be used together")
			}
			dir := a.root
			if dir == "" {
				dir = a.cwd
			}
			r, created, err := store.Init(dir)
			if err != nil {
				return fmt.Errorf("init: %w", err)
			}
			path := filepath.ToSlash(r.Dir())
			snippet, err := a.maybeAddSnippet(dir, addSnippet, skipSnippet)
			if err != nil {
				return err
			}
			if a.json {
				return a.writeJSON(initResult{Created: created, Path: path, AgentSnippet: snippet})
			}
			if created {
				a.printf(a.out, "initialised %s\n", path)
			} else {
				a.printf(a.out, "already initialised %s\n", path)
			}
			if snippet != nil && snippet.Written {
				a.printf(a.out, "added agent snippet to %s\n", snippet.Path)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&addSnippet, "agent-snippet", false, "add the agent onboarding snippet without asking")
	cmd.Flags().BoolVar(&skipSnippet, "no-agent-snippet", false, "do not add or offer the agent onboarding snippet")
	return cmd
}

func (a *app) maybeAddSnippet(dir string, add, skip bool) (*service.SnippetResult, error) {
	if skip {
		return nil, nil
	}
	if !add {
		if !a.interactive {
			return nil, nil
		}
		name, present, err := service.AgentSnippetTarget(dir)
		if err != nil || present {
			return nil, err
		}
		a.printf(a.err, "Add a tektite snippet for agents to %s? [y/N] ", name)
		line, _ := bufio.NewReader(a.in).ReadString('\n')
		if ans := strings.ToLower(strings.TrimSpace(line)); ans != "y" && ans != "yes" {
			return nil, nil
		}
	}
	return service.AddAgentSnippet(dir)
}
