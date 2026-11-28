package service

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/tektite-tracker/tektite/internal/node"
	"github.com/tektite-tracker/tektite/parse"
)

var (
	jiraMapped = map[string]bool{
		"summary": true, "issue key": true, "issue id": true, "issue type": true, "status": true, "priority": true,
		"description": true, "environment": true, "labels": true, "assignee": true, "created": true, "resolved": true,
		"parent": true, "parent key": true, "parent summary": true, "status category": true, "status category changed": true,
		"resolution": true, "comment": true, "updated": true, "last viewed": true, "votes": true, "work ratio": true,
		"custom field (rank)": true, "custom field (issue color)": true, "custom field (epic link)": true, "epic link": true,
		"project description": true, "project lead id": true, "assignee id": true, "reporter id": true, "creator id": true,
	}
	jiraLinkColumn  = regexp.MustCompile(`^(outward|inward) issue link \((.+)\)$`)
	jiraNumericDate = regexp.MustCompile(`^(\d{1,2})[-/.](\d{1,2})[-/.](\d{2,4})(?:[ T](\d{1,2}):(\d{2})(?::(\d{2}))?(?:\s?([AaPp][Mm]))?)?$`)
	jiraNamedDate   = regexp.MustCompile(`^(\d{1,2})/([A-Za-z]{3})/(\d{2,4})(?:\s+(\d{1,2}):(\d{2})(?::(\d{2}))?(?:\s?([AaPp][Mm]))?)?$`)
	jiraDroppedRes  = regexp.MustCompile(`(?i)won'?t|wont|duplicate|cannot reproduce|declined|invalid|rejected|obsolete|abandon`)
)

type jiraSource struct{}

func init() { registerImportSource(jiraSource{}) }

func (jiraSource) name() string              { return "jira" }
func (jiraSource) fixed() bool               { return false }
func (jiraSource) defaultPath(string) string { return "" }

func (jiraSource) read(path string, res *ImportResult) (*importData, error) {
	if isJSONFile(path) {
		return readJiraJSON(path, res)
	}
	t, err := readCSVTable(path, "jira")
	if err != nil {
		return nil, err
	}
	if err := t.require("Jira", "Summary", "Issue key", "Status"); err != nil {
		return nil, err
	}
	dates := detectJiraDates(t)
	if dates.ambiguous {
		res.Warnings = append(res.Warnings, importDiag(parse.CodeImportUnmapped, 0, "Created", "dates such as %q could be day first or month first, they are read as day first", dates.example))
	}
	keyByID := map[string]string{}
	for _, row := range t.rows {
		if id, key := t.get(row, "Issue id"), t.get(row, "Issue key"); id != "" && key != "" {
			keyByID[id] = key
		}
	}
	data := &importData{}
	seen := map[string]bool{}
	for n, row := range t.rows {
		line := n + 2
		id, title := t.get(row, "Issue key"), t.get(row, "Summary")
		switch {
		case id == "" || title == "":
			res.Warnings = append(res.Warnings, importDiag(parse.CodeImportSkipped, line, "Issue key", "row has no issue key or summary, skipped"))
			continue
		case seen[id]:
			res.Warnings = append(res.Warnings, importDiag(parse.CodeImportSkipped, line, "Issue key", "issue %q appears more than once, the first one is used", id))
			continue
		}
		seen[id] = true
		data.issues = append(data.issues, jiraIssueOf(t, row, id, title, line, dates, keyByID))
	}
	return data, nil
}

func jiraIssueOf(t *csvTable, row []string, id, title string, line int, dates jiraDates, keyByID map[string]string) *importIssue {
	is := &importIssue{id: id, title: title, line: line}
	is.rawStatus, is.rawPriority, is.rawTyp = t.get(row, "Status"), t.get(row, "Priority"), t.get(row, "Issue Type")
	is.category = t.get(row, "Status Category")
	is.assignee = t.get(row, "Assignee")
	for _, l := range t.all(row, "Labels") {
		for _, one := range strings.Fields(l) {
			is.labels = append(is.labels, one)
		}
	}
	body := bodyText(t.get(row, "Description"))
	if env := strings.TrimSpace(t.get(row, "Environment")); env != "" {
		if body != "" {
			body += "\n"
		}
		body += "## Environment\n\n" + env + "\n"
	}
	is.body = body
	parent := t.get(row, "Parent key")
	for _, col := range []string{"Parent", "Custom field (Epic Link)", "Epic Link"} {
		if parent == "" {
			parent = t.get(row, col)
		}
	}
	if key, ok := keyByID[parent]; ok {
		parent = key
	}
	is.parent = parent
	is.createdAt = dates.parse(is, t.get(row, "Created"), "Created")
	is.closedAt = dates.parse(is, t.get(row, "Resolved"), "Resolved")
	if !is.closedAt.IsZero() {
		if jiraDroppedRes.MatchString(t.get(row, "Resolution")) {
			is.canceledAt = is.closedAt
		} else {
			is.completedAt = is.closedAt
		}
	}
	for n, h := range t.headers {
		m := jiraLinkColumn.FindStringSubmatch(strings.ToLower(strings.TrimSpace(h)))
		if m == nil || n >= len(row) {
			continue
		}
		for _, other := range splitList(row[n]) {
			is.links = append(is.links, jiraLink(m[1] == "inward", m[2], other))
		}
	}
	for _, c := range t.all(row, "Comment") {
		is.comments = append(is.comments, jiraComment(dates, c))
	}
	is.extra = jiraExtras(t, row)
	return is
}

func jiraExtras(t *csvTable, row []string) []node.Field {
	return leftoverColumns(t, row, func(h string) bool {
		return jiraMapped[h] || jiraLinkColumn.MatchString(h) || strings.HasPrefix(h, "σ")
	}, "jira")
}

func jiraLink(inward bool, kind, other string) importLink {
	switch normalizeName(kind) {
	case "blocks", "blocked":
		return importLink{rel: relBlocks, other: other, inbound: inward}
	case "duplicate", "duplicates":
		return importLink{rel: relSupersedes, other: other, inbound: !inward}
	case "relates", "relates to", "relate", "related":
		return importLink{rel: relRelates, other: other}
	case "cloners", "clones", "clone":
		return importLink{rel: relDerived, other: other, inbound: inward}
	}
	slug := strings.Trim(nonKey.ReplaceAllString(strings.ToLower(kind), "-"), "-")
	return importLink{rel: slug, other: other, inbound: inward}
}

func jiraComment(dates jiraDates, raw string) importComment {
	parts := strings.SplitN(raw, ";", 3)
	if len(parts) == 3 {
		if at, ok := dates.try(parts[0]); ok {
			return importComment{text: strings.TrimSpace(parts[2]), at: at}
		}
	}
	return importComment{text: strings.TrimSpace(raw)}
}

type jiraDates struct {
	monthFirst bool
	ambiguous  bool
	example    string
}

func detectJiraDates(t *csvTable) jiraDates {
	d := jiraDates{}
	decided := false
	for _, row := range t.rows {
		for _, col := range []string{"Created", "Resolved", "Updated"} {
			m := jiraNumericDate.FindStringSubmatch(t.get(row, col))
			if m == nil {
				continue
			}
			a, _ := strconv.Atoi(m[1])
			b, _ := strconv.Atoi(m[2])
			switch {
			case a > 12:
				d.monthFirst, decided = false, true
			case b > 12:
				d.monthFirst, decided = true, true
			case !decided && d.example == "":
				d.example = m[0]
			}
			if decided {
				return jiraDates{monthFirst: d.monthFirst}
			}
		}
	}
	d.ambiguous = d.example != ""
	return d
}

func (d jiraDates) parse(is *importIssue, raw, column string) time.Time {
	if raw == "" {
		return time.Time{}
	}
	if t, ok := d.try(raw); ok {
		return t
	}
	is.createWarns = append(is.createWarns, importWarn{parse.CodeImportUnmapped, column, fmt.Sprintf("%s %q is not a recognised date, the import time is used", column, raw)})
	return time.Time{}
}

func (d jiraDates) try(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t.UTC(), true
	}
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC(), true
		}
	}
	var year, month, day, hour, minute, second int
	var meridiem string
	if m := jiraNumericDate.FindStringSubmatch(raw); m != nil {
		day, _ = strconv.Atoi(m[1])
		month, _ = strconv.Atoi(m[2])
		if d.monthFirst {
			day, month = month, day
		}
		year, hour, minute, second, meridiem = atoiYear(m[3]), atoiOr(m[4]), atoiOr(m[5]), atoiOr(m[6]), m[7]
	} else if m := jiraNamedDate.FindStringSubmatch(raw); m != nil {
		day, _ = strconv.Atoi(m[1])
		mon, ok := monthNumber(m[2])
		if !ok {
			return time.Time{}, false
		}
		month = mon
		year, hour, minute, second, meridiem = atoiYear(m[3]), atoiOr(m[4]), atoiOr(m[5]), atoiOr(m[6]), m[7]
	} else {
		return time.Time{}, false
	}
	switch strings.ToLower(meridiem) {
	case "pm":
		if hour < 12 {
			hour += 12
		}
	case "am":
		if hour == 12 {
			hour = 0
		}
	}
	if month < 1 || month > 12 || day < 1 || day > 31 || hour > 23 || minute > 59 {
		return time.Time{}, false
	}
	t := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.UTC)
	if t.Day() != day {
		return time.Time{}, false
	}
	return t, true
}

func atoiOr(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func atoiYear(s string) int {
	n, _ := strconv.Atoi(s)
	if len(s) <= 2 {
		return 2000 + n
	}
	return n
}

func monthNumber(s string) (int, bool) {
	for i, m := range []string{"jan", "feb", "mar", "apr", "may", "jun", "jul", "aug", "sep", "oct", "nov", "dec"} {
		if strings.EqualFold(s, m) {
			return i + 1, true
		}
	}
	return 0, false
}
