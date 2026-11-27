package main

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/bundle"
)

func gitHead(dir string) func() string {
	return func() string {
		c := exec.Command("git", "rev-parse", "HEAD")
		c.Dir = dir
		out, err := c.Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
}

func (a *app) contextCmd() *cobra.Command {
	var budget int
	cmd := &cobra.Command{
		Use:   "context <ref>",
		Short: "Build a prompt-ready context bundle for a node",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("budget") && budget < 1 {
				return usage("--budget must be at least 1")
			}
			r, err := a.repo()
			if err != nil {
				return err
			}
			snap, err := r.Load()
			if err != nil {
				return fmt.Errorf("load repository: %w", err)
			}
			nf, err := snap.Resolve(args[0])
			if err != nil {
				return fmt.Errorf("context: %w", err)
			}
			if !cmd.Flags().Changed("budget") {
				cfg, err := r.Config()
				if err != nil {
					return fmt.Errorf("context: %w", err)
				}
				budget = cfg.ContextBudget
			}
			head := ""
			if a.gitHead != nil {
				head = a.gitHead()
			}
			b := bundle.Build(snap, nf, bundle.Options{Budget: budget, Head: head})
			if a.json {
				return a.writeJSON(b)
			}
			a.printf(a.out, "%s", b.Markdown())
			return nil
		},
	}
	cmd.Flags().IntVar(&budget, "budget", 0, "token budget, defaults to context_budget in .tektite/config or 8000")
	return cmd
}
