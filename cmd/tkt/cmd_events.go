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

type precheck func(snap *store.Snapshot, nf *store.NodeFile, actor string) ([]string, error)

func (a *app) writeEvent(actorFlag, query string, d *event.Draft, check precheck) (*appended, error) {
	r, err := a.repo()
	if err != nil {
		return nil, err
	}
	snap, err := r.Load()
	if err != nil {
		return nil, fmt.Errorf("load repository: %w", err)
	}
	nf, err := snap.Resolve(query)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", d.Type, err)
	}
	actor, warnings := a.resolveActor(actorFlag)
	d.Actor, d.At = actor, a.now()
	if check != nil {
		extra, err := check(snap, nf, actor)
		if err != nil {
			return nil, err
		}
		warnings = append(extra, warnings...)
	}
	id, err := node.NewID(d.At, a.rand)
	if err != nil {
		return nil, fmt.Errorf("new id: %w", err)
	}
	data, err := event.Render(*d)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", d.Type, err)
	}
	path, err := r.WriteEvent(nf.Ref, id, data)
	if err != nil {
		return nil, fmt.Errorf("append %s event: %w", d.Type, err)
	}
	return &appended{Ref: nf.Ref, Event: id, Type: d.Type, Actor: actor, At: d.At.UTC().Format("2006-01-02T15:04:05Z"), Path: path, Warnings: warnings}, nil
}

func (a *app) report(res *appended) error {
	if a.json {
		return a.writeJSON(res)
	}
	for _, w := range res.Warnings {
		a.printf(a.err, "warning: %s\n", w)
	}
	a.printf(a.out, "%s %s %s\n", res.Ref, res.Type, res.Event)
	return nil
}

func (a *app) appendEvent(actorFlag, query string, d *event.Draft, check precheck) error {
	res, err := a.writeEvent(actorFlag, query, d, check)
	if err != nil {
		return err
	}
	return a.report(res)
}

func (a *app) setStatusCmd() *cobra.Command {
	var actor, override string
	var revision int
	cmd := &cobra.Command{
		Use:   "set-status <ref> <status>",
		Short: "Set the status of a node",
		Args:  exactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := checkRevisionFlag(cmd, revision); err != nil {
				return err
			}
			status := strings.TrimSpace(args[1])
			if status == "" {
				return usage("status must not be empty")
			}
			fields := []node.Field{{Key: "status", Value: status}}
			if strings.TrimSpace(override) != "" {
				fields = append(fields, node.Field{Key: "override", Value: strings.TrimSpace(override)})
			}
			if revision > 0 {
				fields = append(fields, node.Field{Key: "revision", Value: revision})
			}
			check := func(*store.Snapshot, *store.NodeFile, string) ([]string, error) {
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
	cmd.Flags().IntVar(&revision, "revision", 0, "plan revision the work is built against")
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
			check := func(snap *store.Snapshot, _ *store.NodeFile, _ string) ([]string, error) {
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
