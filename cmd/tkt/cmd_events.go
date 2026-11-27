package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/service"
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

func toAppended(res *service.Appended) *appended {
	return &appended{Ref: res.Ref, Event: res.Event, Type: res.Type, Actor: res.Actor, At: res.At, Path: res.Path, Warnings: warningTexts(res.Warnings)}
}

func (a *app) report(res *appended) error {
	return a.emit(res, func() {
		a.warn(res.Warnings)
		a.printf(a.out, "%s %s %s\n", res.Ref, res.Type, res.Event)
	})
}

func (a *app) reportEvent(res *service.Appended, err error) error {
	if err != nil {
		return err
	}
	return a.report(toAppended(res))
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
			svc, err := a.svc()
			if err != nil {
				return err
			}
			return a.reportEvent(svc.SetStatus(service.SetStatusInput{Ref: args[0], Status: args[1], Actor: actor, Override: override, Revision: revision}))
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
			svc, err := a.svc()
			if err != nil {
				return err
			}
			return a.reportEvent(svc.Note(service.NoteInput{Ref: args[0], Text: text, Actor: actor}))
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
			svc, err := a.svc()
			if err != nil {
				return err
			}
			return a.reportEvent(svc.Link(service.LinkInput{Verb: typ, Ref: args[0], Rel: args[1], Target: args[2], Actor: actor}))
		},
	}
	addActorFlag(cmd, &actor)
	return cmd
}

func (a *app) closeGapCmd() *cobra.Command {
	var actor, evidence string
	cmd := &cobra.Command{
		Use:   "close-gap <ref>#<block-id>",
		Short: "Close a gap block with evidence",
		Args:  exactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			svc, err := a.svc()
			if err != nil {
				return err
			}
			return a.reportEvent(svc.CloseGap(service.CloseGapInput{Ref: args[0], Evidence: evidence, Actor: actor}))
		},
	}
	addActorFlag(cmd, &actor)
	cmd.Flags().StringVar(&evidence, "evidence", "", "evidence node that proves the gap is closed")
	return cmd
}
