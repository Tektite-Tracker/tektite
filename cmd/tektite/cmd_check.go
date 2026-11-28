package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/check"
)

func (a *app) checkCmd() *cobra.Command {
	var stale, archived, all bool
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Report problems in the repository",
		Args:  exactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			svc, err := a.svc()
			if err != nil {
				return err
			}
			rep, err := svc.Check(stale, archived || all)
			if err != nil {
				return err
			}
			if err := a.emit(rep, func() { a.printReport(rep) }); err != nil {
				return err
			}
			if rep.HasErrors() {
				return exitSilently(exitError)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&stale, "stale", false, "report evidence that later commits may have outdated")
	cmd.Flags().BoolVar(&archived, "archived", false, "also check archived nodes")
	cmd.Flags().BoolVar(&all, "all", false, "also check archived nodes")
	return cmd
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
