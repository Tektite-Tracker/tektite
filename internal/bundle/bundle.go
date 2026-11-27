package bundle

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/internal/store"
	"github.com/tektite-tracker/tektite/parse"
)

const (
	defaultBudget  = 8000
	estimator      = "bytes/4"
	modeFull       = "full"
	modeSummary    = "summary"
	modeTruncated  = "truncated"
	summaryBytes   = 240
	evidenceLimit  = 5
	historyLimit   = 10
	historyNoteMax = 160
)

type Entry struct {
	Ref      string `json:"ref"`
	Kind     string `json:"kind,omitempty"`
	Title    string `json:"title"`
	Mode     string `json:"mode"`
	Revision int    `json:"revision,omitempty"`
	Block    string `json:"block,omitempty"`
	Rel      string `json:"rel,omitempty"`
	Stale    bool   `json:"stale,omitempty"`
	Text     string `json:"text"`
	Tokens   int    `json:"tokens"`
}

type Omitted struct {
	Section string `json:"section"`
	Ref     string `json:"ref"`
	Title   string `json:"title"`
	Fetch   string `json:"fetch"`
}

type Bundle struct {
	Ref       string             `json:"ref"`
	Budget    int                `json:"budget"`
	Tokens    int                `json:"tokens"`
	Estimator string             `json:"estimator"`
	Sections  map[string][]Entry `json:"-"`
	Shortened []string           `json:"shortened"`
	Omitted   []Omitted          `json:"omitted"`
}

type sectionDef struct {
	Key   string
	Title string
}

var sectionDefs = []sectionDef{
	{"node", "Node"},
	{"ancestors", "Ancestor specs and plans"},
	{"gaps", "Open gaps"},
	{"evidence", "Recent evidence"},
	{"links", "Linked nodes"},
	{"history", "Recent history"},
}

type Options struct {
	Budget   int
	Head     string
	BlobSize func(attachment string) (int64, bool)
}

type candidate struct {
	section string
	entry   Entry
	summary *Entry
	group   string
}

func estimate(s string) int {
	return (len(s) + 3) / 4
}

func Build(snap *store.Snapshot, nf *store.NodeFile, opts Options) *Bundle {
	if opts.Budget < 1 {
		opts.Budget = defaultBudget
	}
	graph := snap.BuildGraph()
	b := &Bundle{Ref: nf.Ref, Budget: opts.Budget, Estimator: estimator, Sections: map[string][]Entry{}, Shortened: []string{}, Omitted: []Omitted{}}
	for _, s := range sectionDefs {
		b.Sections[s.Key] = []Entry{}
	}

	self := nodeEntry(snap, nf)
	self.Mode = modeFull
	used := self.Tokens
	if used > opts.Budget {
		self = truncate(self, opts.Budget)
		used = self.Tokens
		b.Shortened = append(b.Shortened, nf.Ref)
	}
	b.Sections["node"] = []Entry{self}

	seen := map[string]bool{nf.Ref: true}
	cands := collect(snap, graph, nf, opts, seen)
	dropped := false
	droppedGroups := map[string]int{}
	for _, c := range cands {
		if !dropped {
			switch {
			case used+c.entry.Tokens <= opts.Budget:
				used += c.entry.Tokens
				b.Sections[c.section] = append(b.Sections[c.section], c.entry)
				continue
			case c.summary != nil && used+c.summary.Tokens <= opts.Budget:
				used += c.summary.Tokens
				b.Sections[c.section] = append(b.Sections[c.section], *c.summary)
				b.Shortened = append(b.Shortened, c.entry.Ref)
				continue
			}
			dropped = true
		}
		if c.group != "" {
			droppedGroups[c.section+"\x00"+c.group]++
			continue
		}
		b.Omitted = append(b.Omitted, Omitted{Section: c.section, Ref: c.entry.Ref, Title: c.entry.Title, Fetch: "tkt show " + c.entry.Ref})
	}
	for _, s := range sectionDefs {
		if n := droppedGroups[s.Key+"\x00"+nf.Ref]; n > 0 {
			b.Omitted = append(b.Omitted, Omitted{Section: s.Key, Ref: nf.Ref, Title: fmt.Sprintf("%d %s of %s", n, strings.ToLower(s.Title), nf.Ref), Fetch: "tkt show " + nf.Ref})
		}
	}
	slices.SortStableFunc(b.Omitted, func(x, y Omitted) int { return cmp.Compare(sectionIndex(x.Section), sectionIndex(y.Section)) })
	b.Tokens = used
	return b
}

func sectionIndex(key string) int {
	for i, s := range sectionDefs {
		if s.Key == key {
			return i
		}
	}
	return len(sectionDefs)
}

func collect(snap *store.Snapshot, graph *store.Graph, nf *store.NodeFile, opts Options, seen map[string]bool) []candidate {
	var out []candidate
	ancestors, _ := snap.Tree.Ancestors(nf.Ref)
	for _, ref := range ancestors {
		if anc, ok := snap.Get(ref); ok && !seen[ref] {
			seen[ref] = true
			out = append(out, nodeCandidate("ancestors", snap, anc, ""))
		}
	}
	for _, blk := range nf.Node.Blocks {
		if !blk.Gap || blk.Checked {
			continue
		}
		text := blk.Text
		if blk.ID != "" {
			text = blk.ID + ": " + text
		}
		out = append(out, candidate{section: "gaps", group: nf.Ref, entry: sized(Entry{Ref: nf.Ref, Kind: nf.Node.Kind, Title: blk.Text, Mode: modeFull, Block: blk.ID, Text: text})})
	}
	out = append(out, evidenceCandidates(snap, graph, nf, opts, seen)...)
	for _, l := range graph.Out[nf.Ref] {
		if t, err := snap.Resolve(l.Target); err == nil && !seen[t.Ref] {
			seen[t.Ref] = true
			out = append(out, nodeCandidate("links", snap, t, l.Rel))
		}
	}
	for _, bl := range graph.In[nf.Ref] {
		if t, ok := snap.Get(bl.Ref); ok && !seen[t.Ref] {
			seen[t.Ref] = true
			out = append(out, nodeCandidate("links", snap, t, bl.Rel))
		}
	}
	hist := []event.Entry{}
	for _, h := range graph.States[nf.Ref].History {
		if h.Applied {
			hist = append(hist, h)
		}
	}
	if len(hist) > historyLimit {
		hist = hist[len(hist)-historyLimit:]
	}
	for _, h := range hist {
		line := fmt.Sprintf("%s %s %s", parse.FormatTime(h.Event.At), h.Actor, h.Event.Type)
		if h.Event.Type == parse.EventSetStatus {
			line += " " + h.Event.String("status")
		}
		if h.Event.Type == parse.EventNote && h.Event.Body != "" {
			line += ": " + clip(strings.Join(strings.Fields(h.Event.Body), " "), historyNoteMax)
		}
		out = append(out, candidate{section: "history", group: nf.Ref, entry: sized(Entry{Ref: nf.Ref, Kind: nf.Node.Kind, Title: h.Event.ID, Mode: modeFull, Text: line})})
	}
	return out
}

func nodeCandidate(section string, snap *store.Snapshot, nf *store.NodeFile, rel string) candidate {
	full := nodeEntry(snap, nf)
	full.Mode, full.Rel = modeFull, rel
	sum := full
	sum.Mode = modeSummary
	sum.Text = summaryOf(nf)
	sum = sized(sum)
	return candidate{section: section, entry: full, summary: &sum}
}

func evidenceCandidates(snap *store.Snapshot, graph *store.Graph, nf *store.NodeFile, opts Options, seen map[string]bool) []candidate {
	found := snap.EvidenceFor(graph, nf.Ref)
	if len(found) > evidenceLimit {
		found = found[:evidenceLimit]
	}
	var out []candidate
	for _, ev := range found {
		if seen[ev.Node.Ref] {
			continue
		}
		seen[ev.Node.Ref] = true
		head, stale := evidenceHead(ev, opts)
		full := nodeEntry(snap, ev.Node)
		full.Mode, full.Stale = modeFull, stale
		full.Text = head + bodyBlock(full.Text)
		sum := full
		sum.Mode, sum.Text = modeSummary, strings.TrimRight(head, "\n")
		out = append(out, candidate{section: "evidence", entry: sized(full), summary: summaryEntry(sum)})
	}
	return out
}

func summaryEntry(e Entry) *Entry {
	s := sized(e)
	return &s
}

func bodyBlock(body string) string {
	if body == "" {
		return ""
	}
	return "\n" + body
}

func evidenceHead(ev store.Evidence, opts Options) (string, bool) {
	var b strings.Builder
	if ev.Claim != "" {
		fmt.Fprintf(&b, "Claim: %s\n", ev.Claim)
	}
	if ev.Commit != "" {
		fmt.Fprintf(&b, "Commit: %s\n", ev.Commit)
	}
	if ev.Command != "" {
		fmt.Fprintf(&b, "Command: %s\n", ev.Command)
	}
	if ev.ExitCode != nil {
		fmt.Fprintf(&b, "Exit code: %d\n", *ev.ExitCode)
	}
	for _, att := range ev.Attachments {
		if opts.BlobSize == nil {
			fmt.Fprintf(&b, "Attachment: %s\n", att)
			continue
		}
		if size, ok := opts.BlobSize(att); ok {
			fmt.Fprintf(&b, "Attachment: %s (%d bytes, content not included)\n", att, size)
		} else {
			fmt.Fprintf(&b, "Attachment: %s (missing)\n", att)
		}
	}
	stale := ev.Commit != "" && opts.Head != "" && !strings.HasPrefix(opts.Head, ev.Commit) && !strings.HasPrefix(ev.Commit, opts.Head)
	if stale {
		fmt.Fprintf(&b, "Possibly stale: recorded at %s, HEAD is %s\n", ev.Commit, opts.Head)
	}
	return b.String(), stale
}

func nodeEntry(snap *store.Snapshot, nf *store.NodeFile) Entry {
	body, rev := currentBody(snap, nf)
	return sized(Entry{Ref: nf.Ref, Kind: nf.Node.Kind, Title: nf.Node.Title, Revision: rev, Text: body})
}

func sized(e Entry) Entry {
	e.Tokens = estimate(e.Text)
	return e
}

func currentBody(snap *store.Snapshot, nf *store.NodeFile) (string, int) {
	st := snap.State(nf.Ref)
	if st.Revision > 0 {
		return st.Body, st.Revision
	}
	return nf.Node.Body, 0
}

func summaryOf(nf *store.NodeFile) string {
	if s, ok := nf.Node.Fields["summary"].(string); ok && strings.TrimSpace(s) != "" {
		return clip(strings.Join(strings.Fields(s), " "), summaryBytes)
	}
	para, _, _ := strings.Cut(strings.TrimSpace(nf.Node.Body), "\n\n")
	return clip(strings.Join(strings.Fields(para), " "), summaryBytes)
}

func clip(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := s[:limit]
	if i := strings.LastIndexByte(cut, ' '); i > limit/2 {
		cut = cut[:i]
	}
	for len(cut) > 0 && cut[len(cut)-1]&0xC0 == 0x80 {
		cut = cut[:len(cut)-1]
	}
	return strings.TrimRight(cut, " ") + "…"
}

func truncate(e Entry, budget int) Entry {
	marker := fmt.Sprintf("\n\n[body truncated: about %d of %d tokens shown, run `tkt show %s` for the full body]", budget, e.Tokens, e.Ref)
	limit := budget*4 - len(marker)
	if limit < 0 {
		limit = 0
	}
	paras := strings.Split(e.Text, "\n\n")
	kept := ""
	for _, p := range paras {
		next := p
		if kept != "" {
			next = kept + "\n\n" + p
		}
		if len(next) > limit {
			break
		}
		kept = next
	}
	if kept == "" {
		kept = clip(paras[0], limit)
	}
	e.Text = kept + marker
	e.Mode = modeTruncated
	return sized(e)
}

func (b *Bundle) Markdown() string {
	var w strings.Builder
	for _, s := range sectionDefs {
		fmt.Fprintf(&w, "## %s\n\n", s.Title)
		entries := b.Sections[s.Key]
		if len(entries) == 0 {
			w.WriteString("(none)\n\n")
			continue
		}
		for _, e := range entries {
			switch s.Key {
			case "gaps", "history":
				fmt.Fprintf(&w, "- %s\n", e.Text)
			default:
				fmt.Fprintf(&w, "### %s\n\n%s\n\n", heading(e), e.Text)
			}
		}
		if s.Key == "gaps" || s.Key == "history" {
			w.WriteString("\n")
		}
	}
	if len(b.Shortened) > 0 {
		fmt.Fprintf(&w, "Shortened: %s\n\n", strings.Join(b.Shortened, ", "))
	}
	if len(b.Omitted) > 0 {
		w.WriteString("## Omitted\n\n")
		for _, o := range b.Omitted {
			fmt.Fprintf(&w, "- %s: %s, fetch with `%s`\n", o.Section, o.Title, o.Fetch)
		}
	}
	return strings.TrimRight(w.String(), "\n") + "\n"
}

func heading(e Entry) string {
	parts := []string{e.Ref, e.Kind}
	if e.Rel != "" {
		parts = append(parts, e.Rel)
	}
	if e.Revision > 0 {
		parts = append(parts, "revision "+strconv.Itoa(e.Revision))
	}
	if e.Mode != modeFull {
		parts = append(parts, e.Mode)
	}
	return e.Title + " (" + strings.Join(parts, ", ") + ")"
}

func (b *Bundle) MarshalJSON() ([]byte, error) {
	doc := map[string]any{
		"ref": b.Ref, "budget": b.Budget, "tokens": b.Tokens, "estimator": b.Estimator,
		"shortened": b.Shortened, "omitted": b.Omitted,
	}
	for _, s := range sectionDefs {
		doc[s.Key] = b.Sections[s.Key]
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("marshal bundle: %w", err)
	}
	return out, nil
}
