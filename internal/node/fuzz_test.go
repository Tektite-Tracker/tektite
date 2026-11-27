package node_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/tektite-tracker/tektite/internal/node"
)

var slugShape = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func FuzzSlug(f *testing.F) {
	f.Add("Add retry on 429")
	f.Add("  --  ")
	f.Add("Ünï — Hello")
	f.Fuzz(func(t *testing.T, title string) {
		s := node.Slug(title)
		if !slugShape.MatchString(s) || len(s) > 40 {
			t.Fatalf("bad slug %q for %q", s, title)
		}
		if node.Slug(s) != s {
			t.Fatalf("slug %q is not stable", s)
		}
	})
}

func FuzzNewRef(f *testing.F) {
	f.Add("Title", "01HZ3K8Q5W2X9N4T7V6B1C0D3F", "a-d3f\nb-0d3f")
	f.Fuzz(func(t *testing.T, title, id, joined string) {
		existing := strings.Split(joined, "\n")
		ref, err := node.NewRef(title, id, existing)
		if err != nil {
			return
		}
		h := node.HashOf(ref)
		if len(h) < node.MinHashLen || !strings.HasSuffix(strings.ToLower(id), h) {
			t.Fatalf("bad hash %q for id %q", h, id)
		}
		for _, e := range existing {
			if strings.HasSuffix(strings.ToLower(node.HashOf(e)), h) {
				t.Fatalf("hash %q collides with %q", h, e)
			}
		}
	})
}

func FuzzResolve(f *testing.F) {
	f.Add("auth-retry-3fa\nauth-hardening-7c1", "3fa")
	f.Add("a-3fa\nb-3fa", "x-3fa")
	f.Fuzz(func(t *testing.T, joined, query string) {
		refs := strings.Split(joined, "\n")
		got, err := node.Resolve(refs, query)
		if err != nil {
			return
		}
		found := false
		for _, r := range refs {
			if r == got {
				found = true
			}
		}
		if !found {
			t.Fatalf("%q is not one of %q", got, refs)
		}
		q := strings.ToLower(node.HashOf(strings.TrimSpace(query)))
		if len(q) < node.MinHashLen || !strings.HasSuffix(strings.ToLower(node.HashOf(got)), q) {
			t.Fatalf("%q does not match query %q", got, query)
		}
	})
}

func FuzzCycles(f *testing.F) {
	f.Add([]byte{1, 0, 0, 2})
	f.Add([]byte{0, 1, 2, 3, 4})
	f.Fuzz(func(t *testing.T, edges []byte) {
		if len(edges) > 64 {
			edges = edges[:64]
		}
		n := len(edges)
		name := func(i int) string { return string(rune('a' + i)) }
		parents := map[string]string{}
		for i, e := range edges {
			if int(e)%(n+1) == n {
				parents[name(i)] = ""
				continue
			}
			parents[name(i)] = name(int(e) % (n + 1))
		}
		inCycle := map[string]bool{}
		for _, c := range node.Cycles(parents) {
			for i, id := range c {
				if parents[id] != c[(i+1)%len(c)] {
					t.Fatalf("%v is not a cycle in %v", c, parents)
				}
				if inCycle[id] {
					t.Fatalf("%s reported twice", id)
				}
				inCycle[id] = true
			}
		}
		for id := range parents {
			anc, cyc := node.Ancestors(parents, id)
			seen := map[string]bool{id: true}
			for _, a := range anc {
				if seen[a] {
					t.Fatalf("ancestors of %s repeat %s", id, a)
				}
				seen[a] = true
			}
			if inCycle[id] && !cyc {
				t.Fatalf("%s is in a cycle but Ancestors did not report it", id)
			}
		}
	})
}
