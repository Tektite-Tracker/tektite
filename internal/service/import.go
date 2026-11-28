package service

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/internal/node"
	"github.com/tektite-tracker/tektite/internal/store"
	"github.com/tektite-tracker/tektite/parse"
)

const (
	actionCreate = "create"
	actionSkip   = "skip"
	importActor  = "tool:import-"
)

type ImportInput struct {
	Source  string
	Path    string
	DryRun  bool
	Actor   string
	Yes     bool
	MapFile string
	Ask     func(ImportQuestion) (string, error)
}

type ImportItem struct {
	SourceID string `json:"source_id"`
	Ref      string `json:"ref"`
	Title    string `json:"title"`
	Kind     string `json:"kind"`
	Status   string `json:"status"`
	Action   string `json:"action"`
}

type ImportResult struct {
	Source      string             `json:"source"`
	Tracker     string             `json:"tracker"`
	DryRun      bool               `json:"dry_run"`
	Items       []ImportItem       `json:"items"`
	Created     int                `json:"created"`
	Skipped     int                `json:"skipped"`
	Links       int                `json:"links"`
	Mapping     *ImportMapping     `json:"mapping,omitempty"`
	MappingPath string             `json:"mapping_path,omitempty"`
	MappingNew  bool               `json:"mapping_saved,omitempty"`
	Warnings    []parse.Diagnostic `json:"warnings"`
}

func (s *Service) Import(in ImportInput) (*ImportResult, error) {
	src, ok := importSources[in.Source]
	if !ok {
		return nil, usageError("unknown import source %q, use one of %v", in.Source, ImportSources())
	}
	root := filepath.Dir(s.Repo.Dir())
	path := in.Path
	if path == "" {
		if path = src.defaultPath(root); path == "" {
			return nil, usageError("pass --from to point at the %s export", src.name())
		}
	}
	res := &ImportResult{Source: filepath.ToSlash(path), Tracker: src.name(), DryRun: in.DryRun, Items: []ImportItem{}, Warnings: []parse.Diagnostic{}}
	data, err := src.read(path, res)
	if err != nil {
		return nil, err
	}
	if !src.fixed() {
		mapping, err := s.resolveImportMapping(in, src, data, res)
		if err != nil {
			return nil, err
		}
		res.Mapping = mapping
		applyImportMapping(data, mapping)
	}
	issues := make(map[string]*importIssue, len(data.issues))
	for _, is := range data.issues {
		issues[is.id] = is
	}
	snap, err := s.load()
	if err != nil {
		return nil, err
	}
	for _, nf := range snap.Nodes {
		if id, ok := importedID(nf.Node.Fields, src.name()); ok {
			if is := issues[id]; is != nil && is.ref == "" {
				is.ref = nf.Ref
			}
		}
	}
	actor := in.Actor
	if actor == "" {
		actor = importActor + src.name()
	}
	m := &importer{s: s, in: in, tracker: src.name(), res: res, issues: issues, actor: actor, snap: snap, visiting: map[string]bool{}, done: map[string]bool{}, seen: map[string]bool{}}
	for _, is := range data.issues {
		if err := m.ensure(is.id); err != nil {
			return nil, err
		}
	}
	for _, is := range data.issues {
		if is.isNew {
			if err := m.link(is); err != nil {
				return nil, err
			}
		}
	}
	if err := s.saveImportMapping(in, res); err != nil {
		return nil, err
	}
	return res, nil
}

func importedID(fields map[string]any, tracker string) (string, bool) {
	if src, ok := fields[sourceField]; ok && fmt.Sprint(src) == tracker {
		if id, ok := fields[sourceIDField]; ok {
			return fmt.Sprint(id), true
		}
	}
	if tracker == "beads" {
		if id, ok := fields[legacyBeadsID]; ok {
			return fmt.Sprint(id), true
		}
	}
	return "", false
}

type importer struct {
	s        *Service
	in       ImportInput
	tracker  string
	res      *ImportResult
	issues   map[string]*importIssue
	actor    string
	snap     *store.Snapshot
	visiting map[string]bool
	done     map[string]bool
	seen     map[string]bool
}

func (m *importer) warn(is *importIssue, code, field, format string, args ...any) {
	m.res.Warnings = append(m.res.Warnings, importDiag(code, is.line, field, "%s: %s", is.id, fmt.Sprintf(format, args...)))
}

func (m *importer) emit(is *importIssue, ws []importWarn) {
	for _, w := range ws {
		m.warn(is, w.code, w.field, "%s", w.msg)
	}
}

func (m *importer) at(t time.Time) time.Time {
	if t.IsZero() {
		return m.s.Now()
	}
	return t
}

func (m *importer) ensure(id string) error {
	is := m.issues[id]
	if m.done[id] {
		return nil
	}
	m.done[id] = true
	existing := is.ref != ""
	parentRef, err := m.parentRef(is)
	if err != nil {
		return err
	}
	m.emit(is, is.statusWarns)
	item := ImportItem{SourceID: id, Ref: is.ref, Title: is.title, Kind: is.kind, Status: is.status, Action: actionSkip}
	if existing {
		m.res.Skipped++
		m.res.Items = append(m.res.Items, item)
		return nil
	}
	item.Action = actionCreate
	is.isNew = true
	m.res.Created++
	if m.in.DryRun {
		is.ref = "(new)"
		m.res.Items = append(m.res.Items, item)
		return nil
	}
	return m.create(is, item, parentRef)
}

func (m *importer) parentRef(is *importIssue) (string, error) {
	if is.parent == "" {
		return "", nil
	}
	parent := m.issues[is.parent]
	switch {
	case parent == nil:
		m.warn(is, parse.CodeImportSkipped, "dependencies", "parent %q is not in the export, imported without a parent", is.parent)
		return "", nil
	case m.visiting[is.parent]:
		m.warn(is, parse.CodeImportSkipped, "dependencies", "parent %q forms a cycle, imported without a parent", is.parent)
		return "", nil
	}
	m.visiting[is.id] = true
	err := m.ensure(is.parent)
	delete(m.visiting, is.id)
	if err != nil {
		return "", err
	}
	return parent.ref, nil
}

func (m *importer) create(is *importIssue, item ImportItem, parentRef string) error {
	fields := m.fields(is)
	m.emit(is, is.createWarns)
	created, err := m.s.Repo.CreateNode(m.snap, m.at(is.createdAt), m.s.Rand, store.NewNode{Kind: is.kind, Title: is.title, Parent: parentRef, Extra: fields, Body: is.body})
	if err != nil {
		return fmt.Errorf("import-%s: %s: %w", m.tracker, is.id, err)
	}
	is.ref, item.Ref = created.Ref, created.Ref
	m.snap.Reserve(created.Ref)
	m.res.Items = append(m.res.Items, item)
	return m.events(is)
}

func (m *importer) fields(is *importIssue) []node.Field {
	fields := []node.Field{{Key: sourceField, Value: m.tracker}, {Key: sourceIDField, Value: is.id}}
	if is.priority != "" {
		fields = append(fields, node.Field{Key: "priority", Value: is.priority})
	}
	if len(is.labels) > 0 {
		fields = append(fields, node.Field{Key: "labels", Value: is.labels})
	}
	if is.assignee != "" {
		fields = append(fields, node.Field{Key: "assignee", Value: is.assignee})
	}
	return append(fields, is.extra...)
}

type importNote struct {
	text string
	at   time.Time
}

func (m *importer) events(is *importIssue) error {
	if is.status != event.StatusOpen {
		if err := m.statusEvent(is); err != nil {
			return err
		}
	}
	m.emit(is, is.eventWarns)
	for _, n := range m.notes(is) {
		if _, err := m.s.Note(NoteInput{Ref: is.ref, Text: n.text, Actor: m.actor, At: n.at}); err != nil {
			return fmt.Errorf("import-%s: %s: %w", m.tracker, is.id, err)
		}
	}
	return nil
}

func (m *importer) statusEvent(is *importIssue) error {
	at := is.createdAt
	switch is.status {
	case event.StatusDone, event.StatusDropped:
		at = is.closedAt
	case event.StatusActive:
		if !is.startedAt.IsZero() {
			at = is.startedAt
		}
	}
	if _, err := m.s.appendEvent(eventInput{typ: parse.EventSetStatus, query: is.ref, actor: m.actor, at: m.at(at), prepare: func(context) (step, error) {
		return step{fields: []node.Field{{Key: "status", Value: is.status}}}, nil
	}}); err != nil {
		return fmt.Errorf("import-%s: %s: %w", m.tracker, is.id, err)
	}
	return nil
}

func (m *importer) notes(is *importIssue) []importNote {
	notes := []importNote{}
	if is.closeNote != "" {
		notes = append(notes, importNote{is.closeNote, m.at(is.closedAt)})
	}
	for _, c := range is.comments {
		text := c.text
		if c.author != "" {
			text = c.author + ": " + text
		}
		at := c.at
		if at.IsZero() {
			at = m.at(is.createdAt)
		}
		notes = append(notes, importNote{text, at})
	}
	return notes
}

func (m *importer) link(is *importIssue) error {
	for _, l := range is.links {
		other, ok := m.linkTarget(is, l)
		if !ok {
			continue
		}
		key := linkKey(is, other, l)
		if m.seen[key] {
			continue
		}
		m.seen[key] = true
		m.res.Links++
		if m.in.DryRun {
			continue
		}
		in := LinkInput{Verb: parse.EventLink, Ref: is.ref, Rel: l.rel, Target: other.ref, Actor: m.actor, At: m.at(is.createdAt)}
		if l.inbound {
			in.Ref, in.Target = other.ref, is.ref
		}
		if _, err := m.s.Link(in); err != nil {
			return fmt.Errorf("import-%s: %s: %w", m.tracker, is.id, err)
		}
	}
	return nil
}

func (m *importer) linkTarget(is *importIssue, l importLink) (*importIssue, bool) {
	other := m.issues[l.other]
	switch {
	case l.unmaps != "":
		m.warn(is, parse.CodeImportUnmapped, "dependencies", "%s", l.unmaps)
		return nil, false
	case other == nil:
		m.warn(is, parse.CodeImportSkipped, "dependencies", "dependency on %q is not in the export, not imported", l.other)
		return nil, false
	}
	blocker, blocked := is, other
	if l.inbound {
		blocker, blocked = other, is
	}
	if l.rel == relBlocks && blocker.status == event.StatusDropped && blocked.status != event.StatusDropped {
		m.warn(is, parse.CodeImportSkipped, "dependencies", "dependency on deleted issue %q would block a live ticket, not imported", l.other)
		return nil, false
	}
	return other, true
}

func linkKey(is, other *importIssue, l importLink) string {
	from, to := is.id, other.id
	if l.inbound {
		from, to = other.id, is.id
	}
	if l.rel == relRelates && from > to {
		from, to = to, from
	}
	return l.rel + "\x00" + from + "\x00" + to
}

func importDiag(code string, line int, field, format string, args ...any) parse.Diagnostic {
	return parse.Diagnostic{Code: code, Severity: parse.SeverityWarning, Message: fmt.Sprintf(format, args...), Line: line, Field: field}
}

func str(v any) string {
	s, _ := v.(string)
	return s
}
