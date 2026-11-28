package main

import (
	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/service"
)

func (a *app) importCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import issues from another tracker's file export, once and one way",
	}
	cmd.AddCommand(a.importBeadsCmd())
	return cmd
}

func (a *app) importBeadsCmd() *cobra.Command {
	var from, actor string
	var dry bool
	cmd := &cobra.Command{
		Use:   "beads",
		Short: "Import a Beads issues.jsonl export into nodes",
		Args:  exactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			svc, err := a.svc()
			if err != nil {
				return err
			}
			res, err := svc.MigrateBeads(service.MigrateBeadsInput{Path: from, DryRun: dry, Actor: actor})
			if err != nil {
				return err
			}
			return a.emit(res, func() {
				for _, it := range res.Items {
					verb := it.Action
					if verb == "create" && res.DryRun {
						verb = "would create"
					}
					a.printf(a.out, "%s %s %s %s\n", verb, it.BeadsID, it.Kind, it.Title)
				}
				a.printf(a.out, "%d created, %d skipped, %d links\n", res.Created, res.Skipped, res.Links)
				a.warn(warningTexts(res.Warnings))
			})
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "path to the beads export (default .beads/issues.jsonl next to .tektite/)")
	cmd.Flags().BoolVar(&dry, "dry-run", false, "list what would be imported without writing")
	cmd.Flags().StringVar(&actor, "actor", "", "actor recorded on imported events")
	return cmd
}
