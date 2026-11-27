package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/tektite-tracker/tektite/internal/node"
)

var (
	fieldKey   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)
	coreFields = map[string]bool{"id": true, "kind": true, "title": true, "parent": true}
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

type setField struct {
	key   string
	value any
}

func parseSets(sets []string) ([]setField, error) {
	out := []setField{}
	for _, s := range sets {
		key, raw, ok := strings.Cut(s, "=")
		key = strings.TrimSpace(key)
		if !ok || !fieldKey.MatchString(key) {
			return nil, usage("--set expects key=value, got %q", s)
		}
		if coreFields[key] {
			return nil, usage("--set cannot change %q", key)
		}
		var v any
		if err := yaml.Unmarshal([]byte(raw), &v); err != nil || v == nil {
			v = raw
		}
		out = append(out, setField{key: key, value: v})
	}
	return out, nil
}

func (a *app) create(kind, title, parent, body string, bodySet bool, sets []string) error {
	kind, title = strings.TrimSpace(kind), strings.TrimSpace(title)
	if kind == "" || title == "" {
		return usage("kind and title must not be empty")
	}
	extra, err := parseSets(sets)
	if err != nil {
		return err
	}
	r, err := a.repo()
	if err != nil {
		return err
	}
	snap, err := r.Load()
	if err != nil {
		return fmt.Errorf("load repository: %w", err)
	}
	schemas, err := r.LoadSchemas()
	if err != nil {
		return fmt.Errorf("load schemas: %w", err)
	}
	parentRef := ""
	if strings.TrimSpace(parent) != "" {
		p, err := snap.Resolve(parent)
		if err != nil {
			return fmt.Errorf("parent: %w", err)
		}
		parentRef = p.Ref
	}
	schema, hasSchema := schemas.For(kind)
	text := ""
	switch {
	case bodySet && body == "-":
		data, err := io.ReadAll(a.in)
		if err != nil {
			return fmt.Errorf("read body from stdin: %w", err)
		}
		text = string(data)
	case bodySet:
		text = body
	case hasSchema:
		text = schema.Template
	}
	if !bodySet && a.interactive && a.getenv("EDITOR") != "" {
		if text, err = a.edit(text); err != nil {
			return err
		}
	}

	id, err := node.NewID(a.now(), a.rand)
	if err != nil {
		return fmt.Errorf("new id: %w", err)
	}
	ref, err := node.NewRef(title, id, snap.Refs())
	if err != nil {
		return fmt.Errorf("new reference: %w", err)
	}
	fields := []node.Field{{Key: "id", Value: id}, {Key: "kind", Value: kind}, {Key: "title", Value: title}, {Key: "parent", Value: parentRef}}
	present := map[string]bool{}
	for _, f := range extra {
		fields = append(fields, node.Field{Key: f.key, Value: f.value})
		present[f.key] = true
	}
	text = strings.TrimSpace(text)
	if text != "" {
		text += "\n"
	}
	data, err := node.Render(fields, text)
	if err != nil {
		return fmt.Errorf("render node: %w", err)
	}
	path, err := r.WriteNode(ref, data)
	if err != nil {
		return fmt.Errorf("create node: %w", err)
	}

	missing := []string{}
	if hasSchema {
		for _, f := range schema.Required {
			if !present[f] && !coreFields[f] {
				missing = append(missing, f)
			}
		}
	}
	for _, f := range missing {
		a.printf(a.err, "note: field %q is required by the %s schema, set it with --set %s=<value>\n", f, kind, f)
	}
	return a.printCreated(ref, id, kind, title, parentRef, path, missing)
}

func (a *app) printCreated(ref, id, kind, title, parent, path string, missing []string) error {
	if a.json {
		return a.writeJSON(map[string]any{
			"ref": ref, "id": id, "kind": kind, "title": title, "parent": parent, "path": path, "missing_required": missing,
		})
	}
	a.printf(a.out, "%s\n", ref)
	return nil
}

func (a *app) edit(initial string) (string, error) {
	f, err := os.CreateTemp("", "tkt-*.md")
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
	parts := strings.Fields(a.getenv("EDITOR"))
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
