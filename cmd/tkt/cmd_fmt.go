package main

import (
	"strings"

	"github.com/spf13/cobra"
)

func (a *app) fmtCmd() *cobra.Command {
	var dry bool
	cmd := &cobra.Command{
		Use:   "fmt",
		Short: "Assign missing ids and normalise frontmatter",
		Args:  exactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			svc, err := a.svc()
			if err != nil {
				return err
			}
			items, err := svc.Format(dry)
			if err != nil {
				return err
			}
			return a.emit(items, func() {
				for _, it := range items {
					switch {
					case it.Skipped != "":
						a.printf(a.out, "skipped %s: %s\n", it.Path, it.Skipped)
					case it.Written:
						a.printf(a.out, "updated %s: %s\n", it.Path, strings.Join(it.Changes, ", "))
					default:
						a.printf(a.out, "would update %s: %s\n", it.Path, strings.Join(it.Changes, ", "))
					}
				}
			})
		},
	}
	cmd.Flags().BoolVar(&dry, "dry-run", false, "show the changes without writing")
	return cmd
}
