package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/bundle"
	"github.com/tektite-tracker/tektite/internal/store"
)

func blobSizer(r *store.Repo) func(string) (int64, bool) {
	return func(attachment string) (int64, bool) {
		name := store.BlobName(attachment)
		if name == "" {
			return 0, false
		}
		info, err := os.Stat(filepath.Join(r.BlobsDir(), name))
		if err != nil {
			return 0, false
		}
		return info.Size(), true
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
			r, snap, err := a.load()
			if err != nil {
				return err
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
			b := bundle.Build(snap, nf, bundle.Options{Budget: budget, Head: a.gitHead(), BlobSize: blobSizer(r)})
			return a.emit(b, func() { a.printf(a.out, "%s", b.Markdown()) })
		},
	}
	cmd.Flags().IntVar(&budget, "budget", 0, "token budget, defaults to context_budget in .tektite/config or 8000")
	return cmd
}
