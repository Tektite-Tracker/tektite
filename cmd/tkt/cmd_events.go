package main

import (
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/internal/node"
	"github.com/tektite-tracker/tektite/internal/store"
	"github.com/tektite-tracker/tektite/parse"
)

type appended struct {
	Ref      string   `json:"ref"`
	Event    string   `json:"event"`
	Type     string   `json:"type"`
	Actor    string   `json:"actor"`
	At       string   `json:"at"`
	Path     string   `json:"path"`
	Warnings []string `json:"warnings"`
}

func (a *app) resolveActor(flag string) (string, []string) {
	if v := strings.TrimSpace(flag); v != "" {
		return v, nil
	}
	if v := strings.TrimSpace(a.getenv("TKT_ACTOR")); v != "" {
		return v, nil
	}
	if a.gitUser != nil {
		if v := strings.TrimSpace(a.gitUser()); v != "" {
			return "human:" + v, nil
		}
	}
	return event.UnknownActor, []string{fmt.Sprintf("%s: no actor given, set --actor or TKT_ACTOR, recorded as %q", parse.CodeMissingActor, event.UnknownActor)}
}

func gitUserName(dir string) func() string {
	return func() string {
		c := exec.Command("git", "config", "user.name")
		c.Dir = dir
		out, err := c.Output()
		if err != nil {
			return ""
		}
		return string(out)
	}
}

func (a *app) appendEvent(actorFlag, query string, d *event.Draft, check func(snap *store.Snapshot) ([]string, error)) error {
	r, err := a.repo()
	if err != nil {
		return err
	}
	snap, err := r.Load()
	if err != nil {
		return fmt.Errorf("load repository: %w", err)
	}
	nf, err := snap.Resolve(query)
	if err != nil {
		return fmt.Errorf("%s: %w", d.Type, err)
	}
	warnings := []string{}
	if check != nil {
		if warnings, err = check(snap); err != nil {
			return err
		}
	}
	actor, actorWarn := a.resolveActor(actorFlag)
	warnings = append(warnings, actorWarn...)
	d.Actor, d.At = actor, a.now()
	id, err := node.NewID(d.At, a.rand)
	if err != nil {
		return fmt.Errorf("new id: %w", err)
	}
	data, err := event.Render(*d)
	if err != nil {
		return fmt.Errorf("%s: %w", d.Type, err)
	}
	path, err := r.WriteEvent(nf.Ref, id, data)
	if err != nil {
		return fmt.Errorf("append %s event: %w", d.Type, err)
	}
	res := appended{Ref: nf.Ref, Event: id, Type: d.Type, Actor: actor, At: d.At.UTC().Format("2006-01-02T15:04:05Z"), Path: path, Warnings: warnings}
	if a.json {
		return a.writeJSON(res)
	}
	for _, w := range warnings {
		a.printf(a.err, "warning: %s\n", w)
	}
	a.printf(a.out, "%s %s %s\n", nf.Ref, d.Type, id)
	return nil
}

func (a *app) setStatusCmd() *cobra.Command {
	var actor, override string
	cmd := &cobra.Command{
		Use:   "set-status <ref> <status>",
		Short: "Set the status of a node",
		Args:  exactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			status := strings.TrimSpace(args[1])
			if status == "" {
				return usage("status must not be empty")
			}
			fields := []node.Field{{Key: "status", Value: status}}
			if strings.TrimSpace(override) != "" {
				fields = append(fields, node.Field{Key: "override", Value: strings.TrimSpace(override)})
			}
			check := func(*store.Snapshot) ([]string, error) {
				if event.KnownStatus(status) {
					return nil, nil
				}
				return []string{fmt.Sprintf("%s: unknown status %q, expected one of open, active, blocked, review, done, dropped", parse.CodeUnknownStatus, status)}, nil
			}
			return a.appendEvent(actor, args[0], &event.Draft{Type: "set-status", Fields: fields}, check)
		},
	}
	cmd.Flags().StringVar(&actor, "actor", "", "actor recorded on the event")
	cmd.Flags().StringVar(&override, "override", "", "reason for overriding a rule")
	return cmd
}

func (a *app) noteCmd() *cobra.Command {
	var actor string
	cmd := &cobra.Command{
		Use:   "note <ref> <text>",
		Short: "Add a note to a node",
		Args:  exactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			text := args[1]
			if text == "-" {
				data, err := io.ReadAll(a.in)
				if err != nil {
					return fmt.Errorf("read note from stdin: %w", err)
				}
				text = string(data)
			}
			if strings.TrimSpace(text) == "" {
				return usage("note text must not be empty")
			}
			return a.appendEvent(actor, args[0], &event.Draft{Type: "note", Body: text}, nil)
		},
	}
	cmd.Flags().StringVar(&actor, "actor", "", "actor recorded on the event")
	return cmd
}

func (a *app) linkCmd(typ, short string) *cobra.Command {
	var actor string
	cmd := &cobra.Command{
		Use:   typ + " <ref> <rel> <target>",
		Short: short,
		Args:  exactArgs(3),
		RunE: func(_ *cobra.Command, args []string) error {
			rel, target := strings.TrimSpace(args[1]), strings.TrimSpace(args[2])
			if rel == "" || target == "" {
				return usage("rel and target must not be empty")
			}
			d := &event.Draft{Type: typ}
			check := func(snap *store.Snapshot) ([]string, error) {
				warnings := []string{}
				if t, err := snap.Resolve(target); err == nil {
					target = t.Ref
				} else {
					warnings = append(warnings, fmt.Sprintf("target %q matches no node", target))
				}
				d.Fields = []node.Field{{Key: "rel", Value: rel}, {Key: "target", Value: target}}
				return warnings, nil
			}
			return a.appendEvent(actor, args[0], d, check)
		},
	}
	cmd.Flags().StringVar(&actor, "actor", "", "actor recorded on the event")
	return cmd
}
