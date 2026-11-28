package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/view"
)

func (a *app) showCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <ref>",
		Short: "Show a node with its state",
		Args:  exactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			sv, err := a.svc()
			if err != nil {
				return err
			}
			doc, err := sv.Show(args[0])
			if err != nil {
				return err
			}
			return a.emit(doc, func() { a.printShow(doc) })
		},
	}
}

func (a *app) printShow(d view.ShowDoc) {
	a.printf(a.out, "%s  %s  %s\n%s\n", d.Ref, d.Kind, d.Status, d.Title)
	a.printf(a.out, "path: %s\n", d.Path)
	if d.Archived {
		a.printf(a.out, "archived: yes\n")
	}
	if d.Claim != nil {
		state := "expires"
		if !d.Claim.Live {
			state = "expired"
		}
		a.printf(a.out, "claimed by: %s (%s %s)\n", d.Claim.Holder, state, d.Claim.ExpiresAt)
	}
	if d.Revision > 0 {
		a.printf(a.out, "revision: %d\n", d.Revision)
	}
	if d.BuiltAgainst != nil {
		a.printf(a.out, "built against revision: %d\n", *d.BuiltAgainst)
	}
	if d.Parent != "" {
		a.printf(a.out, "parent: %s\n", d.Parent)
	}
	if len(d.Ancestors) > 0 {
		a.printf(a.out, "under: %s\n", strings.Join(reverseCopy(d.Ancestors), " > "))
	}
	if len(d.Children) > 0 {
		a.printf(a.out, "children: %s\n", strings.Join(d.Children, ", "))
	}
	for _, l := range d.Links {
		target := l.Ref
		if target == "" {
			target = l.Target + " (unresolved)"
		}
		a.printf(a.out, "%s: %s\n", l.Rel, target)
	}
	for _, b := range d.Backlinks {
		a.printf(a.out, "%s: %s\n", b.Rel, b.Ref)
	}
	for _, ev := range d.Evidence {
		line := ev.Ref
		if ev.Claim != "" {
			line += "  " + ev.Claim
		}
		if ev.Commit != "" {
			line += "  @" + ev.Commit
		}
		if ev.ExitCode != nil {
			line += fmt.Sprintf("  exit %d", *ev.ExitCode)
		}
		a.printf(a.out, "evidence: %s\n", line)
	}
	for _, g := range d.Gaps {
		state := "open"
		if g.Closed {
			state = "closed"
			if g.Evidence != "" {
				state += " by " + g.Evidence
			}
		}
		a.printf(a.out, "gap %s: %s (%s)\n", g.Block, g.Text, state)
	}
	if d.Body != "" {
		a.printf(a.out, "\n%s\n", d.Body)
	}
	if len(d.Notes) > 0 {
		a.printf(a.out, "\nnotes:\n")
		for _, n := range d.Notes {
			a.printf(a.out, "  %s  %s  %s\n", n.At, n.Actor, strings.ReplaceAll(n.Body, "\n", "\n    "))
		}
	}
	if len(d.History) > 0 {
		a.printf(a.out, "\nhistory:\n")
		for _, h := range d.History {
			line := fmt.Sprintf("  %s  %s  %s", h.At, h.Actor, h.Type)
			if h.Override != "" {
				line += fmt.Sprintf(" (override: %s)", h.Override)
			}
			a.printf(a.out, "%s\n", line)
		}
	}
}

func reverseCopy(in []string) []string {
	out := slices.Clone(in)
	slices.Reverse(out)
	return out
}
