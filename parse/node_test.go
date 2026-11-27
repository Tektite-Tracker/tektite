package parse_test

import (
	"strings"
	"testing"

	"github.com/tektite-tracker/tektite/parse"
)

func TestNode_ParseCompleteNode(t *testing.T) {
	t.Run("Parsing a complete node", func(t *testing.T) {
		n, d := goldenNode(t, "complete")
		if n.ID != "01HZ3K8Q5W2X9N4T7V6B1C0D3F" || n.Kind != "ticket" || n.Title != "Add retry on 429" || n.Parent != "auth-hardening-7c1" {
			t.Errorf("unexpected node %+v", n)
		}
		if n.Body != "The client should back off and retry when the API returns 429." {
			t.Errorf("unexpected body %q", n.Body)
		}
		if len(d) != 0 {
			t.Errorf("unexpected diagnostics %v", d)
		}
	})
}

func TestNode_OptionalFields(t *testing.T) {
	t.Run("Optional fields are preserved but not required", func(t *testing.T) {
		n, _ := goldenNode(t, "optional-fields")
		if _, ok := n.Fields["labels"]; !ok {
			t.Error("labels missing")
		}
		if _, ok := n.Fields["priority"]; !ok {
			t.Error("priority missing")
		}
		bare, d := goldenNode(t, "complete")
		if _, ok := bare.Fields["labels"]; ok || len(d) != 0 {
			t.Error("node without optional fields should be equally valid")
		}
	})
}

func TestNode_RootNode(t *testing.T) {
	t.Run("A root node has no parent", func(t *testing.T) {
		n, d := goldenNode(t, "root-node")
		if n.Parent != "" {
			t.Errorf("parent = %q", n.Parent)
		}
		if len(d) != 0 {
			t.Errorf("unexpected diagnostics %v", d)
		}
	})
}

func TestNode_UnknownKind(t *testing.T) {
	t.Run("An unknown kind is accepted", func(t *testing.T) {
		n, d := goldenNode(t, "unknown-kind")
		if n.Kind != "retro" || len(d) != 0 {
			t.Errorf("kind=%q diags=%v", n.Kind, d)
		}
	})
}

func TestNode_NoFrontmatter(t *testing.T) {
	t.Run("A file with no frontmatter is a valid node", func(t *testing.T) {
		n, d := goldenNode(t, "no-frontmatter")
		if n.Kind != "note" || n.Title != "Notes on session handling" {
			t.Errorf("kind=%q title=%q", n.Kind, n.Title)
		}
		if !hasCode(d, parse.CodeMissingID) {
			t.Errorf("missing id not reported: %v", d)
		}
		fallback, _ := goldenNodeNamed(t, "no-heading", "loose-idea.md")
		if fallback.Title != "loose-idea" {
			t.Errorf("filename fallback title = %q", fallback.Title)
		}
	})
}

func TestNode_MalformedFrontmatter(t *testing.T) {
	t.Run("Malformed frontmatter does not block reads", func(t *testing.T) {
		n, d := goldenNode(t, "malformed-yaml")
		if !strings.HasPrefix(n.Body, "---\n") {
			t.Errorf("whole file should be body, got %q", n.Body)
		}
		if n.Title != "Heading survives" {
			t.Errorf("title = %q", n.Title)
		}
		var found *parse.Diagnostic
		for i := range d {
			if d[i].Code == parse.CodeMalformedFrontmatter {
				found = &d[i]
			}
		}
		if found == nil || found.Line == 0 {
			t.Errorf("yaml error with line number expected: %v", d)
		}
		u, ud := goldenNode(t, "unterminated")
		if !hasCode(ud, parse.CodeUnterminatedFrontmtr) || !strings.HasPrefix(u.Body, "---") {
			t.Errorf("unterminated frontmatter mishandled: %v", ud)
		}
	})
}

func TestNode_UnknownFrontmatterFields(t *testing.T) {
	t.Run("Unknown frontmatter fields are preserved", func(t *testing.T) {
		n, _ := goldenNode(t, "unknown-fields")
		if n.Fields["reviewed_by"] != "human:jesus" {
			t.Errorf("reviewed_by = %v", n.Fields["reviewed_by"])
		}
		want := []string{"id", "kind", "title", "parent", "reviewed_by", "nested"}
		if strings.Join(n.FieldOrder, ",") != strings.Join(want, ",") {
			t.Errorf("field order = %v", n.FieldOrder)
		}
	})
}

func TestNode_ChecklistBlocks(t *testing.T) {
	t.Run("A checklist item can carry a stable block id", func(t *testing.T) {
		n, _ := goldenNode(t, "block-id")
		b := n.Blocks[0]
		if b.ID != "01HZ3KB2" || b.Text != "missing retry on 429" || b.Checked {
			t.Errorf("block = %+v", b)
		}
		if !n.Blocks[1].Checked {
			t.Error("second block should be checked")
		}
	})

	t.Run("Blocks without ids are still parsed", func(t *testing.T) {
		n, _ := goldenNode(t, "block-no-id")
		if len(n.Blocks) != 1 || n.Blocks[0].ID != "" || n.Blocks[0].Text != "handle timeouts" {
			t.Errorf("blocks = %+v", n.Blocks)
		}
	})

	t.Run("A hand-written GAPS.md list is treated like gap blocks", func(t *testing.T) {
		n, _ := goldenNode(t, "gaps-section")
		gaps := 0
		for _, b := range n.Blocks {
			if b.Gap {
				gaps++
			}
		}
		if gaps != 3 || len(n.Blocks) != 5 {
			t.Errorf("gaps=%d blocks=%d", gaps, len(n.Blocks))
		}
		g, _ := goldenNode(t, "gaps-md")
		for _, b := range g.Blocks {
			if !b.Gap {
				t.Errorf("block %q should be a gap", b.Text)
			}
		}
	})
}

func TestNode_Wikilinks(t *testing.T) {
	t.Run("Wikilinks in prose are recognised as links", func(t *testing.T) {
		n, _ := goldenNode(t, "wikilinks")
		var targets []string
		for _, l := range n.Links {
			targets = append(targets, l.Target)
		}
		want := "auth-retry-3fa,auth-hardening-7c1,does-not-exist-000"
		if strings.Join(targets, ",") != want {
			t.Errorf("targets = %v", targets)
		}
		c, _ := goldenNode(t, "code-masking")
		if len(c.Links) != 1 || c.Links[0].Target != "real-222" || len(c.Blocks) != 0 {
			t.Errorf("code should be ignored: links=%v blocks=%v", c.Links, c.Blocks)
		}
	})
}

func TestNode_StatusField(t *testing.T) {
	t.Run("A status field in frontmatter is ignored with a warning", func(t *testing.T) {
		n, d := goldenNode(t, "status-field")
		if !hasCode(d, parse.CodeStatusInFrontmatter) {
			t.Fatalf("warning missing: %v", d)
		}
		if d[0].Severity != parse.SeverityWarning {
			t.Errorf("severity = %v", d[0].Severity)
		}
		if n.Kind != "ticket" {
			t.Errorf("kind = %q", n.Kind)
		}
	})
}

func TestNode_LenientInput(t *testing.T) {
	t.Run("A BOM and CRLF line endings do not change the result", func(t *testing.T) {
		n, d := goldenNode(t, "crlf-bom")
		if n.Title != "Windows file" || len(n.Blocks) != 1 || n.Blocks[0].ID != "ABC12345" || len(d) != 0 {
			t.Errorf("node=%+v diags=%v", n, d)
		}
	})
}

func TestSplitFrontmatter(t *testing.T) {
	t.Run("Frontmatter and body can be split without rewriting the body", func(t *testing.T) {
		fm, body, ok := parse.SplitFrontmatter([]byte("\xef\xbb\xbf---\r\nid: x\r\n---\r\n# Body\r\n\r\ntext  \r\n"))
		if !ok || string(fm) != "id: x\n" || string(body) != "# Body\n\ntext  \n" {
			t.Errorf("fm=%q body=%q ok=%v", fm, body, ok)
		}
		for _, in := range []string{"plain\n", "---\nid: x\nno close\n", "---"} {
			fm, body, ok := parse.SplitFrontmatter([]byte(in))
			if ok || fm != nil || string(body) != in {
				t.Errorf("%q: fm=%q body=%q ok=%v", in, fm, body, ok)
			}
		}
	})
}
