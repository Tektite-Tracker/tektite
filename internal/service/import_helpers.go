package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/tektite-tracker/tektite/internal/node"
	"github.com/tektite-tracker/tektite/parse"
)

var trackerTaken = map[string]bool{"id": true, "kind": true, "title": true, "parent": true, sourceField: true, sourceIDField: true, "priority": true, "labels": true, "assignee": true}

type jsonExport struct {
	tracker   string
	single    string
	notExport string
	idField   string
	missing   string
	idTitle   func(obj map[string]any) (id, title string)
	build     func(id, title string, line int, obj map[string]any) *importIssue
}

func readJSONIssues(path string, res *ImportResult, x jsonExport) (*importData, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", x.tracker, err)
	}
	raw = bytes.TrimPrefix(bytes.TrimSpace(raw), []byte("\xef\xbb\xbf"))
	var list []any
	switch raw[0] {
	case '[':
		if err := json.Unmarshal(raw, &list); err != nil {
			return nil, usageError("%s is not valid JSON: %v", filepath.ToSlash(path), err)
		}
	default:
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			return nil, usageError("%s is not valid JSON: %v", filepath.ToSlash(path), err)
		}
		if issues, ok := doc["issues"].([]any); ok {
			list = issues
		} else if _, ok := doc[x.single]; ok {
			list = []any{doc}
		} else {
			return nil, usageError("%s", x.notExport)
		}
	}
	data := &importData{}
	seen := map[string]bool{}
	for n, e := range list {
		obj, _ := e.(map[string]any)
		id, title := x.idTitle(obj)
		id, title = strings.TrimSpace(id), strings.TrimSpace(title)
		line := n + 1
		switch {
		case id == "" || title == "":
			res.Warnings = append(res.Warnings, importDiag(parse.CodeImportSkipped, line, x.idField, "issue %d has no %s, skipped", n+1, x.missing))
			continue
		case seen[id]:
			res.Warnings = append(res.Warnings, importDiag(parse.CodeImportSkipped, line, x.idField, "issue %q appears more than once, the first one is used", id))
			continue
		}
		seen[id] = true
		data.issues = append(data.issues, x.build(id, title, line, obj))
	}
	return data, nil
}

type timeFormat struct {
	layouts []string
	noun    string
	trim    bool
}

var (
	csvTimeFormat      = timeFormat{layouts: []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"}, noun: "time"}
	jiraJSONTimeFormat = timeFormat{layouts: []string{"2006-01-02T15:04:05.000-0700", "2006-01-02T15:04:05-0700", time.RFC3339Nano}, noun: "date", trim: true}
)

func (f timeFormat) parse(is *importIssue, raw, field string) time.Time {
	if f.trim {
		raw = strings.TrimSpace(raw)
	}
	if raw == "" {
		return time.Time{}
	}
	for _, layout := range f.layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC()
		}
	}
	is.createWarns = append(is.createWarns, importWarn{parse.CodeImportUnmapped, field, fmt.Sprintf("%s %q is not a recognised %s, the import time is used", field, raw, f.noun)})
	return time.Time{}
}

func bodyText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return s + "\n"
}

func nameOf(v any) string {
	m, _ := v.(map[string]any)
	for _, k := range []string{"name", "displayName", "value", "key"} {
		if s := strings.TrimSpace(str(m[k])); s != "" {
			return s
		}
	}
	return ""
}

func nameOfString(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return nameOf(v)
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
