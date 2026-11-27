package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/internal/store"
	"github.com/tektite-tracker/tektite/parse"
)

type historyItem struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Actor   string `json:"actor"`
	At      string `json:"at"`
	Applied bool   `json:"applied"`
}

type noteItem struct {
	ID    string `json:"id"`
	Actor string `json:"actor"`
	At    string `json:"at"`
	Body  string `json:"body"`
}

type linkItem struct {
	Rel     string `json:"rel"`
	Target  string `json:"target"`
	Ref     string `json:"ref,omitempty"`
	Line    int    `json:"line"`
	Display string `json:"display,omitempty"`
	Block   string `json:"block,omitempty"`
}

type backlinkItem struct {
	Rel   string `json:"rel"`
	Ref   string `json:"ref"`
	Title string `json:"title"`
	Line  int    `json:"line"`
}

type showDoc struct {
	Ref         string             `json:"ref"`
	ID          string             `json:"id"`
	Kind        string             `json:"kind"`
	Title       string             `json:"title"`
	Parent      string             `json:"parent"`
	Status      string             `json:"status"`
	Claim       *claimItem         `json:"claim"`
	Path        string             `json:"path"`
	Fields      map[string]any     `json:"fields"`
	Body        string             `json:"body"`
	Ancestors   []string           `json:"ancestors"`
	Children    []string           `json:"children"`
	Links       []linkItem         `json:"links"`
	Backlinks   []backlinkItem     `json:"backlinks"`
	Notes       []noteItem         `json:"notes"`
	History     []historyItem      `json:"history"`
	Diagnostics []parse.Diagnostic `json:"diagnostics"`
}

func (a *app) showCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <ref>",
		Short: "Show a node with its state",
		Args:  exactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			r, err := a.repo()
			if err != nil {
				return err
			}
			snap, err := r.Load()
			if err != nil {
				return fmt.Errorf("load repository: %w", err)
			}
			nf, err := snap.Resolve(args[0])
			if err != nil {
				return fmt.Errorf("show: %w", err)
			}
			doc := buildShow(snap, nf, a.now())
			if a.json {
				return a.writeJSON(doc)
			}
			a.printShow(doc)
			return nil
		},
	}
}

func buildShow(snap *store.Snapshot, nf *store.NodeFile, now time.Time) showDoc {
	st := event.Fold(snap.EventsOf(nf.Ref), event.Options{})
	ancestors, _ := snap.Tree.Ancestors(nf.Ref)
	children := snap.Tree.Children[nf.Ref]
	if children == nil {
		children = []string{}
	}
	doc := showDoc{
		Ref: nf.Ref, ID: nf.Node.ID, Kind: nf.Node.Kind, Title: nf.Node.Title, Parent: nf.ParentRef,
		Status: st.Status, Claim: claimOf(st, now), Path: nf.Path, Fields: nf.Node.Fields, Body: nf.Node.Body,
		Ancestors: ancestors, Children: children, Links: []linkItem{}, Backlinks: []backlinkItem{}, Notes: []noteItem{}, History: []historyItem{},
		Diagnostics: append(append([]parse.Diagnostic{}, nf.Diags...), st.Diagnostics...),
	}
	if doc.Parent == "" {
		doc.Parent = nf.Node.Parent
	}
	graph := snap.BuildGraph()
	for _, l := range graph.Out[nf.Ref] {
		item := linkItem{Rel: l.Rel, Target: l.Target, Line: l.Line, Display: l.Display, Block: l.Block}
		if t, err := snap.Resolve(l.Target); err == nil {
			item.Ref = t.Ref
		}
		doc.Links = append(doc.Links, item)
	}
	for _, b := range graph.In[nf.Ref] {
		item := backlinkItem{Rel: b.Rel, Ref: b.Ref, Line: b.Line}
		if from, ok := snap.Get(b.Ref); ok {
			item.Title = from.Node.Title
		}
		doc.Backlinks = append(doc.Backlinks, item)
	}
	for _, h := range st.History {
		if h.Event.Type == "note" && h.Event.Valid {
			doc.Notes = append(doc.Notes, noteItem{ID: h.Event.ID, Actor: h.Actor, At: h.Event.At.UTC().Format("2006-01-02T15:04:05Z"), Body: h.Event.Body})
		}
		doc.History = append(doc.History, historyItem{
			ID: h.Event.ID, Type: h.Event.Type, Actor: h.Actor, At: h.Event.At.UTC().Format("2006-01-02T15:04:05Z"), Applied: h.Applied,
		})
	}
	return doc
}

func (a *app) printShow(d showDoc) {
	a.printf(a.out, "%s  %s  %s\n%s\n", d.Ref, d.Kind, d.Status, d.Title)
	a.printf(a.out, "path: %s\n", d.Path)
	if d.Claim != nil {
		state := "expires"
		if !d.Claim.Live {
			state = "expired"
		}
		a.printf(a.out, "claimed by: %s (%s %s)\n", d.Claim.Holder, state, d.Claim.ExpiresAt)
	}
	if d.Parent != "" {
		a.printf(a.out, "parent: %s\n", d.Parent)
	}
	if len(d.Ancestors) > 0 {
		a.printf(a.out, "under: %s\n", strings.Join(reversed(d.Ancestors), " > "))
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
			a.printf(a.out, "  %s  %s  %s\n", h.At, h.Actor, h.Type)
		}
	}
}

func reversed(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[len(in)-1-i] = s
	}
	return out
}
