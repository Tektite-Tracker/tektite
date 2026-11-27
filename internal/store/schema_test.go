package store_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/tektite-tracker/tektite/internal/store"
	"github.com/tektite-tracker/tektite/parse"
)

const ticketSchema = `---
kind: ticket
required: [estimate]
statuses: [open, active, blocked, review, done, dropped]
parent_kinds: [plan, ticket]
---
## Template
### Context
### Acceptance criteria
`

func loadSchemas(t *testing.T, files map[string]string) *store.Schemas {
	t.Helper()
	r, root := repo(t)
	for name, content := range files {
		write(t, root, ".tektite/schemas/"+name, content)
	}
	s, err := r.LoadSchemas()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestSchemas(t *testing.T) {
	t.Run("Loading a schema", func(t *testing.T) {
		s := loadSchemas(t, map[string]string{"ticket.md": ticketSchema})
		sc, ok := s.For("ticket")
		if !ok || !reflect.DeepEqual(sc.Required, []string{"estimate"}) || !reflect.DeepEqual(sc.ParentKinds, []string{"plan", "ticket"}) {
			t.Fatalf("schema = %+v", sc)
		}
		if len(sc.Statuses) != 6 || sc.Template != "### Context\n### Acceptance criteria" || sc.Source != store.SourceLocal || sc.Path != ".tektite/schemas/ticket.md" {
			t.Errorf("schema = %+v", sc)
		}
		if len(s.Problems) != 0 {
			t.Errorf("problems = %+v", s.Problems)
		}
	})

	t.Run("The file name must match the kind", func(t *testing.T) {
		s := loadSchemas(t, map[string]string{"ticket.md": "---\nkind: plan\nrequired: [owner]\n---\n"})
		sc, ok := s.For("plan")
		if !ok || !reflect.DeepEqual(sc.Required, []string{"owner"}) {
			t.Errorf("plan schema = %+v", sc)
		}
		tk, _ := s.For("ticket")
		if len(tk.Required) != 0 || tk.Source != store.SourceBuiltIn {
			t.Errorf("ticket schema = %+v", tk)
		}
		if len(s.Problems) != 1 || s.Problems[0].Diag.Code != store.CodeSchemaKindMismatch || s.Problems[0].Diag.Severity != parse.SeverityWarning ||
			s.Problems[0].Path != ".tektite/schemas/ticket.md" {
			t.Errorf("problems = %+v", s.Problems)
		}
	})

	t.Run("A malformed schema is reported and ignored", func(t *testing.T) {
		s := loadSchemas(t, map[string]string{"gap.md": "---\nkind: [unclosed\n---\n", "plan.md": "no frontmatter"})
		if len(s.Problems) != 2 {
			t.Fatalf("problems = %+v", s.Problems)
		}
		for _, p := range s.Problems {
			if p.Diag.Code != store.CodeSchemaMalformed || p.Diag.Severity != parse.SeverityError {
				t.Errorf("problem = %+v", p)
			}
		}
		if s.Problems[0].Path != ".tektite/schemas/gap.md" || s.Problems[0].Diag.Line == 0 {
			t.Errorf("problem = %+v", s.Problems[0])
		}
		gap, _ := s.For("gap")
		if gap.Source != store.SourceBuiltIn {
			t.Errorf("gap = %+v", gap)
		}
	})

	t.Run("Unknown keys in a schema are preserved", func(t *testing.T) {
		s := loadSchemas(t, map[string]string{"ticket.md": "---\nkind: ticket\nowner: team-a\n---\n"})
		sc, _ := s.For("ticket")
		if sc.Extra["owner"] != "team-a" {
			t.Errorf("extra = %v", sc.Extra)
		}
	})

	t.Run("Defaults exist without any schema file", func(t *testing.T) {
		s := loadSchemas(t, nil)
		sc, ok := s.For("plan")
		if !ok || sc.Source != store.SourceBuiltIn || len(sc.Statuses) != 6 {
			t.Errorf("plan = %+v", sc)
		}
	})

	t.Run("A local schema overrides defaults key by key", func(t *testing.T) {
		s := loadSchemas(t, map[string]string{"ticket.md": "---\nkind: ticket\nrequired: [estimate]\n---\n"})
		sc, _ := s.For("ticket")
		builtIn, _ := loadSchemas(t, nil).For("ticket")
		if !reflect.DeepEqual(sc.Required, []string{"estimate"}) ||
			!reflect.DeepEqual(sc.Statuses, builtIn.Statuses) || !reflect.DeepEqual(sc.ParentKinds, builtIn.ParentKinds) || sc.Template != builtIn.Template {
			t.Errorf("schema = %+v, built-in = %+v", sc, builtIn)
		}
	})

	t.Run("The built-in kinds", func(t *testing.T) {
		s := loadSchemas(t, map[string]string{"ticket.md": ticketSchema})
		got := map[string]string{}
		for _, sc := range s.List() {
			got[sc.Kind] = sc.Source
		}
		want := map[string]string{"decision": "built-in", "evidence": "built-in", "gap": "built-in", "plan": "built-in", "spec": "built-in", "ticket": "local"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("kinds = %v", got)
		}
		kinds := []string{}
		for _, sc := range s.List() {
			kinds = append(kinds, sc.Kind)
		}
		if strings.Join(kinds, ",") != "decision,evidence,gap,plan,spec,ticket" {
			t.Errorf("order = %v", kinds)
		}
	})

	t.Run("Custom kinds need no schema", func(t *testing.T) {
		s := loadSchemas(t, nil)
		if _, ok := s.For("retro"); ok {
			t.Error("retro must have no schema")
		}
	})
}
