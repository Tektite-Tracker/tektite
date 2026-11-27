package node_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/tektite-tracker/tektite/internal/node"
)

func TestRef(t *testing.T) {
	t.Run("The slug is derived from the title plus a short hash of the id", func(t *testing.T) {
		ref, err := node.NewRef("Add retry on 429", "01HZ3K8Q5W2X9N4T7V6B1C0D3F", nil)
		if err != nil || ref != "add-retry-on-429-d3f" {
			t.Errorf("ref=%q err=%v", ref, err)
		}
		if got := node.Slug("  Ünï — Hello,  World!! "); got != "n-hello-world" {
			t.Errorf("slug = %q", got)
		}
		if node.Slug("???") != "node" {
			t.Error("empty slug should fall back")
		}
		if got := node.Slug(strings.Repeat("word ", 20)); len(got) > 40 || strings.HasSuffix(got, "-") {
			t.Errorf("slug = %q", got)
		}
	})

	t.Run("The hash is at least three characters and unique at creation", func(t *testing.T) {
		id := "01HZ3K8Q5W2X9N4T7V6B1C0D3F"
		ref, _ := node.NewRef("Other", id, []string{"auth-retry-d3f"})
		if ref != "other-0d3f" {
			t.Errorf("ref = %q", ref)
		}
		ref, _ = node.NewRef("Other", id, []string{"a-0d3f", "b-d3f"})
		if ref != "other-c0d3f" {
			t.Errorf("ref = %q", ref)
		}
		ref, _ = node.NewRef("Other", id, []string{"a-x1d3f"})
		if ref != "other-0d3f" {
			t.Errorf("ref = %q", ref)
		}
		if _, err := node.NewRef("x", id, []string{"a-" + strings.ToLower(id)}); err == nil {
			t.Error("exhausted hashes should fail")
		}
	})
}

func TestResolve(t *testing.T) {
	refs := []string{"auth-retry-3fa", "auth-hardening-7c1"}

	t.Run("A reference resolves by its hash suffix even if the slug part is stale", func(t *testing.T) {
		got, err := node.Resolve([]string{"add-retry-on-429-3fa"}, "retry-ticket-3fa")
		if err != nil || got != "add-retry-on-429-3fa" {
			t.Errorf("got=%q err=%v", got, err)
		}
	})

	t.Run("A reference resolves by unique hash suffix", func(t *testing.T) {
		got, err := node.Resolve(refs, "3fa")
		if err != nil || got != "auth-retry-3fa" {
			t.Errorf("got=%q err=%v", got, err)
		}
		if got, _ := node.Resolve(refs, "c1"); got != "" {
			t.Errorf("short query resolved to %q", got)
		}
		if _, err := node.Resolve(refs, "fa"); !errors.Is(err, node.ErrTooShort) {
			t.Errorf("err = %v", err)
		}
		if got, _ := node.Resolve(refs, "a-3fa"); got != "auth-retry-3fa" {
			t.Errorf("suffix got=%q", got)
		}
		if _, err := node.Resolve(refs, "zzz"); !errors.Is(err, node.ErrNotFound) {
			t.Errorf("err = %v", err)
		}
		if got, _ := node.Resolve([]string{"a-3fa", "b-e3fa"}, "3fa"); got != "a-3fa" {
			t.Errorf("exact hash should win, got %q", got)
		}
	})

	t.Run("A reference with fewer than three hash characters is rejected", func(t *testing.T) {
		if _, err := node.Resolve(refs, "fa"); !errors.Is(err, node.ErrTooShort) {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("An ambiguous reference is rejected", func(t *testing.T) {
		_, err := node.Resolve([]string{"b-17fa", "a-3fa", "c-9c1"}, "7fa")
		var amb *node.AmbiguousError
		if err != nil {
			t.Errorf("unique suffix should resolve: %v", err)
		}
		_, err = node.Resolve([]string{"b-17fa", "a-27fa", "c-9c1"}, "7fa")
		if !errors.As(err, &amb) || !reflect.DeepEqual(amb.Matches, []string{"a-27fa", "b-17fa"}) {
			t.Errorf("err = %v", err)
		}
		_, err = node.Resolve([]string{"a-3fa", "b-3fa"}, "x-3fa")
		if !errors.As(err, &amb) {
			t.Errorf("duplicate hashes should be ambiguous: %v", err)
		}
	})
}
