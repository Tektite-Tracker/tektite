package main

import (
	"github.com/spf13/cobra"
)

func (a *app) contextCmd() *cobra.Command {
	var budget int
	var archived, all bool
	cmd := &cobra.Command{
		Use:   "context <ref>",
		Short: "Build a prompt-ready context bundle for a node",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("budget") && budget < 1 {
				return usage("--budget must be at least 1")
			}
			s, err := a.svc()
			if err != nil {
				return err
			}
			b, err := s.Context(args[0], budget, archived || all)
			if err != nil {
				return err
			}
			return a.emit(b, func() { a.printf(a.out, "%s", b.Markdown()) })
		},
	}
	cmd.Flags().IntVar(&budget, "budget", 0, "token budget, defaults to context_budget in .tektite/config or 8000")
	cmd.Flags().BoolVar(&archived, "archived", false, "include archived nodes in the bundle")
	cmd.Flags().BoolVar(&all, "all", false, "include archived nodes in the bundle")
	return cmd
}
