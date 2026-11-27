package check

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/internal/node"
	"github.com/tektite-tracker/tektite/internal/store"
	"github.com/tektite-tracker/tektite/parse"
)

const (
	CodeDuplicateID      = "TKT-E101"
	CodeBrokenLink       = "TKT-W101"
	CodeSelfLink         = "TKT-W102"
	CodeBrokenParent     = "TKT-W103"
	CodeParentCycle      = "TKT-W104"
	CodeOrphanEvents     = "TKT-W105"
	CodeAmbiguousRef     = "TKT-W106"
	CodeShortRef         = "TKT-W107"
	CodeMissingRequired  = "TKT-W108"
	CodeDisallowedStatus = "TKT-W109"
	CodeDisallowedParent = "TKT-W111"
	CodeStaleSlug        = "TKT-N101"
)

var typedLinks = []string{"implements", "blocks", "evidences", "supersedes", "derived-from"}

var hints = map[string]string{
	parse.CodeStatusInFrontmatter:  "remove status from the frontmatter and set it with a set-status event",
	parse.CodeMissingID:            "run `tkt fmt` to assign an id",
	parse.CodeMissingKind:          "add a kind to the frontmatter",
	parse.CodeMissingTitle:         "add a title to the frontmatter",
	parse.CodeMalformedFrontmatter: "fix the YAML between the --- lines",
	parse.CodeUnterminatedFrontmtr: "close the frontmatter with a --- line",
	parse.CodeDuplicateKey:         "keep one occurrence of the key",
	parse.CodeMissingActor:         "add an actor such as human:name to the event",
	parse.CodeEventNameNotULID:     "name the event file after a ULID",
	parse.CodeUnknownStatus:        "use one of open, active, blocked, review, done, dropped",
	parse.CodeDuplicateEventID:     "rename one of the event files",
	CodeDuplicateID:                "give one of the nodes a new id",
	CodeBrokenLink:                 "fix the reference or remove the link",
	CodeSelfLink:                   "remove the link to the node itself",
	CodeBrokenParent:               "point parent at an existing node or clear it",
	CodeParentCycle:                "change the parent of one node in the cycle",
	CodeOrphanEvents:               "move the events under an existing node or delete them",
	CodeAmbiguousRef:               "use a longer hash",
	CodeShortRef:                   "use at least three hash characters",
	CodeMissingRequired:            "add the field to the frontmatter",
	CodeDisallowedStatus:           "set a status the schema allows or update the schema",
	CodeDisallowedParent:           "move the node under an allowed parent kind or update the schema",
	store.CodeSchemaMalformed:      "fix the schema frontmatter",
	store.CodeSchemaKindMismatch:   "rename the file or fix its kind",
}

type Problem struct {
	Severity parse.Severity `json:"severity"`
	Code     string         `json:"code"`
	Path     string         `json:"path"`
	Line     int            `json:"line"`
	Message  string         `json:"message"`
	Hint     string         `json:"hint,omitempty"`
}

type Report struct {
	Problems []Problem `json:"problems"`
	Errors   int       `json:"errors"`
	Warnings int       `json:"warnings"`
	Notes    int       `json:"notes"`
}

func (r Report) HasErrors() bool {
	return r.Errors > 0
}

type link struct {
	rel    string
	target string
	line   int
}

type run struct {
	snap    *store.Snapshot
	schemas *store.Schemas
	out     []Problem
}

func Run(r *store.Repo) (Report, error) {
	snap, err := r.Load()
	if err != nil {
		return Report{}, fmt.Errorf("load repository: %w", err)
	}
	schemas, err := r.LoadSchemas()
	if err != nil {
		return Report{}, fmt.Errorf("load schemas: %w", err)
	}
	return Analyze(snap, schemas), nil
}

func Analyze(snap *store.Snapshot, schemas *store.Schemas) Report {
	c := &run{snap: snap, schemas: schemas, out: []Problem{}}
	c.schemaProblems()
	c.duplicateIDs()
	c.cycles()
	for _, n := range snap.Nodes {
		c.node(n)
	}
	c.orphans()
	return c.report()
}

func (c *run) add(sev parse.Severity, code, path string, line int, msg string) {
	c.out = append(c.out, Problem{Severity: sev, Code: code, Path: path, Line: line, Message: msg, Hint: hints[code]})
}

func (c *run) addDiag(path string, d parse.Diagnostic) {
	c.add(d.Severity, d.Code, path, d.Line, d.Message)
}

func (c *run) report() Report {
	sort.SliceStable(c.out, func(i, j int) bool {
		a, b := c.out[i], c.out[j]
		switch {
		case a.Path != b.Path:
			return a.Path < b.Path
		case a.Line != b.Line:
			return a.Line < b.Line
		case a.Code != b.Code:
			return a.Code < b.Code
		}
		return a.Message < b.Message
	})
	rep := Report{Problems: c.out}
	for _, p := range c.out {
		switch p.Severity {
		case parse.SeverityError:
			rep.Errors++
		case parse.SeverityWarning:
			rep.Warnings++
		default:
			rep.Notes++
		}
	}
	return rep
}

func (c *run) schemaProblems() {
	for _, p := range c.schemas.Problems {
		c.addDiag(p.Path, p.Diag)
	}
}

func (c *run) duplicateIDs() {
	dups := c.snap.DuplicateIDs()
	ids := make([]string, 0, len(dups))
	for id := range dups {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		paths := make([]string, 0, len(dups[id]))
		for _, ref := range dups[id] {
			nf, _ := c.snap.Get(ref)
			paths = append(paths, nf.Path)
		}
		for i, p := range paths {
			others := append(append([]string{}, paths[:i]...), paths[i+1:]...)
			c.add(parse.SeverityError, CodeDuplicateID, p, 0, fmt.Sprintf("id %s is also declared by %s", id, strings.Join(others, ", ")))
		}
	}
}

func (c *run) cycles() {
	for _, cyc := range c.snap.Tree.Cycles {
		nf, _ := c.snap.Get(cyc[0])
		c.add(parse.SeverityWarning, CodeParentCycle, nf.Path, 0, "parent cycle: "+strings.Join(append(append([]string{}, cyc...), cyc[0]), " -> "))
	}
}

func (c *run) orphans() {
	for _, o := range c.snap.Orphans {
		c.add(parse.SeverityWarning, CodeOrphanEvents, o.Dir, 0, fmt.Sprintf("%d event file(s) belong to no node", len(o.Events)))
		for _, f := range o.Events {
			for _, d := range f.Diags {
				c.addDiag(f.Path, d)
			}
		}
	}
}

func (c *run) node(nf *store.NodeFile) {
	for _, d := range nf.Diags {
		c.addDiag(nf.Path, d)
	}
	c.parent(nf)
	for _, l := range c.links(nf) {
		c.link(nf, l)
	}
	st := c.events(nf)
	c.schema(nf, st)
}

func (c *run) parent(nf *store.NodeFile) {
	switch {
	case nf.ParentErr != nil:
		c.add(parse.SeverityWarning, CodeBrokenParent, nf.Path, 0, fmt.Sprintf("parent %q of %s cannot be resolved: %s", nf.Node.Parent, nf.Ref, reason(nf.ParentErr)))
	case nf.ParentRef != "":
		c.stale(nf.Path, 0, "parent", nf.Node.Parent, nf.ParentRef)
	}
}

func (c *run) links(nf *store.NodeFile) []link {
	out := []link{}
	for _, l := range nf.Node.Links {
		out = append(out, link{rel: "mentions", target: l.Target, line: l.Line})
	}
	for _, rel := range typedLinks {
		v, ok := nf.Node.Fields[rel]
		if !ok {
			continue
		}
		for _, t := range targets(v) {
			out = append(out, link{rel: rel, target: t})
		}
	}
	for _, e := range c.snap.EventsOf(nf.Ref) {
		if e.Valid && e.Type == "link" {
			out = append(out, link{rel: fmt.Sprint(e.Fields["rel"]), target: strings.TrimSpace(fmt.Sprint(e.Fields["target"]))})
		}
	}
	return out
}

func (c *run) link(nf *store.NodeFile, l link) {
	target, err := c.snap.Resolve(l.target)
	switch {
	case err == nil:
	case errors.Is(err, node.ErrNotFound):
		c.add(parse.SeverityWarning, CodeBrokenLink, nf.Path, l.line, fmt.Sprintf("%s has a %s link to %q that matches no node", nf.Ref, l.rel, l.target))
		return
	case errors.Is(err, node.ErrTooShort):
		c.add(parse.SeverityWarning, CodeShortRef, nf.Path, l.line, fmt.Sprintf("%s has a %s link to %q with a hash shorter than %d characters", nf.Ref, l.rel, l.target, node.MinHashLen))
		return
	default:
		c.add(parse.SeverityWarning, CodeAmbiguousRef, nf.Path, l.line, fmt.Sprintf("%s has a %s link to %q: %s", nf.Ref, l.rel, l.target, reason(err)))
		return
	}
	if target.Ref == nf.Ref {
		c.add(parse.SeverityWarning, CodeSelfLink, nf.Path, l.line, fmt.Sprintf("%s has a %s link to itself", nf.Ref, l.rel))
		return
	}
	c.stale(nf.Path, l.line, l.rel+" link", l.target, target.Ref)
}

func (c *run) stale(path string, line int, what, query, ref string) {
	if q := strings.TrimSpace(query); q != "" && q != node.HashOf(q) && !strings.EqualFold(q, ref) {
		c.add(parse.SeverityNote, CodeStaleSlug, path, line, fmt.Sprintf("%s %q resolves to %q because the slug part differs", what, q, ref))
	}
}

func (c *run) events(nf *store.NodeFile) event.State {
	files := c.snap.Events[nf.Ref]
	for _, f := range files {
		for _, d := range f.Diags {
			c.addDiag(f.Path, d)
		}
	}
	st := event.Fold(c.snap.EventsOf(nf.Ref), event.Options{})
	for _, d := range st.Diagnostics {
		if d.Code != parse.CodeEventSkipped {
			c.addDiag(nf.Path, d)
		}
	}
	return st
}

func (c *run) schema(nf *store.NodeFile, st event.State) {
	sc, ok := c.schemas.For(nf.Node.Kind)
	if !ok {
		return
	}
	for _, f := range sc.Required {
		if !hasField(nf.Node, f) {
			c.add(parse.SeverityWarning, CodeMissingRequired, nf.Path, 0, fmt.Sprintf("%s (%s) is missing required field %q", nf.Ref, sc.Kind, f))
		}
	}
	if len(sc.Statuses) > 0 && !contains(sc.Statuses, st.Status) {
		by := ""
		for _, h := range st.History {
			if h.Applied && h.Event.Type == "set-status" {
				by = fmt.Sprintf(" set by event %s", h.Event.ID)
			}
		}
		c.add(parse.SeverityWarning, CodeDisallowedStatus, nf.Path, 0, fmt.Sprintf("%s (%s) has status %q%s, allowed: %s", nf.Ref, sc.Kind, st.Status, by, strings.Join(sc.Statuses, ", ")))
	}
	if nf.ParentRef == "" || len(sc.ParentKinds) == 0 {
		return
	}
	if p, ok := c.snap.Get(nf.ParentRef); ok && !contains(sc.ParentKinds, p.Node.Kind) {
		c.add(parse.SeverityWarning, CodeDisallowedParent, nf.Path, 0, fmt.Sprintf("%s (%s) has parent %s of kind %q, allowed: %s", nf.Ref, sc.Kind, p.Ref, p.Node.Kind, strings.Join(sc.ParentKinds, ", ")))
	}
}

func hasField(n *parse.Node, name string) bool {
	switch name {
	case "id":
		return n.ID != ""
	case "title":
		return n.Frontmatter && contains(n.FieldOrder, "title") && n.Title != ""
	case "kind":
		return n.Frontmatter && contains(n.FieldOrder, "kind") && n.Kind != ""
	case "parent":
		return n.Parent != ""
	}
	v, ok := n.Fields[name]
	if !ok || v == nil {
		return false
	}
	if s, isString := v.(string); isString && strings.TrimSpace(s) == "" {
		return false
	}
	return true
}

func targets(v any) []string {
	out := []string{}
	switch x := v.(type) {
	case []any:
		for _, item := range x {
			if s := strings.TrimSpace(fmt.Sprint(item)); item != nil && s != "" {
				out = append(out, s)
			}
		}
	case string:
		if s := strings.TrimSpace(x); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func reason(err error) string {
	var amb *node.AmbiguousError
	if errors.As(err, &amb) {
		return "matches " + strings.Join(amb.Matches, ", ")
	}
	return err.Error()
}
