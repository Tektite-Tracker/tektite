package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tektite-tracker/tektite/internal/node"
	"github.com/tektite-tracker/tektite/parse"
)

const linearTruncated = "(truncated, use `get_issue` for full description)"

var linearImage = regexp.MustCompile(`(?s)<linear-image>.*?</linear-image>`)

func readLinearJSON(path string, res *ImportResult) (*importData, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("import-linear: %w", err)
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
		} else if _, ok := doc["id"]; ok {
			list = []any{doc}
		} else {
			return nil, usageError("not a Linear JSON export, expected an issues list or issue objects with an id and a title")
		}
	}
	data := &importData{}
	seen := map[string]bool{}
	for n, e := range list {
		obj, _ := e.(map[string]any)
		id, title := strings.TrimSpace(str(obj["id"])), strings.TrimSpace(str(obj["title"]))
		line := n + 1
		switch {
		case id == "" || title == "":
			res.Warnings = append(res.Warnings, importDiag(parse.CodeImportSkipped, line, "id", "issue %d has no id or title, skipped", n+1))
			continue
		case seen[id]:
			res.Warnings = append(res.Warnings, importDiag(parse.CodeImportSkipped, line, "id", "issue %q appears more than once, the first one is used", id))
			continue
		}
		seen[id] = true
		data.issues = append(data.issues, linearJSONIssue(id, title, line, obj))
	}
	return data, nil
}

func linearJSONIssue(id, title string, line int, o map[string]any) *importIssue {
	is := &importIssue{id: id, title: title, line: line}
	desc := str(o["description"])
	if strings.Contains(desc, linearTruncated) {
		desc = strings.TrimSpace(strings.Replace(desc, linearTruncated, "", 1))
		desc = strings.TrimSuffix(desc, "…")
		is.createWarns = append(is.createWarns, importWarn{parse.CodeImportUnmapped, "description", "the description was truncated by the export, save the issue with get_issue to import all of it"})
	}
	is.body = bodyText(linearImage.ReplaceAllString(desc, "(image omitted)"))
	is.rawStatus, is.category, is.rawPriority = str(o["status"]), str(o["statusType"]), nameOf(o["priority"])
	if is.rawPriority == "" {
		is.rawPriority = str(o["priority"])
	}
	is.assignee = nameOfString(o["assignee"])
	if labels, ok := o["labels"].([]any); ok {
		for _, l := range labels {
			if name := nameOfString(l); name != "" {
				is.labels = append(is.labels, name)
			}
		}
	}
	is.parent = nameOfString(o["parentId"])
	is.createdAt = csvTime(is, str(o["createdAt"]), "createdAt")
	is.startedAt = csvTime(is, str(o["startedAt"]), "startedAt")
	is.completedAt = csvTime(is, str(o["completedAt"]), "completedAt")
	is.canceledAt = csvTime(is, str(o["canceledAt"]), "canceledAt")
	is.closedAt = is.completedAt
	if !is.canceledAt.IsZero() {
		is.closedAt = is.canceledAt
	}
	if rel, ok := o["relations"].(map[string]any); ok {
		for _, other := range relationIDs(rel["blocks"]) {
			is.links = append(is.links, importLink{rel: relBlocks, other: other})
		}
		for _, other := range relationIDs(rel["blockedBy"]) {
			is.links = append(is.links, importLink{rel: relBlocks, other: other, inbound: true})
		}
		for _, other := range relationIDs(rel["relatedTo"]) {
			is.links = append(is.links, importLink{rel: relRelates, other: other})
		}
		for _, other := range relationIDs(rel["duplicateOf"]) {
			is.links = append(is.links, importLink{rel: relSupersedes, other: other, inbound: true})
		}
	}
	extra := []node.Field{}
	for _, f := range []struct{ key, value string }{
		{"creator", nameOfString(o["createdBy"])},
		{"due_date", nameOfString(o["dueDate"])},
		{"estimate", estimateOf(o["estimate"])},
		{"project", nameOfString(o["project"])},
		{"team", nameOfString(o["team"])},
	} {
		if f.value != "" {
			extra = append(extra, node.Field{Key: f.key, Value: f.value})
		}
	}
	is.extra = extra
	return is
}

func nameOfString(v any) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return nameOf(v)
}

func estimateOf(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	if n, ok := m["value"].(float64); ok {
		return fmt.Sprint(n)
	}
	return nameOf(m)
}

func relationIDs(v any) []string {
	ids := []string{}
	add := func(e any) {
		switch x := e.(type) {
		case string:
			if x = strings.TrimSpace(x); x != "" {
				ids = append(ids, x)
			}
		case map[string]any:
			if id := strings.TrimSpace(str(x["id"])); id != "" {
				ids = append(ids, id)
			}
		}
	}
	if list, ok := v.([]any); ok {
		for _, e := range list {
			add(e)
		}
		return ids
	}
	add(v)
	return ids
}
