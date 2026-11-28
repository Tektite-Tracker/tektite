package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/service"
)

func (a *app) newCmd() *cobra.Command {
	var parent, body string
	var sets []string
	cmd := &cobra.Command{
		Use:   "new <kind> <title>",
		Short: "Create a node",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.create(args[0], args[1], parent, body, cmd.Flags().Changed("body"), sets)
		},
	}
	cmd.Flags().StringVar(&parent, "parent", "", "reference of the parent node")
	cmd.Flags().StringVar(&body, "body", "", "body text, or - to read it from stdin")
	cmd.Flags().StringArrayVar(&sets, "set", nil, "extra frontmatter field as key=value, repeatable")
	return cmd
}

func (a *app) create(kind, title, parent, body string, bodySet bool, sets []string) error {
	in := service.CreateInput{Kind: kind, Title: title, Parent: parent, Sets: sets}
	switch {
	case bodySet && body == "-":
		data, err := io.ReadAll(a.in)
		if err != nil {
			return fmt.Errorf("read body from stdin: %w", err)
		}
		text := string(data)
		in.Body = &text
	case bodySet:
		in.Body = &body
	case a.interactive && a.getenv("EDITOR") != "":
		in.Edit = a.edit
	}
	svc, err := a.svc()
	if err != nil {
		return err
	}
	created, err := svc.Create(in)
	if err != nil {
		return err
	}
	for _, f := range created.MissingRequired {
		a.printf(a.err, "note: field %q is required by the %s schema, set it with --set %s=<value>\n", f, created.Kind, f)
	}
	return a.printCreated(created)
}

type createdResult struct {
	ID              string   `json:"id"`
	Kind            string   `json:"kind"`
	MissingRequired []string `json:"missing_required"`
	Parent          string   `json:"parent"`
	Path            string   `json:"path"`
	Ref             string   `json:"ref"`
	Title           string   `json:"title"`
}

func (a *app) printCreated(c *service.Created) error {
	res := createdResult{ID: c.ID, Kind: c.Kind, MissingRequired: c.MissingRequired, Parent: c.Parent, Path: c.Path, Ref: c.Ref, Title: c.Title}
	return a.emit(res, func() { a.printf(a.out, "%s\n", c.Ref) })
}

func (a *app) edit(initial string) (string, error) {
	parts := strings.Fields(a.getenv("EDITOR"))
	if len(parts) == 0 {
		return "", usage("EDITOR is set but empty, set EDITOR to your editor command or pass --body")
	}
	f, err := os.CreateTemp("", "tektite-*.md")
	if err != nil {
		return "", fmt.Errorf("create editor file: %w", err)
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err := f.WriteString(initial); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("write editor file: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("close editor file: %w", err)
	}
	c := exec.Command(parts[0], append(parts[1:], f.Name())...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := c.Run(); err != nil {
		return "", fmt.Errorf("run editor: %w", err)
	}
	data, err := os.ReadFile(f.Name())
	if err != nil {
		return "", fmt.Errorf("read editor file: %w", err)
	}
	return string(data), nil
}
