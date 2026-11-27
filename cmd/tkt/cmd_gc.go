package main

import (
	"github.com/spf13/cobra"
)

type gcResult struct {
	Removed []string `json:"removed"`
	DryRun  bool     `json:"dry_run"`
}

func (a *app) gcCmd() *cobra.Command {
	var blobs, dry bool
	cmd := &cobra.Command{
		Use:   "gc",
		Short: "Remove data nothing refers to anymore",
		Args:  exactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			if !blobs {
				return usage("nothing to collect, pass --blobs to remove unreferenced blobs")
			}
			svc, err := a.svc()
			if err != nil {
				return err
			}
			res, err := svc.GCBlobs(dry)
			if err != nil {
				return err
			}
			return a.emit(gcResult{Removed: res.Removed, DryRun: res.DryRun}, func() {
				verb := "removed"
				if res.DryRun {
					verb = "would remove"
				}
				for _, name := range res.Removed {
					a.printf(a.out, "%s blobs/%s\n", verb, name)
				}
			})
		},
	}
	cmd.Flags().BoolVar(&blobs, "blobs", false, "remove blobs that no node references")
	cmd.Flags().BoolVar(&dry, "dry-run", false, "list what would be removed without removing it")
	return cmd
}
