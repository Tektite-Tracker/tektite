package main

import (
	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/mcp"
)

var version = "dev"

func (a *app) mcpCmd() *cobra.Command {
	var allowRun bool
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Serve tektite to agents over the Model Context Protocol on stdio",
		Args:  exactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			srv := mcp.New(mcp.Options{Name: "tkt", Version: version, Open: a.svc, AllowRun: allowRun})
			return srv.Serve(a.in, a.out)
		},
	}
	cmd.Flags().BoolVar(&allowRun, "allow-run", false, "let tools run shell commands to record evidence")
	return cmd
}
