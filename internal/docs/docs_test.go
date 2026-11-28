package docs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestList(t *testing.T) {
	t.Run("Listing the topics", func(t *testing.T) {
		fsys := fstest.MapFS{
			"zeta.md":       {Data: []byte("# Zeta page\n")},
			"index.md":      {Data: []byte("# Home\n")},
			"alpha.md":      {Data: []byte("no heading\n")},
			"notes.txt":     {Data: []byte("skip")},
			"cli/deploy.md": {Data: []byte("# Deploy\n")},
		}
		got, err := list(fsys)
		if err != nil {
			t.Fatal(err)
		}
		want := []Topic{
			{"index", "Home"},
			{"alpha", "alpha"},
			{"cli/deploy", "Deploy"},
			{"zeta", "Zeta page"},
		}
		if len(got) != len(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("topic %d = %v, want %v", i, got[i], want[i])
			}
		}
	})

	t.Run("The index lists every topic", func(t *testing.T) {
		topics, err := List()
		if err != nil {
			t.Fatal(err)
		}
		index, err := Read("index")
		if err != nil {
			t.Fatal(err)
		}
		for _, tp := range topics {
			if tp.Topic != "index" && !strings.Contains(index, "]("+tp.Topic+".md)") {
				t.Errorf("docs/index.md does not link %s", tp.Topic)
			}
		}
	})
}

func TestRead(t *testing.T) {
	fsys := fstest.MapFS{
		"design.md":     {Data: []byte("# Design\nbody\n")},
		"cli/deploy.md": {Data: []byte("# Deploy\n")},
	}

	t.Run("Printing a topic", func(t *testing.T) {
		got, err := read(fsys, "design")
		if err != nil || got != "# Design\nbody\n" {
			t.Errorf("got %q, %v", got, err)
		}
	})

	t.Run("A nested topic is addressed by its path", func(t *testing.T) {
		got, err := read(fsys, "cli/deploy")
		if err != nil || got != "# Deploy\n" {
			t.Errorf("got %q, %v", got, err)
		}
	})

	t.Run("An unknown topic is refused", func(t *testing.T) {
		for _, name := range []string{"nope", "", "../go", "design.txt", "cli"} {
			if _, err := read(fsys, name); !errors.Is(err, ErrNotFound) {
				t.Errorf("%q: err = %v, want ErrNotFound", name, err)
			}
		}
	})
}

func TestEmbeddedPagesMatchDisk(t *testing.T) {
	t.Run("Printing a topic", func(t *testing.T) {
		want, err := os.ReadFile(filepath.Join("..", "..", "docs", "design.md"))
		if err != nil {
			t.Fatal(err)
		}
		got, err := Read("design")
		if err != nil || got != string(want) {
			t.Errorf("embedded design differs from docs/design.md: %v", err)
		}
	})
}
