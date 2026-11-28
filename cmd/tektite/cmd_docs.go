package main

import (
	"errors"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/docs"
)

func (a *app) docsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "docs [topic]",
		Short: "List documentation topics or print one",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 1 {
				return a.docsTopic(args[0])
			}
			topics, err := docs.List()
			if err != nil {
				return fmt.Errorf("list docs: %w", err)
			}
			if a.json {
				return a.writeJSON(topics)
			}
			w := tabwriter.NewWriter(a.out, 0, 4, 2, ' ', 0)
			for _, t := range topics {
				a.printf(w, "%s\t%s\n", t.Topic, t.Title)
			}
			if err := w.Flush(); err != nil {
				return fmt.Errorf("write list: %w", err)
			}
			return nil
		},
	}
}

func (a *app) docsTopic(topic string) error {
	body, err := docs.Read(topic)
	if errors.Is(err, docs.ErrNotFound) {
		return fail(exitNotFound, "run `tektite docs` to list the topics", "%s", err)
	}
	if err != nil {
		return fmt.Errorf("read docs: %w", err)
	}
	a.printf(a.out, "%s", body)
	return nil
}
