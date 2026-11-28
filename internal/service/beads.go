package service

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/internal/node"
	"github.com/tektite-tracker/tektite/internal/store"
	"github.com/tektite-tracker/tektite/parse"
)

const (
	beadsIDField   = "beads_id"
	beadsActor     = "tool:migrate-beads"
	actionCreate   = "create"
	actionSkip     = "skip"
	beadsPlanKind  = "epic"
	depParentChild = "parent-child"
	depBlocks      = "blocks"
	depDiscovered  = "discovered-from"
)

var (
	beadsStatuses = map[string]string{
		"open":        event.StatusOpen,
		"in_progress": event.StatusActive,
		"blocked":     event.StatusBlocked,
		"closed":      event.StatusDone,
		"deferred":    event.StatusBlocked,
	}
	beadsPriorities = []string{"critical", "high", "medium", "low", "low"}
	beadsMapped     = map[string]bool{
		"id": true, "title": true, "description": true, "design": true, "acceptance_criteria": true,
		"notes": true, "status": true, "priority": true, "labels": true, "dependencies": true,
		"comments": true, "assignee": true, "close_reason": true,
	}
	beadsTaken = map[string]bool{"id": true, "kind": true, "title": true, "parent": true, beadsIDField: true, "priority": true, "labels": true, "assignee": true}
)

type MigrateBeadsInput struct {
	Path   string
	DryRun bool
	Actor  string
}

type MigrateItem struct {
	BeadsID string `json:"beads_id"`
	Ref     string `json:"ref"`
	Title   string `json:"title"`
	Kind    string `json:"kind"`
	Status  string `json:"status"`
	Action  string `json:"action"`
}

type MigrateBeadsResult struct {
	Source   string             `json:"source"`
	DryRun   bool               `json:"dry_run"`
	Items    []MigrateItem      `json:"items"`
	Created  int                `json:"created"`
	Skipped  int                `json:"skipped"`
	Links    int                `json:"links"`
	Warnings []parse.Diagnostic `json:"warnings"`
}

type beadsIssue struct {
	id      string
	title   string
	raw     map[string]any
	line    int
	parent  string
	deps    []beadsDep
	created bool
	ref     string
}

type beadsDep struct {
	target string
	typ    string
}

func (s *Service) MigrateBeads(in MigrateBeadsInput) (*MigrateBeadsResult, error) {
	path := in.Path
	if path == "" {
		path = filepath.Join(filepath.Dir(s.Repo.Dir()), ".beads", "issues.jsonl")
	}
	res := &MigrateBeadsResult{Source: filepath.ToSlash(path), DryRun: in.DryRun, Items: []MigrateItem{}, Warnings: []parse.Diagnostic{}}
	issues, order, err := s.readBeads(path, res)
	if err != nil {
		return nil, err
	}
	snap, err := s.load()
	if err != nil {
		return nil, err
	}
	for _, nf := range snap.Nodes {
		if id, ok := nf.Node.Fields[beadsIDField]; ok {
			if is := issues[fmt.Sprint(id)]; is != nil && is.ref == "" {
				is.ref = nf.Ref
			}
		}
	}
	actor := in.Actor
	if actor == "" {
		actor = beadsActor
	}
	m := &migration{s: s, in: in, res: res, issues: issues, actor: actor, snap: snap, visiting: map[string]bool{}, done: map[string]bool{}}
	for _, id := range order {
		if err := m.ensure(id); err != nil {
			return nil, err
		}
	}
	for _, id := range order {
		if is := issues[id]; is.created {
			if err := m.link(is); err != nil {
				return nil, err
			}
		}
	}
	return res, nil
}

func (s *Service) readBeads(path string, res *MigrateBeadsResult) (map[string]*beadsIssue, []string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, newError(KindNotFound, "pass --from to point at an issues.jsonl", "no beads export at %s", filepath.ToSlash(path))
		}
		return nil, nil, fmt.Errorf("migrate-beads: %w", err)
	}
	defer func() { _ = f.Close() }()
	issues := map[string]*beadsIssue{}
	order := []string{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for line := 1; sc.Scan(); line++ {
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			continue
		}
		raw := map[string]any{}
		if err := json.Unmarshal([]byte(text), &raw); err != nil {
			res.Warnings = append(res.Warnings, importDiag(parse.CodeImportSkipped, line, "", "line is not a JSON object, skipped: %v", err))
			continue
		}
		id, title := strings.TrimSpace(str(raw["id"])), strings.TrimSpace(str(raw["title"]))
		switch {
		case id == "" || title == "":
			res.Warnings = append(res.Warnings, importDiag(parse.CodeImportSkipped, line, "id", "issue has no id or title, skipped"))
			continue
		case issues[id] != nil:
			res.Warnings = append(res.Warnings, importDiag(parse.CodeImportSkipped, line, "id", "issue %q appears more than once, the first one is used", id))
			continue
		}
		is := &beadsIssue{id: id, title: title, raw: raw, line: line}
		is.deps, is.parent = beadsDeps(id, raw["dependencies"])
		issues[id] = is
		order = append(order, id)
	}
	if err := sc.Err(); err != nil {
		return nil, nil, fmt.Errorf("migrate-beads: read %s: %w", filepath.ToSlash(path), err)
	}
	return issues, order, nil
}

func beadsDeps(id string, v any) ([]beadsDep, string) {
	list, _ := v.([]any)
	deps, parent := []beadsDep{}, ""
	for _, e := range list {
		d, ok := e.(map[string]any)
		if !ok {
			continue
		}
		if owner := str(d["issue_id"]); owner != "" && owner != id {
			continue
		}
		target, typ := strings.TrimSpace(str(d["depends_on_id"])), strings.TrimSpace(str(d["type"]))
		if typ == "" {
			typ = depBlocks
		}
		if typ == depParentChild && parent == "" {
			parent = target
			continue
		}
		deps = append(deps, beadsDep{target: target, typ: typ})
	}
	return deps, parent
}

type migration struct {
	s        *Service
	in       MigrateBeadsInput
	res      *MigrateBeadsResult
	issues   map[string]*beadsIssue
	actor    string
	snap     *store.Snapshot
	visiting map[string]bool
	done     map[string]bool
}

func (m *migration) warn(is *beadsIssue, code, field, format string, args ...any) {
	m.res.Warnings = append(m.res.Warnings, importDiag(code, is.line, field, "%s: %s", is.id, fmt.Sprintf(format, args...)))
}

func (m *migration) ensure(id string) error {
	is := m.issues[id]
	if m.done[id] {
		return nil
	}
	m.done[id] = true
	existing := is.ref != ""
	parentRef := ""
	if is.parent != "" {
		parent := m.issues[is.parent]
		switch {
		case parent == nil:
			m.warn(is, parse.CodeImportSkipped, "dependencies", "parent %q is not in the export, imported without a parent", is.parent)
		case m.visiting[is.parent]:
			m.warn(is, parse.CodeImportSkipped, "dependencies", "parent %q forms a cycle, imported without a parent", is.parent)
		default:
			m.visiting[id] = true
			err := m.ensure(is.parent)
			delete(m.visiting, id)
			if err != nil {
				return err
			}
			parentRef = parent.ref
		}
	}
	kind, status := m.kindAndStatus(is)
	item := MigrateItem{BeadsID: id, Ref: is.ref, Title: is.title, Kind: kind, Status: status, Action: actionSkip}
	if existing {
		m.res.Skipped++
		m.res.Items = append(m.res.Items, item)
		return nil
	}
	item.Action = actionCreate
	is.created = true
	m.res.Created++
	if m.in.DryRun {
		is.ref = "(new)"
		m.res.Items = append(m.res.Items, item)
		return nil
	}
	extra, body := m.fields(is)
	created, err := m.s.Repo.CreateNode(m.snap, m.s.Now(), m.s.Rand, store.NewNode{Kind: kind, Title: is.title, Parent: parentRef, Extra: extra, Body: body})
	if err != nil {
		return fmt.Errorf("migrate-beads: %s: %w", id, err)
	}
	is.ref, item.Ref = created.Ref, created.Ref
	if m.snap, err = m.s.load(); err != nil {
		return err
	}
	m.res.Items = append(m.res.Items, item)
	return m.events(is, status)
}

func (m *migration) kindAndStatus(is *beadsIssue) (string, string) {
	kind := parse.KindTicket
	if str(is.raw["issue_type"]) == beadsPlanKind {
		kind = parse.KindPlan
	}
	raw := str(is.raw["status"])
	status, ok := beadsStatuses[raw]
	switch {
	case raw == "":
		status = event.StatusOpen
	case !ok:
		status = event.StatusOpen
		m.warn(is, parse.CodeImportUnmapped, "status", "unknown beads status %q, imported as open", raw)
	case raw == "deferred":
		m.warn(is, parse.CodeImportUnmapped, "status", "beads status deferred has no tektite equivalent, imported as blocked")
	}
	return kind, status
}

func (m *migration) fields(is *beadsIssue) ([]node.Field, string) {
	fields := []node.Field{{Key: beadsIDField, Value: is.id}}
	switch p := is.raw["priority"].(type) {
	case nil:
	case float64:
		if i := int(p); float64(i) == p && i >= 0 && i < len(beadsPriorities) {
			fields = append(fields, node.Field{Key: "priority", Value: beadsPriorities[i]})
		} else {
			m.warn(is, parse.CodeImportUnmapped, "priority", "priority %v is outside 0 to 4, not imported", p)
		}
	default:
		m.warn(is, parse.CodeImportUnmapped, "priority", "priority %v is not a number, not imported", p)
	}
	if labels, ok := is.raw["labels"].([]any); ok && len(labels) > 0 {
		fields = append(fields, node.Field{Key: "labels", Value: labels})
	}
	if a := strings.TrimSpace(str(is.raw["assignee"])); a != "" {
		fields = append(fields, node.Field{Key: "assignee", Value: a})
	}
	keys := []string{}
	for k := range is.raw {
		if !beadsMapped[k] {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	for _, k := range keys {
		v := is.raw[k]
		if v == nil {
			continue
		}
		if !fieldKey.MatchString(k) {
			m.warn(is, parse.CodeImportUnmapped, k, "field name %q is not a valid frontmatter key, not imported", k)
			continue
		}
		name := k
		if beadsTaken[k] {
			name = "beads_" + k
		}
		fields = append(fields, node.Field{Key: name, Value: v})
	}
	return fields, beadsBody(is.raw)
}

func beadsBody(raw map[string]any) string {
	parts := []string{}
	for _, sec := range []struct{ key, heading string }{{"description", ""}, {"design", "Design"}, {"acceptance_criteria", "Acceptance criteria"}, {"notes", "Notes"}} {
		text := strings.TrimSpace(str(raw[sec.key]))
		switch {
		case text == "":
		case sec.heading == "":
			parts = append(parts, text)
		default:
			parts = append(parts, "## "+sec.heading+"\n\n"+text)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "\n\n") + "\n"
}

func (m *migration) events(is *beadsIssue, status string) error {
	if status != event.StatusOpen {
		if _, err := m.s.appendEvent(eventInput{typ: parse.EventSetStatus, query: is.ref, actor: m.actor, prepare: func(context) (step, error) {
			return step{fields: []node.Field{{Key: "status", Value: status}}}, nil
		}}); err != nil {
			return fmt.Errorf("migrate-beads: %s: %w", is.id, err)
		}
	}
	notes := []string{}
	if reason := strings.TrimSpace(str(is.raw["close_reason"])); reason != "" {
		notes = append(notes, "Closed: "+reason)
	}
	comments, _ := is.raw["comments"].([]any)
	for _, c := range comments {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		text := strings.TrimSpace(str(cm["text"]))
		if text == "" {
			continue
		}
		if author := strings.TrimSpace(str(cm["author"])); author != "" {
			text = author + ": " + text
		}
		notes = append(notes, text)
	}
	for _, text := range notes {
		if _, err := m.s.Note(NoteInput{Ref: is.ref, Text: text, Actor: m.actor}); err != nil {
			return fmt.Errorf("migrate-beads: %s: %w", is.id, err)
		}
	}
	return nil
}

func (m *migration) link(is *beadsIssue) error {
	for _, d := range is.deps {
		other := m.issues[d.target]
		switch {
		case d.typ != depBlocks && d.typ != depDiscovered:
			m.warn(is, parse.CodeImportUnmapped, "dependencies", "dependency of type %q on %q has no tektite link, not imported", d.typ, d.target)
			continue
		case other == nil:
			m.warn(is, parse.CodeImportSkipped, "dependencies", "dependency on %q is not in the export, not imported", d.target)
			continue
		}
		m.res.Links++
		if m.in.DryRun {
			continue
		}
		in := LinkInput{Verb: parse.EventLink, Ref: other.ref, Rel: "blocks", Target: is.ref, Actor: m.actor}
		if d.typ == depDiscovered {
			in = LinkInput{Verb: parse.EventLink, Ref: is.ref, Rel: "derived-from", Target: other.ref, Actor: m.actor}
		}
		if _, err := m.s.Link(in); err != nil {
			return fmt.Errorf("migrate-beads: %s: %w", is.id, err)
		}
	}
	return nil
}

func importDiag(code string, line int, field, format string, args ...any) parse.Diagnostic {
	return parse.Diagnostic{Code: code, Severity: parse.SeverityWarning, Message: fmt.Sprintf(format, args...), Line: line, Field: field}
}

func str(v any) string {
	s, _ := v.(string)
	return s
}
