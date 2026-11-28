package service

import (
	"bytes"
	"strings"

	"github.com/tektite-tracker/tektite/internal/node"
)

func isJSONFile(path string) bool {
	raw, err := readCapped(path)
	if err != nil {
		return false
	}
	raw = bytes.TrimPrefix(bytes.TrimSpace(raw), []byte("\xef\xbb\xbf"))
	return len(raw) > 0 && (raw[0] == '{' || raw[0] == '[')
}

func readJiraJSON(path string, res *ImportResult) (*importData, error) {
	return readJSONIssues(path, res, jsonExport{
		tracker:   "import-jira",
		single:    "key",
		notExport: "not a Jira JSON export, expected an issues list or issue objects with a key and fields",
		idField:   "key",
		missing:   "key or summary",
		idTitle: func(obj map[string]any) (string, string) {
			fields, _ := obj["fields"].(map[string]any)
			return str(obj["key"]), str(fields["summary"])
		},
		build: func(id, title string, line int, obj map[string]any) *importIssue {
			fields, _ := obj["fields"].(map[string]any)
			return jiraJSONIssue(id, title, line, fields)
		},
	})
}

func jiraJSONIssue(id, title string, line int, f map[string]any) *importIssue {
	is := &importIssue{id: id, title: title, line: line}
	is.rawStatus, is.rawPriority, is.rawTyp = nameOf(f["status"]), nameOf(f["priority"]), nameOf(f["issuetype"])
	if st, ok := f["status"].(map[string]any); ok {
		is.category = nameOf(st["statusCategory"])
	}
	is.assignee = nameOf(f["assignee"])
	if labels, ok := f["labels"].([]any); ok {
		for _, l := range labels {
			if s := strings.TrimSpace(str(l)); s != "" {
				is.labels = append(is.labels, s)
			}
		}
	}
	body := bodyText(richText(f["description"]))
	if env := strings.TrimSpace(richText(f["environment"])); env != "" {
		if body != "" {
			body += "\n"
		}
		body += "## Environment\n\n" + env + "\n"
	}
	is.body = body
	is.parent = nameOfKey(f["parent"])
	if is.parent == "" {
		is.parent = strings.TrimSpace(str(f["customfield_10014"]))
	}
	is.createdAt = jiraJSONTimeFormat.parse(is, str(f["created"]), "created")
	is.closedAt = jiraJSONTimeFormat.parse(is, str(f["resolutiondate"]), "resolutiondate")
	if !is.closedAt.IsZero() {
		if jiraDroppedRes.MatchString(nameOf(f["resolution"])) {
			is.canceledAt = is.closedAt
		} else {
			is.completedAt = is.closedAt
		}
	}
	links, _ := f["issuelinks"].([]any)
	for _, e := range links {
		l, _ := e.(map[string]any)
		kind, _ := l["type"].(map[string]any)
		name := strings.TrimSpace(str(kind["name"]))
		if out := nameOfKey(l["outwardIssue"]); out != "" {
			is.links = append(is.links, jiraLink(false, name, out))
		}
		if in := nameOfKey(l["inwardIssue"]); in != "" {
			is.links = append(is.links, jiraLink(true, name, in))
		}
	}
	if c, ok := f["comment"].(map[string]any); ok {
		comments, _ := c["comments"].([]any)
		for _, e := range comments {
			cm, _ := e.(map[string]any)
			text := strings.TrimSpace(richText(cm["body"]))
			if text == "" {
				continue
			}
			ic := importComment{author: nameOf(cm["author"]), text: text}
			ic.at = jiraJSONTimeFormat.parse(is, str(cm["created"]), "comment")
			is.comments = append(is.comments, ic)
		}
	}
	is.extra = jiraJSONExtras(f)
	return is
}

func nameOfKey(v any) string {
	m, _ := v.(map[string]any)
	return strings.TrimSpace(str(m["key"]))
}

func jiraJSONExtras(f map[string]any) []node.Field {
	fields := []node.Field{}
	if v := nameOf(f["creator"]); v != "" {
		fields = append(fields, node.Field{Key: "creator", Value: v})
	}
	if p, ok := f["project"].(map[string]any); ok {
		if k := strings.TrimSpace(str(p["key"])); k != "" {
			fields = append(fields, node.Field{Key: "project_key", Value: k})
		}
	}
	if v := nameOf(f["reporter"]); v != "" {
		fields = append(fields, node.Field{Key: "reporter", Value: v})
	}
	return fields
}

func richText(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case map[string]any:
		var sb strings.Builder
		writeADF(&sb, x)
		text := sb.String()
		for strings.Contains(text, "\n\n\n") {
			text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
		}
		return strings.TrimSpace(strings.ReplaceAll(text, "\n\n- ", "\n- "))
	}
	return ""
}

func writeADF(sb *strings.Builder, n map[string]any) {
	typ := str(n["type"])
	switch typ {
	case "text":
		sb.WriteString(markText(str(n["text"]), n["marks"]))
		return
	case "hardBreak":
		sb.WriteString("\n")
		return
	case "mention":
		if a, ok := n["attrs"].(map[string]any); ok {
			sb.WriteString(str(a["text"]))
		}
		return
	case "listItem":
		sb.WriteString("- ")
	case "heading":
		level := 1
		if a, ok := n["attrs"].(map[string]any); ok {
			if l, ok := a["level"].(float64); ok && l >= 1 && l <= 6 {
				level = int(l)
			}
		}
		sb.WriteString(strings.Repeat("#", level) + " ")
	case "codeBlock":
		sb.WriteString("```\n")
		defer sb.WriteString("```\n")
	}
	children, _ := n["content"].([]any)
	for _, c := range children {
		if cm, ok := c.(map[string]any); ok {
			writeADF(sb, cm)
		}
	}
	switch typ {
	case "paragraph", "heading", "codeBlock", "blockquote":
		sb.WriteString("\n\n")
	case "listItem":
		sb.WriteString("\n")
	}
}

func markText(text string, marks any) string {
	list, _ := marks.([]any)
	for _, m := range list {
		mm, _ := m.(map[string]any)
		switch str(mm["type"]) {
		case "strong":
			text = "**" + text + "**"
		case "em":
			text = "*" + text + "*"
		case "code":
			text = "`" + text + "`"
		case "link":
			if a, ok := mm["attrs"].(map[string]any); ok && str(a["href"]) != "" {
				text = "[" + text + "](" + str(a["href"]) + ")"
			}
		}
	}
	return text
}
