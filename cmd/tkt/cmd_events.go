package main

import (
	"fmt"
	"io"
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
	if v := strings.TrimSpace(a.gitUser()); v != "" {
		return "human:" + v, nil
	}
	return event.UnknownActor, []string{fmt.Sprintf("%s: no actor given, set --actor or TKT_ACTOR, recorded as %q", parse.CodeMissingActor, event.UnknownActor)}
}

type precheck func(snap *store.Snapshot, nf *store.NodeFile, actor string) ([]string, error)

func (a *app) writeEvent(actorFlag, query string, d *event.Draft, check precheck) (*appended, error) {
	r, snap, err := a.load()
	if err != nil {
		return nil, err
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
	return &appended{Ref: nf.Ref, Event: id, Type: d.Type, Actor: actor, At: parse.FormatTime(d.At), Path: path, Warnings: warnings}, nil
}

func (a *app) report(res *appended) error {
	if a.json {
		return a.writeJSON(res)
	}
	a.warn(res.Warnings)
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
			check := func(snap *store.Snapshot, nf *store.NodeFile, who string) ([]string, error) {
				warnings := []string{}
				if status == event.StatusDone {
					w, err := a.gateDone(snap, nf, who, strings.TrimSpace(override))
					if err != nil {
						return nil, err
					}
					warnings = w
				}
				if !event.KnownStatus(status) {
					warnings = append(warnings, fmt.Sprintf("%s: unknown status %q, expected one of open, active, blocked, review, done, dropped", parse.CodeUnknownStatus, status))
				}
				return warnings, nil
			}
			return a.appendEvent(actor, args[0], &event.Draft{Type: parse.EventSetStatus, Fields: fields}, check)
		},
	}
	addActorFlag(cmd, &actor)
	cmd.Flags().StringVar(&override, "override", "", "reason for overriding a rule")
	addRevisionFlag(cmd, &revision)
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
			return a.appendEvent(actor, args[0], &event.Draft{Type: parse.EventNote, Body: text}, nil)
		},
	}
	addActorFlag(cmd, &actor)
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
	addActorFlag(cmd, &actor)
	return cmd
}
