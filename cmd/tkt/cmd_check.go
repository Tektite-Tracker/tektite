package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/check"
)

func (a *app) checkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Report problems in the repository",
		Args:  exactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			r, err := a.repo()
			if err != nil {
				return err
			}
			rep, err := check.Run(r)
			if err != nil {
				return fmt.Errorf("check: %w", err)
			}
			if a.json {
				if err := a.writeJSON(rep); err != nil {
					return err
				}
			} else {
				a.printReport(rep)
			}
			if rep.HasErrors() {
				return exitSilently(exitError)
			}
			return nil
		},
	}
}

func (a *app) printReport(rep check.Report) {
	for _, p := range rep.Problems {
		loc := p.Path
		if p.Line > 0 {
			loc = fmt.Sprintf("%s:%d", p.Path, p.Line)
		}
		a.printf(a.out, "%s: %s %s %s\n", loc, p.Severity, p.Code, p.Message)
		if p.Hint != "" {
			a.printf(a.out, "  hint: %s\n", p.Hint)
		}
	}
	a.printf(a.out, "%d errors, %d warnings, %d notes\n", rep.Errors, rep.Warnings, rep.Notes)
}
