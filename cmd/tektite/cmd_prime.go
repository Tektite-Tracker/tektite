package main

import (
	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/view"
)

func (a *app) primeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "prime",
		Short: "Print the agent briefing: workflow steps and exit codes",
		Args:  exactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			b := view.NewBriefing()
			if a.json {
				return a.writeJSON(b)
			}
			a.printf(a.out, "%s", b.Text())
			return nil
		},
	}
}
