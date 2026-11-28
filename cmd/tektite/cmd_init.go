package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/store"
)

type initResult struct {
	Created bool   `json:"created"`
	Path    string `json:"path"`
}

func (a *app) initCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create .tektite/ in the project directory",
		Args:  exactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			dir := a.root
			if dir == "" {
				dir = a.cwd
			}
			r, created, err := store.Init(dir)
			if err != nil {
				return fmt.Errorf("init: %w", err)
			}
			path := filepath.ToSlash(r.Dir())
			if a.json {
				return a.writeJSON(initResult{Created: created, Path: path})
			}
			if created {
				a.printf(a.out, "initialised %s\n", path)
			} else {
				a.printf(a.out, "already initialised %s\n", path)
			}
			return nil
		},
	}
}
