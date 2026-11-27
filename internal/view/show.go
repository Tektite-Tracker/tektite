package view

import (
	"time"

	"github.com/tektite-tracker/tektite/internal/store"
	"github.com/tektite-tracker/tektite/parse"
)

type HistoryItem struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Actor    string `json:"actor"`
	At       string `json:"at"`
	Applied  bool   `json:"applied"`
	Override string `json:"override,omitempty"`
}

type NoteItem struct {
	ID    string `json:"id"`
	Actor string `json:"actor"`
	At    string `json:"at"`
	Body  string `json:"body"`
}

type LinkItem struct {
	Rel     string `json:"rel"`
	Target  string `json:"target"`
	Ref     string `json:"ref,omitempty"`
	Line    int    `json:"line"`
	Display string `json:"display,omitempty"`
	Block   string `json:"block,omitempty"`
}

type BacklinkItem struct {
	Rel   string `json:"rel"`
	Ref   string `json:"ref"`
	Title string `json:"title"`
	Line  int    `json:"line"`
}

type EvidenceItem struct {
	Ref         string   `json:"ref"`
	Title       string   `json:"title"`
	Claim       string   `json:"claim"`
	Actor       string   `json:"actor"`
	At          string   `json:"at"`
	Commit      string   `json:"commit"`
	Dirty       bool     `json:"dirty"`
	Command     string   `json:"command"`
	ExitCode    *int     `json:"exit_code"`
	Attachments []string `json:"attachments"`
}

type ShowDoc struct {
	Ref          string             `json:"ref"`
	ID           string             `json:"id"`
	Kind         string             `json:"kind"`
	Title        string             `json:"title"`
	Parent       string             `json:"parent"`
	Status       string             `json:"status"`
	Claim        *ClaimItem         `json:"claim"`
	Revision     int                `json:"revision"`
	BuiltAgainst *int               `json:"built_against"`
	Path         string             `json:"path"`
	Fields       map[string]any     `json:"fields"`
	Body         string             `json:"body"`
	Ancestors    []string           `json:"ancestors"`
	Children     []string           `json:"children"`
	Links        []LinkItem         `json:"links"`
	Backlinks    []BacklinkItem     `json:"backlinks"`
	Evidence     []EvidenceItem     `json:"evidence"`
	Notes        []NoteItem         `json:"notes"`
	History      []HistoryItem      `json:"history"`
	Diagnostics  []parse.Diagnostic `json:"diagnostics"`
}

func BuildShow(snap *store.Snapshot, nf *store.NodeFile, now time.Time) ShowDoc {
	st := snap.State(nf.Ref)
	ancestors, _ := snap.Tree.Ancestors(nf.Ref)
	children := snap.Tree.Children[nf.Ref]
	if children == nil {
		children = []string{}
	}
	doc := ShowDoc{
		Ref: nf.Ref, ID: nf.Node.ID, Kind: nf.Node.Kind, Title: nf.Node.Title, Parent: nf.ParentOrRaw(),
		Status: st.Status, Claim: ClaimOf(st, now), Path: nf.Path, Fields: nf.Node.Fields, Body: nf.Node.Body, Revision: st.Revision, BuiltAgainst: st.BuiltAgainst,
		Ancestors: ancestors, Children: children, Links: []LinkItem{}, Backlinks: []BacklinkItem{}, Evidence: []EvidenceItem{}, Notes: []NoteItem{}, History: []HistoryItem{},
		Diagnostics: append(append([]parse.Diagnostic{}, nf.Diags...), st.Diagnostics...),
	}
	if st.Revision > 0 {
		doc.Body = st.Body
	}
	graph := snap.BuildGraph()
	for _, l := range graph.Out[nf.Ref] {
		item := LinkItem{Rel: l.Rel, Target: l.Target, Line: l.Line, Display: l.Display, Block: l.Block}
		if t, err := snap.Resolve(l.Target); err == nil {
			item.Ref = t.Ref
		}
		doc.Links = append(doc.Links, item)
	}
	for _, b := range graph.In[nf.Ref] {
		item := BacklinkItem{Rel: b.Rel, Ref: b.Ref, Line: b.Line}
		if from, ok := snap.Get(b.Ref); ok {
			item.Title = from.Node.Title
		}
		doc.Backlinks = append(doc.Backlinks, item)
	}
	for _, ev := range snap.EvidenceFor(graph, nf.Ref) {
		item := EvidenceItem{
			Ref: ev.Node.Ref, Title: ev.Node.Node.Title, Claim: ev.Claim, Actor: ev.Actor, Commit: ev.Commit, Dirty: ev.Dirty,
			Command: ev.Command, ExitCode: ev.ExitCode, Attachments: append([]string{}, ev.Attachments...),
		}
		if !ev.At.IsZero() {
			item.At = parse.FormatTime(ev.At)
		}
		doc.Evidence = append(doc.Evidence, item)
	}
	for _, h := range st.History {
		if h.Event.Type == parse.EventNote && h.Event.Valid {
			doc.Notes = append(doc.Notes, NoteItem{ID: h.Event.ID, Actor: h.Actor, At: parse.FormatTime(h.Event.At), Body: h.Event.Body})
		}
		doc.History = append(doc.History, HistoryItem{
			ID: h.Event.ID, Type: h.Event.Type, Actor: h.Actor, At: parse.FormatTime(h.Event.At), Applied: h.Applied, Override: h.Override,
		})
	}
	return doc
}
