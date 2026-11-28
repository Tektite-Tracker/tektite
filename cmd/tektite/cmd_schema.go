package main

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

func (a *app) schemaCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Inspect kind schemas",
		Args:  exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "ls",
		Short: "List kind schemas with their source",
		Args:  exactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			r, err := a.repo()
			if err != nil {
				return err
			}
			schemas, err := r.LoadSchemas()
			if err != nil {
				return fmt.Errorf("load schemas: %w", err)
			}
			list := schemas.List()
			if a.json {
				return a.writeJSON(list)
			}
			w := tabwriter.NewWriter(a.out, 0, 4, 2, ' ', 0)
			for _, sc := range list {
				a.printf(w, "%s\t%s\t%s\n", sc.Kind, sc.Source, strings.Join(sc.Required, ","))
			}
			if err := w.Flush(); err != nil {
				return fmt.Errorf("write list: %w", err)
			}
			return nil
		},
	})
	return cmd
}
