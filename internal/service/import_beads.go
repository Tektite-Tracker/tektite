package service

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/internal/node"
	"github.com/tektite-tracker/tektite/parse"
)

const (
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
		"tombstone":   event.StatusDropped,
	}
	beadsPriorities = []string{"critical", "high", "medium", "low", "low"}
	beadsMapped     = map[string]bool{
		"id": true, "title": true, "description": true, "design": true, "acceptance_criteria": true,
		"notes": true, "status": true, "priority": true, "labels": true, "dependencies": true,
		"comments": true, "assignee": true, "close_reason": true, "created_at": true, "closed_at": true,
		"_type": true, "comment_count": true, "dependency_count": true, "dependent_count": true, "updated_at": true,
	}
	beadsTaken = map[string]bool{"id": true, "kind": true, "title": true, "parent": true, sourceField: true, sourceIDField: true, "priority": true, "labels": true, "assignee": true}
)

type beadsSource struct{}

func init() { registerImportSource(beadsSource{}) }

func (beadsSource) name() string { return "beads" }
func (beadsSource) fixed() bool  { return true }

func (beadsSource) defaultPath(root string) string {
	return filepath.Join(root, ".beads", "issues.jsonl")
}

func (beadsSource) read(path string, res *ImportResult) (*importData, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, newError(KindNotFound, "pass --from to point at an issues.jsonl", "no beads export at %s", filepath.ToSlash(path))
		}
		return nil, fmt.Errorf("import-beads: %w", err)
	}
	defer func() { _ = f.Close() }()
	data := &importData{}
	seen := map[string]bool{}
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
		case seen[id]:
			res.Warnings = append(res.Warnings, importDiag(parse.CodeImportSkipped, line, "id", "issue %q appears more than once, the first one is used", id))
			continue
		}
		seen[id] = true
		data.issues = append(data.issues, beadsIssueOf(id, title, line, raw))
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("import-beads: read %s: %w", filepath.ToSlash(path), err)
	}
	return data, nil
}

func beadsIssueOf(id, title string, line int, raw map[string]any) *importIssue {
	is := &importIssue{id: id, title: title, line: line, body: beadsBody(raw)}
	is.kind = parse.KindTicket
	if str(raw["issue_type"]) == beadsPlanKind {
		is.kind = parse.KindPlan
	}
	rawStatus := str(raw["status"])
	status, ok := beadsStatuses[rawStatus]
	switch {
	case rawStatus == "":
		status = event.StatusOpen
	case !ok:
		status = event.StatusOpen
		is.statusWarns = append(is.statusWarns, importWarn{parse.CodeImportUnmapped, "status", fmt.Sprintf("unknown beads status %q, imported as open", rawStatus)})
	case rawStatus == "deferred":
		is.statusWarns = append(is.statusWarns, importWarn{parse.CodeImportUnmapped, "status", "beads status deferred has no tektite equivalent, imported as blocked"})
	}
	is.status = status
	is.createdAt = beadsTime(is, raw, "created_at")
	is.closedAt = beadsTime(is, raw, "closed_at")
	switch p := raw["priority"].(type) {
	case nil:
	case float64:
		if i := int(p); float64(i) == p && i >= 0 && i < len(beadsPriorities) {
			is.priority = beadsPriorities[i]
		} else {
			is.createWarns = append(is.createWarns, importWarn{parse.CodeImportUnmapped, "priority", fmt.Sprintf("priority %v is outside 0 to 4, not imported", p)})
		}
	default:
		is.createWarns = append(is.createWarns, importWarn{parse.CodeImportUnmapped, "priority", fmt.Sprintf("priority %v is not a number, not imported", p)})
	}
	if labels, ok := raw["labels"].([]any); ok {
		is.labels = labels
	}
	is.assignee = strings.TrimSpace(str(raw["assignee"]))
	keys := []string{}
	for k := range raw {
		if !beadsMapped[k] {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	for _, k := range keys {
		v := raw[k]
		if v == nil {
			continue
		}
		if !fieldKey.MatchString(k) {
			is.createWarns = append(is.createWarns, importWarn{parse.CodeImportUnmapped, k, fmt.Sprintf("field name %q is not a valid frontmatter key, not imported", k)})
			continue
		}
		name := k
		if beadsTaken[k] {
			name = "beads_" + k
		}
		is.extra = append(is.extra, node.Field{Key: name, Value: v})
	}
	is.links, is.parent = beadsLinks(id, raw["dependencies"])
	if reason := strings.TrimSpace(str(raw["close_reason"])); reason != "" {
		is.closeNote = "Closed: " + reason
	}
	comments, _ := raw["comments"].([]any)
	for _, c := range comments {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		text := strings.TrimSpace(str(cm["text"]))
		if text == "" {
			continue
		}
		ic := importComment{author: strings.TrimSpace(str(cm["author"])), text: text}
		if rawAt := strings.TrimSpace(str(cm["created_at"])); rawAt != "" {
			if t, err := time.Parse(time.RFC3339Nano, rawAt); err != nil {
				is.eventWarns = append(is.eventWarns, importWarn{parse.CodeImportUnmapped, "comments", fmt.Sprintf("comment time %q is not an RFC 3339 time, the issue creation time is used", rawAt)})
			} else {
				ic.at = t.UTC()
			}
		}
		is.comments = append(is.comments, ic)
	}
	return is
}

func beadsTime(is *importIssue, raw map[string]any, field string) time.Time {
	text := strings.TrimSpace(str(raw[field]))
	if text == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		is.createWarns = append(is.createWarns, importWarn{parse.CodeImportUnmapped, field, fmt.Sprintf("%s %q is not an RFC 3339 time, the import time is used", field, text)})
		return time.Time{}
	}
	return t.UTC()
}

func beadsLinks(id string, v any) ([]importLink, string) {
	list, _ := v.([]any)
	links, parent := []importLink{}, ""
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
		switch {
		case typ == depParentChild && parent == "":
			parent = target
		case typ == depBlocks:
			links = append(links, importLink{rel: relBlocks, other: target, inbound: true})
		case typ == depDiscovered:
			links = append(links, importLink{rel: relDerived, other: target})
		default:
			links = append(links, importLink{other: target, unmaps: fmt.Sprintf("dependency of type %q on %q has no tektite link, not imported", typ, target)})
		}
	}
	return links, parent
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
