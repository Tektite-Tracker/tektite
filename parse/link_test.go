package parse_test

import (
	"testing"

	"github.com/tektite-tracker/tektite/parse"
)

func relTargets(links []parse.Link) []string {
	out := []string{}
	for _, l := range links {
		out = append(out, l.Rel+">"+l.Target)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestLinks_Typed(t *testing.T) {
	t.Run("Declaring typed links", func(t *testing.T) {
		src := "---\nid: 01HZ3K8Q5W2X9N4T7V6B1C0D3F\nkind: ticket\ntitle: T\nimplements: [retry-plan-9de]\nblocks: [release-notes-1ab]\nderived-from: \"[[src-4cd]]\"\n---\nbody\n"
		n, _ := parse.ParseNode("n.md", []byte(src))
		want := []string{"implements>retry-plan-9de", "blocks>release-notes-1ab", "derived-from>src-4cd"}
		if got := relTargets(n.Links); !equal(got, want) {
			t.Errorf("links = %v, want %v", got, want)
		}
		if n.Links[0].Line != 5 {
			t.Errorf("line = %d", n.Links[0].Line)
		}
	})

	t.Run("The built-in link types", func(t *testing.T) {
		want := map[string]string{
			"implements": "implemented-by", "blocks": "blocked-by", "evidences": "evidenced-by",
			"supersedes": "superseded-by", "derived-from": "source-of",
		}
		if len(parse.LinkTypes) != len(want) {
			t.Fatalf("types = %v", parse.LinkTypes)
		}
		for _, rel := range parse.LinkTypes {
			if got := parse.Inverse(rel); got != want[rel] {
				t.Errorf("Inverse(%s) = %s, want %s", rel, got, want[rel])
			}
			if parse.Inverse(want[rel]) != rel || !parse.IsBuiltinLink(rel) {
				t.Errorf("%s does not round trip", rel)
			}
		}
	})

	t.Run("An unknown link type is accepted", func(t *testing.T) {
		n, diags := parse.ParseNode("n.md", []byte("inspired-by:: [[a-111]]\n"))
		if got := relTargets(n.Links); !equal(got, []string{"inspired-by>a-111"}) {
			t.Errorf("links = %v", got)
		}
		for _, d := range diags {
			if d.Severity == parse.SeverityError {
				t.Errorf("unexpected error %+v", d)
			}
		}
	})
}

func TestLinks_Wikilinks(t *testing.T) {
	t.Run(`A wikilink is a "mentions" link`, func(t *testing.T) {
		n, _ := parse.ParseNode("n.md", []byte("see [[auth-retry-3fa]]\n"))
		if got := relTargets(n.Links); !equal(got, []string{"mentions>auth-retry-3fa"}) {
			t.Errorf("links = %v", got)
		}
	})

	t.Run("A typed wikilink uses a prefix", func(t *testing.T) {
		n, _ := parse.ParseNode("n.md", []byte("implements:: [[retry-plan-9de]]\n"))
		if got := relTargets(n.Links); !equal(got, []string{"implements>retry-plan-9de"}) {
			t.Errorf("links = %v", got)
		}
	})

	t.Run("A wikilink with an alias keeps the target", func(t *testing.T) {
		n, _ := parse.ParseNode("n.md", []byte("[[auth-retry-3fa|the retry ticket]]\n"))
		if len(n.Links) != 1 || n.Links[0].Target != "auth-retry-3fa" || n.Links[0].Display != "the retry ticket" {
			t.Errorf("links = %+v", n.Links)
		}
	})

	t.Run("A wikilink to a block targets that block", func(t *testing.T) {
		n, _ := parse.ParseNode("n.md", []byte("[[auth-retry-3fa#01HZ3KB2]]\n"))
		if len(n.Links) != 1 || n.Links[0].Target != "auth-retry-3fa" || n.Links[0].Block != "01HZ3KB2" {
			t.Errorf("links = %+v", n.Links)
		}
	})

	t.Run("Wikilinks inside code spans and fences are ignored", func(t *testing.T) {
		n, _ := parse.ParseNode("n.md", []byte("```\n[[not-a-link]]\n```\nand `[[also-not]]`\n"))
		if len(n.Links) != 0 {
			t.Errorf("links = %+v", n.Links)
		}
	})
}
