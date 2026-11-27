package check

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
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
	CodeBlockCycle       = "TKT-W112"
	CodeEvidenceNoCommit = "TKT-W113"
	CodeMissingBlob      = "TKT-W114"
	CodeEvidenceStale    = "TKT-W115"
	CodeBlobTampered     = "TKT-E103"
	CodeUnreferencedBlob = "TKT-N103"
	CodeStaleSlug        = "TKT-N101"
	CodeDroppedBlocker   = "TKT-N102"
	CodeOlderRevision    = "TKT-N104"
)

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
	CodeEvidenceNoCommit:           "re-attach the evidence from a git checkout so it records a commit",
	CodeMissingBlob:                "restore the blob file or attach the evidence again",
	CodeEvidenceStale:              "run `tkt rerun` on the evidence or attach new evidence",
	CodeBlobTampered:               "restore the original blob, blobs must never change",
	CodeUnreferencedBlob:           "remove it with `tkt gc --blobs` if it is no longer needed",
	CodePolicyMalformed:            "fix the frontmatter of .tektite/policy.md, see the policy spec for the keys",
	CodePolicyViolation:            "meet the gate, or have a human record an override with a reason",
	CodeBlockCycle:                 "remove a blocks link from one node in the cycle",
	CodeOlderRevision:              "re-check the work against the current plan revision and record it with --revision",
	CodeDroppedBlocker:             "remove the blocks link or reopen the blocker",
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

type CommitLog func(since string, paths []string) ([]string, error)

type Options struct {
	Stale bool
	Log   CommitLog
}

type run struct {
	snap    *store.Snapshot
	schemas *store.Schemas
	out     []Problem
	graph   *store.Graph
	repo    *store.Repo
	opts    Options
}

func Run(r *store.Repo) (Report, error) {
	return RunWith(r, Options{})
}

func RunWith(r *store.Repo, opts Options) (Report, error) {
	snap, err := r.Load()
	if err != nil {
		return Report{}, fmt.Errorf("load repository: %w", err)
	}
	schemas, err := r.LoadSchemas()
	if err != nil {
		return Report{}, fmt.Errorf("load schemas: %w", err)
	}
	return analyze(snap, schemas, r, opts), nil
}

func Analyze(snap *store.Snapshot, schemas *store.Schemas) Report {
	return analyze(snap, schemas, nil, Options{})
}

func analyze(snap *store.Snapshot, schemas *store.Schemas, repo *store.Repo, opts Options) Report {
	c := &run{snap: snap, schemas: schemas, out: []Problem{}, graph: snap.BuildGraph(), repo: repo, opts: opts}
	c.schemaProblems()
	c.duplicateIDs()
	c.cycles()
	c.blockCycles()
	for _, n := range snap.Nodes {
		c.node(n)
	}
	c.orphans()
	c.blobs()
	c.staleEvidence()
	c.policy()
	return c.report()
}

func (c *run) add(sev parse.Severity, code, path string, line int, msg string) {
	c.out = append(c.out, Problem{Severity: sev, Code: code, Path: path, Line: line, Message: msg, Hint: hints[code]})
}

func (c *run) addDiag(path string, d parse.Diagnostic) {
	c.add(d.Severity, d.Code, path, d.Line, d.Message)
}

func (c *run) report() Report {
	slices.SortStableFunc(c.out, func(a, b Problem) int {
		return cmp.Or(cmp.Compare(a.Path, b.Path), cmp.Compare(a.Line, b.Line), cmp.Compare(a.Code, b.Code), cmp.Compare(a.Message, b.Message))
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
	slices.Sort(ids)
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

func (c *run) blockCycles() {
	refs := c.snap.Refs()
	slices.Sort(refs)
	const (
		visiting = 1
		finished = 2
	)
	color := map[string]int{}
	seen := map[string]bool{}
	var stack []string
	var walk func(string)
	walk = func(ref string) {
		color[ref] = visiting
		stack = append(stack, ref)
		for _, b := range c.graph.Blockers[ref] {
			switch color[b] {
			case 0:
				walk(b)
			case visiting:
				i := len(stack) - 1
				for stack[i] != b {
					i--
				}
				c.blockCycle(append([]string{}, stack[i:]...), seen)
			}
		}
		stack = stack[:len(stack)-1]
		color[ref] = finished
	}
	for _, ref := range refs {
		if color[ref] == 0 {
			walk(ref)
		}
	}
}

func (c *run) blockCycle(cycle []string, seen map[string]bool) {
	lo := 0
	for i, r := range cycle {
		if r < cycle[lo] {
			lo = i
		}
	}
	cycle = append(append([]string{}, cycle[lo:]...), cycle[:lo]...)
	key := strings.Join(cycle, ",")
	if seen[key] {
		return
	}
	seen[key] = true
	nf, _ := c.snap.Get(cycle[0])
	chain := append([]string{}, cycle...)
	chain = append(chain, cycle[0])
	c.add(parse.SeverityWarning, CodeBlockCycle, nf.Path, 0, "blocking cycle: "+strings.Join(chain, " is blocked by "))
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
	for _, l := range c.graph.Out[nf.Ref] {
		c.link(nf, l)
	}
	c.droppedBlockers(nf)
	c.olderRevision(nf)
	c.evidenceCommit(nf)
	st := c.events(nf)
	c.schema(nf, st)
}

func (c *run) olderRevision(nf *store.NodeFile) {
	built := c.graph.States[nf.Ref].BuiltAgainst
	if built == nil {
		return
	}
	plans := []string{}
	if nf.ParentRef != "" {
		plans = append(plans, nf.ParentRef)
	}
	for _, l := range c.graph.Out[nf.Ref] {
		if t, err := c.snap.Resolve(l.Target); err == nil && l.Rel == "implements" && !slices.Contains(plans, t.Ref) {
			plans = append(plans, t.Ref)
		}
	}
	for _, p := range plans {
		if cur := c.graph.States[p].Revision; cur > *built {
			c.add(parse.SeverityNote, CodeOlderRevision, nf.Path, 0, fmt.Sprintf("%s was built against revision %d of %s, which is now at revision %d", nf.Ref, *built, p, cur))
		}
	}
}

func (c *run) parent(nf *store.NodeFile) {
	switch {
	case nf.ParentErr != nil:
		c.add(parse.SeverityWarning, CodeBrokenParent, nf.Path, 0, fmt.Sprintf("parent %q of %s cannot be resolved: %s", nf.Node.Parent, nf.Ref, reason(nf.ParentErr)))
	case nf.ParentRef != "":
		c.stale(nf.Path, 0, "parent", nf.Node.Parent, nf.ParentRef)
	}
}

func (c *run) droppedBlockers(nf *store.NodeFile) {
	for _, b := range c.graph.Blockers[nf.Ref] {
		if c.graph.States[b].Status == event.StatusDropped {
			c.add(parse.SeverityNote, CodeDroppedBlocker, nf.Path, 0, fmt.Sprintf("%s is blocked by %s, which is dropped, so it no longer holds %s", nf.Ref, b, nf.Ref))
		}
	}
}

func (c *run) link(nf *store.NodeFile, l parse.Link) {
	target, err := c.snap.Resolve(l.Target)
	switch {
	case err == nil:
	case errors.Is(err, node.ErrNotFound):
		c.add(parse.SeverityWarning, CodeBrokenLink, nf.Path, l.Line, fmt.Sprintf("%s has a %s link to %q that matches no node", nf.Ref, l.Rel, l.Target))
		return
	case errors.Is(err, node.ErrTooShort):
		c.add(parse.SeverityWarning, CodeShortRef, nf.Path, l.Line, fmt.Sprintf("%s has a %s link to %q with a hash shorter than %d characters", nf.Ref, l.Rel, l.Target, node.MinHashLen))
		return
	default:
		c.add(parse.SeverityWarning, CodeAmbiguousRef, nf.Path, l.Line, fmt.Sprintf("%s has a %s link to %q: %s", nf.Ref, l.Rel, l.Target, reason(err)))
		return
	}
	if target.Ref == nf.Ref {
		c.add(parse.SeverityWarning, CodeSelfLink, nf.Path, l.Line, fmt.Sprintf("%s has a %s link to itself", nf.Ref, l.Rel))
		return
	}
	c.stale(nf.Path, l.Line, l.Rel+" link", l.Target, target.Ref)
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
	st := c.snap.State(nf.Ref)
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
	if len(sc.Statuses) > 0 && !slices.Contains(sc.Statuses, st.Status) {
		by := ""
		for _, h := range st.History {
			if h.Applied && h.Event.Type == parse.EventSetStatus {
				by = fmt.Sprintf(" set by event %s", h.Event.ID)
			}
		}
		c.add(parse.SeverityWarning, CodeDisallowedStatus, nf.Path, 0, fmt.Sprintf("%s (%s) has status %q%s, allowed: %s", nf.Ref, sc.Kind, st.Status, by, strings.Join(sc.Statuses, ", ")))
	}
	if nf.ParentRef == "" || len(sc.ParentKinds) == 0 {
		return
	}
	if p, ok := c.snap.Get(nf.ParentRef); ok && !slices.Contains(sc.ParentKinds, p.Node.Kind) {
		c.add(parse.SeverityWarning, CodeDisallowedParent, nf.Path, 0, fmt.Sprintf("%s (%s) has parent %s of kind %q, allowed: %s", nf.Ref, sc.Kind, p.Ref, p.Node.Kind, strings.Join(sc.ParentKinds, ", ")))
	}
}

func hasField(n *parse.Node, name string) bool {
	switch name {
	case "id":
		return n.ID != ""
	case "title":
		return n.Frontmatter && slices.Contains(n.FieldOrder, "title") && n.Title != ""
	case "kind":
		return n.Frontmatter && slices.Contains(n.FieldOrder, "kind") && n.Kind != ""
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

func reason(err error) string {
	var amb *node.AmbiguousError
	if errors.As(err, &amb) {
		return "matches " + strings.Join(amb.Matches, ", ")
	}
	return err.Error()
}
