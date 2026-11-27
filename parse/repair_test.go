package parse_test

import (
	"strings"
	"testing"
	"time"

	"github.com/tektite-tracker/tektite/parse"
)

func TestRepair_Title(t *testing.T) {
	t.Run("Malformed frontmatter takes its title from a heading or the file name", func(t *testing.T) {
		withHeading, d := parse.ParseNode("x.md", []byte("---\ntitle: [unclosed\n---\n# Real title\n"))
		if withHeading.Title != "Real title" || !hasCode(d, parse.CodeMalformedFrontmatter) {
			t.Errorf("title=%q diags=%v", withHeading.Title, d)
		}
		bare, _ := parse.ParseNode("loose-idea.md", []byte("---\ntitle: [unclosed\n---\nno heading\n"))
		if bare.Title != "loose-idea" {
			t.Errorf("title = %q", bare.Title)
		}
	})

	t.Run("The title falls back to the first heading of any level", func(t *testing.T) {
		n, _ := parse.ParseNode("x.md", []byte("### Deep heading\n\ntext\n"))
		if n.Title != "Deep heading" {
			t.Errorf("title = %q", n.Title)
		}
	})
}

func TestRepair_Frontmatter(t *testing.T) {
	t.Run("An unterminated frontmatter block is reported", func(t *testing.T) {
		n, d := parse.ParseNode("x.md", []byte("---\nid: 1\n# Heading\n"))
		if !hasCode(d, parse.CodeUnterminatedFrontmtr) || !strings.HasPrefix(n.Body, "---") {
			t.Errorf("body=%q diags=%v", n.Body, d)
		}
		lone, ld := parse.ParseNode("x.md", []byte("---"))
		if !hasCode(ld, parse.CodeUnterminatedFrontmtr) || lone.Body != "---" {
			t.Errorf("lone marker: body=%q diags=%v", lone.Body, ld)
		}
		ev, ed := parse.ParseEvent("x.md", []byte("---"))
		if ev.Valid || !hasCode(ed, parse.CodeUnterminatedFrontmtr) {
			t.Errorf("lone marker event: %v", ed)
		}
	})

	t.Run("Duplicate frontmatter keys keep the last value with a warning", func(t *testing.T) {
		n, d := parse.ParseNode("x.md", []byte("---\ntitle: first\nkind: spec\ntitle: second\n---\n"))
		if n.Title != "second" || strings.Join(n.FieldOrder, ",") != "title,kind" {
			t.Errorf("title=%q order=%v", n.Title, n.FieldOrder)
		}
		var found bool
		for _, x := range d {
			if x.Code == parse.CodeDuplicateKey && x.Field == "title" && x.Severity == parse.SeverityWarning {
				found = true
			}
		}
		if !found {
			t.Errorf("diags = %v", d)
		}
	})

	t.Run("A non-string value in a core field is coerced or reported", func(t *testing.T) {
		n, d := parse.ParseNode("x.md", []byte("---\nid: 42\ntitle: [a, b]\n---\n"))
		if n.ID != "42" || !hasCode(d, parse.CodeUnexpectedFieldType) {
			t.Errorf("id=%q diags=%v", n.ID, d)
		}
		if n.Title != "x" {
			t.Errorf("title = %q", n.Title)
		}
	})
}

func TestRepair_Blocks(t *testing.T) {
	t.Run("The g marker is the only gap marker", func(t *testing.T) {
		n, _ := parse.ParseNode("x.md", []byte("- [ ] <!-- g:01HZ3KB2 --> missing retry\n- [ ] <!-- t:01HZ3KB3 --> plain task\n"))
		if len(n.Blocks) != 2 || !n.Blocks[0].Gap || n.Blocks[1].Gap {
			t.Errorf("blocks = %+v", n.Blocks)
		}
	})

	t.Run("A Gaps section ends at the next heading of the same or higher level", func(t *testing.T) {
		src := "## Gaps\n\n- [ ] one\n\n### Detail\n\n- [ ] two\n\n## Notes\n\n- [ ] three\n"
		n, _ := parse.ParseNode("x.md", []byte(src))
		if len(n.Blocks) != 3 || !n.Blocks[0].Gap || !n.Blocks[1].Gap || n.Blocks[2].Gap {
			t.Errorf("blocks = %+v", n.Blocks)
		}
	})

	t.Run("Only checklist items are blocks", func(t *testing.T) {
		n, _ := parse.ParseNode("x.md", []byte("- not a task\n- [ ] a task\n"))
		if len(n.Blocks) != 1 || n.Blocks[0].Text != "a task" {
			t.Errorf("blocks = %+v", n.Blocks)
		}
	})

	t.Run("Wikilink aliases and anchors resolve to the target, and code is ignored", func(t *testing.T) {
		n, _ := parse.ParseNode("x.md", []byte("see [[a|label]] and [[a#part]] and [[a]] but not `[[b]]`\n"))
		if len(n.Links) != 1 || n.Links[0].Target != "a" {
			t.Errorf("links = %+v", n.Links)
		}
	})
}

func TestRepair_Events(t *testing.T) {
	t.Run("An event file whose name is not a ULID is a warning", func(t *testing.T) {
		src := []byte("---\ntype: note\nactor: a\nat: 2026-11-27T10:00:00Z\n---\n")
		e, d := parse.ParseEvent("first-try.md", src)
		if !e.Valid || !hasCode(d, parse.CodeEventNameNotULID) || e.ID != "first-try" {
			t.Errorf("event=%+v diags=%v", e, d)
		}
		_, ok := parse.ParseEvent("01HZ3M0A1B2C3D4E5F6G7H8J9K.md", src)
		if hasCode(ok, parse.CodeEventNameNotULID) {
			t.Errorf("a ULID name must not warn: %v", ok)
		}
	})

	t.Run("The event time is a YAML timestamp or an RFC 3339 string", func(t *testing.T) {
		bare, _ := parse.ParseEvent("x.md", []byte("---\ntype: note\nactor: a\nat: 2026-11-27T10:00:00Z\n---\n"))
		quoted, _ := parse.ParseEvent("x.md", []byte("---\ntype: note\nactor: a\nat: \"2026-11-27T10:00:00Z\"\n---\n"))
		want := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
		if !bare.At.Equal(want) || !quoted.At.Equal(want) {
			t.Errorf("at = %v and %v", bare.At, quoted.At)
		}
		bad, d := parse.ParseEvent("x.md", []byte("---\ntype: note\nactor: a\nat: yesterday\n---\n"))
		if bad.Valid || !hasCode(d, parse.CodeInvalidEventTime) {
			t.Errorf("event=%+v diags=%v", bad, d)
		}
	})

	t.Run("An empty required field counts as missing", func(t *testing.T) {
		e, d := parse.ParseEvent("x.md", []byte("---\ntype: link\nactor: a\nat: 2026-11-27T10:00:00Z\nrel: blocks\ntarget: \"\"\n---\n"))
		if e.Valid || !hasCode(d, parse.CodeMissingRequiredField) || d[len(d)-1].Field != "target" {
			t.Errorf("event=%+v diags=%v", e, d)
		}
	})
}
