package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/internal/node"
	"github.com/tektite-tracker/tektite/internal/store"
	"github.com/tektite-tracker/tektite/parse"
)

type revisionItem struct {
	Number   int    `json:"number"`
	Event    string `json:"event"`
	Actor    string `json:"actor"`
	At       string `json:"at"`
	Bytes    int    `json:"bytes"`
	Current  bool   `json:"current"`
	Diverged bool   `json:"diverged"`
}

type revisedResult struct {
	*appended
	Number int `json:"number"`
}

func checkRevisionFlag(cmd *cobra.Command, revision int) error {
	if cmd.Flags().Changed("revision") && revision < 1 {
		return usage("--revision must be at least 1")
	}
	return nil
}

func (a *app) reviseCmd() *cobra.Command {
	var file, actor string
	cmd := &cobra.Command{
		Use:   "revise <ref>",
		Short: "Record a new revision of a plan with a full body snapshot",
		Args:  exactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			body, err := a.readRevisionBody(file)
			if err != nil {
				return err
			}
			d := &event.Draft{Type: parse.EventRevision, Body: body}
			number := 0
			check := func(snap *store.Snapshot, nf *store.NodeFile, _ string) ([]string, error) {
				number = a.held(snap, nf.Ref).NextRevision()
				d.Fields = []node.Field{{Key: "number", Value: number}}
				return nil, nil
			}
			res, err := a.writeEvent(actor, args[0], d, check)
			if err != nil {
				return err
			}
			if a.json {
				return a.writeJSON(revisedResult{appended: res, Number: number})
			}
			a.warn(res.Warnings)
			a.printf(a.out, "%s revision %d\n", res.Ref, number)
			return nil
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "file with the new body, or - for stdin, which is the default")
	addActorFlag(cmd, &actor)
	return cmd
}

func (a *app) readRevisionBody(file string) (string, error) {
	var data []byte
	var err error
	switch {
	case file != "" && file != "-":
		data, err = a.readFile(file)
	case file == "" && a.interactive:
		return "", usage("pass --file or pipe the new body on stdin")
	default:
		data, err = io.ReadAll(a.in)
	}
	if err != nil {
		return "", fmt.Errorf("read revision body: %w", err)
	}
	body := strings.TrimSpace(string(data))
	if body == "" {
		return "", usage("the revision body is empty")
	}
	return body, nil
}

func (a *app) revisionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "revisions <ref>",
		Short: "List the revisions of a plan",
		Args:  exactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			_, st, nf, err := a.loadState(args[0])
			if err != nil {
				return err
			}
			items := []revisionItem{}
			for _, r := range st.Revisions {
				items = append(items, revisionItem{
					Number: r.Number, Event: r.Event, Actor: r.Actor, At: parse.FormatTime(r.At),
					Bytes: len(r.Body), Current: r.Event == currentRevisionEvent(st), Diverged: st.Diverged(r.Number),
				})
			}
			if a.json {
				return a.writeJSON(items)
			}
			if len(items) == 0 {
				a.printf(a.out, "%s has no revisions\n", nf.Ref)
				return nil
			}
			for _, it := range items {
				flags := ""
				if it.Current {
					flags += "  current"
				}
				if it.Diverged {
					flags += "  diverged"
				}
				a.printf(a.out, "%d  %s  %s  %d bytes%s\n", it.Number, it.At, it.Actor, it.Bytes, flags)
			}
			return nil
		},
	}
}

func currentRevisionEvent(st event.State) string {
	if r, ok := st.RevisionNumbered(st.Revision); ok {
		return r.Event
	}
	return ""
}

func (a *app) diffCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "diff <ref> <rev-a> <rev-b>",
		Short: "Show a unified diff between two revisions of a plan",
		Args:  exactArgs(3),
		RunE: func(_ *cobra.Command, args []string) error {
			_, st, nf, err := a.loadState(args[0])
			if err != nil {
				return err
			}
			var revs [2]event.Revision
			for i, raw := range args[1:] {
				n, convErr := strconv.Atoi(strings.TrimSpace(raw))
				if convErr != nil || n < 1 {
					return usage("revision %q is not a positive number", raw)
				}
				r, ok := st.RevisionNumbered(n)
				if !ok {
					return fail(exitNotFound, fmt.Sprintf("run `tkt revisions %s` to list them", nf.Ref), "%s has no revision %d", nf.Ref, n)
				}
				revs[i] = r
			}
			text := event.UnifiedDiff(fmt.Sprintf("%s revision %d", nf.Ref, revs[0].Number), fmt.Sprintf("%s revision %d", nf.Ref, revs[1].Number), revs[0].Body, revs[1].Body)
			if a.json {
				return a.writeJSON(map[string]any{"ref": nf.Ref, "from": revs[0].Number, "to": revs[1].Number, "diff": text})
			}
			a.printf(a.out, "%s", text)
			return nil
		},
	}
}

func (a *app) loadState(query string) (*store.Snapshot, event.State, *store.NodeFile, error) {
	r, err := a.repo()
	if err != nil {
		return nil, event.State{}, nil, err
	}
	snap, err := r.Load()
	if err != nil {
		return nil, event.State{}, nil, fmt.Errorf("load repository: %w", err)
	}
	nf, err := snap.Resolve(query)
	if err != nil {
		return nil, event.State{}, nil, fmt.Errorf("revisions: %w", err)
	}
	return snap, a.held(snap, nf.Ref), nf, nil
}
