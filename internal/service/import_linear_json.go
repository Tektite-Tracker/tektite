package service

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/tektite-tracker/tektite/internal/node"
	"github.com/tektite-tracker/tektite/parse"
)

const linearTruncated = "(truncated, use `get_issue` for full description)"

var linearImage = regexp.MustCompile(`(?s)<linear-image>.*?</linear-image>`)

func readLinearJSON(path string, res *ImportResult) (*importData, error) {
	return readJSONIssues(path, res, jsonExport{
		tracker:   "import-linear",
		single:    "id",
		notExport: "not a Linear JSON export, expected an issues list or issue objects with an id and a title",
		idField:   "id",
		missing:   "id or title",
		idTitle: func(obj map[string]any) (string, string) {
			return str(obj["id"]), str(obj["title"])
		},
		build: linearJSONIssue,
	})
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
	is.createdAt = csvTimeFormat.parse(is, str(o["createdAt"]), "createdAt")
	is.startedAt = csvTimeFormat.parse(is, str(o["startedAt"]), "startedAt")
	is.completedAt = csvTimeFormat.parse(is, str(o["completedAt"]), "completedAt")
	is.canceledAt = csvTimeFormat.parse(is, str(o["canceledAt"]), "canceledAt")
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
