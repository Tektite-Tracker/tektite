package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tektite-tracker/tektite/parse"
)

const (
	schemasDir     = "schemas"
	SourceBuiltIn  = "built-in"
	SourceLocal    = "local"
	templateHeader = "## template"
)

var lifecycle = []string{"open", "active", "blocked", "review", "done", "dropped"}

type Schema struct {
	Kind        string         `json:"kind"`
	Required    []string       `json:"required"`
	Statuses    []string       `json:"statuses"`
	ParentKinds []string       `json:"parent_kinds"`
	Template    string         `json:"template"`
	Source      string         `json:"source"`
	Path        string         `json:"path,omitempty"`
	Extra       map[string]any `json:"extra,omitempty"`
}

type SchemaProblem struct {
	Path string
	Diag parse.Diagnostic
}

type Schemas struct {
	byKind   map[string]*Schema
	Problems []SchemaProblem
}

const (
	CodeSchemaKindMismatch = "TKT-W110"
	CodeSchemaMalformed    = "TKT-E102"
)

func builtIns() map[string]*Schema {
	mk := func(kind, template string) *Schema {
		return &Schema{
			Kind:        kind,
			Required:    []string{},
			Statuses:    append([]string{}, lifecycle...),
			ParentKinds: []string{},
			Template:    template,
			Source:      SourceBuiltIn,
		}
	}
	return map[string]*Schema{
		"spec":     mk("spec", ""),
		"plan":     mk("plan", ""),
		"ticket":   mk("ticket", "### Context\n\n### Acceptance criteria"),
		"gap":      mk("gap", ""),
		"evidence": mk("evidence", ""),
		"decision": mk("decision", ""),
	}
}

func (r *Repo) LoadSchemas() (*Schemas, error) {
	s := &Schemas{byKind: builtIns()}
	entries, err := readDir(filepath.Join(r.Dir(), schemasDir))
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if !isMarkdown(e) {
			continue
		}
		path := filepath.Join(r.Dir(), schemasDir, e.Name())
		src, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read schema %s: %w", r.Rel(path), err)
		}
		s.apply(r.Rel(path), strings.TrimSuffix(e.Name(), extension), src)
	}
	return s, nil
}

func (s *Schemas) apply(path, name string, src []byte) {
	n, diags := parse.ParseNode(name+extension, src)
	for _, d := range diags {
		if d.Code == parse.CodeMalformedFrontmatter || d.Code == parse.CodeUnterminatedFrontmtr {
			s.Problems = append(s.Problems, SchemaProblem{Path: path, Diag: schemaDiag(CodeSchemaMalformed, "schema is ignored: "+d.Message, d.Line, "")})
			return
		}
	}
	if !n.Frontmatter {
		s.Problems = append(s.Problems, SchemaProblem{Path: path, Diag: schemaDiag(CodeSchemaMalformed, "schema has no frontmatter and is ignored", 0, "")})
		return
	}
	kind := name
	if declared, ok := n.Fields["kind"].(string); ok && strings.TrimSpace(declared) != "" && declared != name {
		s.Problems = append(s.Problems, SchemaProblem{Path: path, Diag: schemaDiag(CodeSchemaKindMismatch, fmt.Sprintf("file name says kind %q but the schema declares %q, applying it to %q", name, declared, declared), 0, "kind")})
		kind = declared
	}
	cur, ok := s.byKind[kind]
	if !ok {
		cur = &Schema{Kind: kind, Required: []string{}, Statuses: []string{}, ParentKinds: []string{}}
	}
	merged := *cur
	merged.Source, merged.Path = SourceLocal, path
	if v, ok := n.Fields["required"]; ok {
		merged.Required = stringList(v)
	}
	if v, ok := n.Fields["statuses"]; ok {
		merged.Statuses = stringList(v)
	}
	if v, ok := n.Fields["parent_kinds"]; ok {
		merged.ParentKinds = stringList(v)
	}
	if t, ok := templateOf(n.Body); ok {
		merged.Template = t
	}
	extra := map[string]any{}
	for k, v := range n.Fields {
		switch k {
		case "kind", "required", "statuses", "parent_kinds":
		default:
			extra[k] = v
		}
	}
	if len(extra) > 0 {
		merged.Extra = extra
	}
	s.byKind[kind] = &merged
}

func (s *Schemas) For(kind string) (*Schema, bool) {
	sc, ok := s.byKind[kind]
	return sc, ok
}

func (s *Schemas) List() []*Schema {
	out := make([]*Schema, 0, len(s.byKind))
	for _, sc := range s.byKind {
		out = append(out, sc)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	return out
}

func templateOf(body string) (string, bool) {
	lines := strings.Split(body, "\n")
	start := -1
	for i, l := range lines {
		if strings.EqualFold(strings.TrimSpace(l), templateHeader) {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return "", false
	}
	end := len(lines)
	for i := start; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			end = i
			break
		}
	}
	return strings.TrimSpace(strings.Join(lines[start:end], "\n")), true
}

func stringList(v any) []string {
	out := []string{}
	switch x := v.(type) {
	case []any:
		for _, item := range x {
			if item != nil {
				out = append(out, strings.TrimSpace(fmt.Sprint(item)))
			}
		}
	case string:
		if strings.TrimSpace(x) != "" {
			out = append(out, strings.TrimSpace(x))
		}
	}
	return out
}

func schemaDiag(code, msg string, line int, field string) parse.Diagnostic {
	sev := parse.SeverityWarning
	if code[4] == 'E' {
		sev = parse.SeverityError
	}
	return parse.Diagnostic{Code: code, Severity: sev, Message: msg, Line: line, Field: field}
}
