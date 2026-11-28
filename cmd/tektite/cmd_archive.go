package main

import (
	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/service"
)

func (a *app) archiveCmd() *cobra.Command {
	var done, dry bool
	var olderThan string
	cmd := &cobra.Command{
		Use:   "archive <ref>... | --done [--older-than <duration>]",
		Short: "Move done and dropped nodes into .tektite/archive/",
		RunE: func(_ *cobra.Command, args []string) error {
			svc, err := a.svc()
			if err != nil {
				return err
			}
			res, err := svc.Archive(service.ArchiveInput{Refs: args, Done: done, OlderThan: olderThan, DryRun: dry})
			if err != nil {
				return err
			}
			return a.emit(res, func() {
				verb := "archived"
				if res.DryRun {
					verb = "would archive"
				}
				for _, n := range res.Archived {
					a.printf(a.out, "%s %s\n", verb, n.Ref)
				}
				for _, sk := range res.Skipped {
					a.printf(a.out, "skipped %s: %s\n", sk.Ref, sk.Reason)
				}
			})
		},
	}
	cmd.Flags().BoolVar(&done, "done", false, "select every done or dropped node")
	cmd.Flags().StringVar(&olderThan, "older-than", "", "with --done, only nodes whose last event is older than this, such as 30d, 2w or 720h")
	cmd.Flags().BoolVar(&dry, "dry-run", false, "list what would move without moving it")
	return cmd
}

func (a *app) unarchiveCmd() *cobra.Command {
	var dry bool
	cmd := &cobra.Command{
		Use:   "unarchive <ref>...",
		Short: "Move archived nodes back into the active set",
		RunE: func(_ *cobra.Command, args []string) error {
			svc, err := a.svc()
			if err != nil {
				return err
			}
			res, err := svc.Unarchive(args, dry)
			if err != nil {
				return err
			}
			return a.emit(res, func() {
				verb := "unarchived"
				if res.DryRun {
					verb = "would unarchive"
				}
				for _, n := range res.Unarchived {
					a.printf(a.out, "%s %s\n", verb, n.Ref)
				}
			})
		},
	}
	cmd.Flags().BoolVar(&dry, "dry-run", false, "list what would move without moving it")
	return cmd
}
