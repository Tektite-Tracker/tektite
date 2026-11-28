package service

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/tektite-tracker/tektite/internal/node"
	"github.com/tektite-tracker/tektite/parse"
)

var (
	linearMapped = map[string]bool{
		"id": true, "title": true, "description": true, "status": true, "priority": true, "labels": true, "assignee": true,
		"created": true, "started": true, "completed": true, "canceled": true, "parent issue": true,
		"related to": true, "blocked by": true, "duplicate of": true,
		"updated": true, "time in status (minutes)": true,
	}
	trackerTaken = map[string]bool{"id": true, "kind": true, "title": true, "parent": true, sourceField: true, sourceIDField: true, "priority": true, "labels": true, "assignee": true}
)

type linearSource struct{}

func init() { registerImportSource(linearSource{}) }

func (linearSource) name() string              { return "linear" }
func (linearSource) fixed() bool               { return false }
func (linearSource) defaultPath(string) string { return "" }

func (linearSource) read(path string, res *ImportResult) (*importData, error) {
	if isJSONFile(path) {
		return readLinearJSON(path, res)
	}
	t, err := readCSVTable(path, "linear")
	if err != nil {
		return nil, err
	}
	if err := t.require("Linear", "ID", "Title", "Status"); err != nil {
		return nil, err
	}
	data := &importData{}
	seen := map[string]bool{}
	for n, row := range t.rows {
		line := n + 2
		id, title := t.get(row, "ID"), t.get(row, "Title")
		switch {
		case id == "" || title == "":
			res.Warnings = append(res.Warnings, importDiag(parse.CodeImportSkipped, line, "ID", "row has no ID or title, skipped"))
			continue
		case seen[id]:
			res.Warnings = append(res.Warnings, importDiag(parse.CodeImportSkipped, line, "ID", "issue %q appears more than once, the first one is used", id))
			continue
		}
		seen[id] = true
		data.issues = append(data.issues, linearIssueOf(t, row, id, title, line))
	}
	return data, nil
}

func linearIssueOf(t *csvTable, row []string, id, title string, line int) *importIssue {
	is := &importIssue{id: id, title: title, line: line, body: bodyText(t.get(row, "Description"))}
	is.rawStatus, is.rawPriority = t.get(row, "Status"), t.get(row, "Priority")
	is.assignee = t.get(row, "Assignee")
	for _, l := range splitList(t.get(row, "Labels")) {
		is.labels = append(is.labels, l)
	}
	is.parent = t.get(row, "Parent issue")
	is.createdAt = csvTime(is, t.get(row, "Created"), "Created")
	is.startedAt = csvTime(is, t.get(row, "Started"), "Started")
	is.completedAt = csvTime(is, t.get(row, "Completed"), "Completed")
	is.canceledAt = csvTime(is, t.get(row, "Canceled"), "Canceled")
	is.closedAt = is.completedAt
	if !is.canceledAt.IsZero() {
		is.closedAt = is.canceledAt
	}
	for _, o := range splitList(t.get(row, "Blocked by")) {
		is.links = append(is.links, importLink{rel: relBlocks, other: o, inbound: true})
	}
	for _, o := range splitList(t.get(row, "Related to")) {
		is.links = append(is.links, importLink{rel: relRelates, other: o})
	}
	for _, o := range splitList(t.get(row, "Duplicate of")) {
		is.links = append(is.links, importLink{rel: relSupersedes, other: o, inbound: true})
	}
	is.extra = leftoverColumns(t, row, func(h string) bool { return linearMapped[h] }, "linear")
	return is
}

func bodyText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return s + "\n"
}

func csvTime(is *importIssue, raw, column string) time.Time {
	if raw == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC()
		}
	}
	is.createWarns = append(is.createWarns, importWarn{parse.CodeImportUnmapped, column, fmt.Sprintf("%s %q is not a recognised time, the import time is used", column, raw)})
	return time.Time{}
}

func leftoverColumns(t *csvTable, row []string, skip func(string) bool, tracker string) []node.Field {
	type col struct {
		key    string
		values []string
	}
	cols := []col{}
	seen := map[string]int{}
	for _, h := range t.headers {
		lower := strings.ToLower(strings.TrimSpace(h))
		if skip(lower) {
			continue
		}
		key := columnKey(h)
		if key == "" || !fieldKey.MatchString(key) {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = len(cols)
		cols = append(cols, col{key: key, values: t.all(row, h)})
	}
	slices.SortFunc(cols, func(a, b col) int { return strings.Compare(a.key, b.key) })
	fields := []node.Field{}
	for _, c := range cols {
		if len(c.values) == 0 {
			continue
		}
		name := c.key
		if trackerTaken[name] {
			name = tracker + "_" + name
		}
		if len(c.values) == 1 {
			fields = append(fields, node.Field{Key: name, Value: c.values[0]})
			continue
		}
		list := make([]any, len(c.values))
		for i, v := range c.values {
			list[i] = v
		}
		fields = append(fields, node.Field{Key: name, Value: list})
	}
	return fields
}
