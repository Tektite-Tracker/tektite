package node_test

import (
	"reflect"
	"testing"

	"github.com/tektite-tracker/tektite/internal/node"
)

func TestTree(t *testing.T) {
	t.Run("Nodes nest to any depth", func(t *testing.T) {
		parents := map[string]string{"sub": "ticket", "ticket": "plan", "plan": "spec", "spec": ""}
		got, cyc := node.Ancestors(parents, "sub")
		if cyc || !reflect.DeepEqual(got, []string{"ticket", "plan", "spec"}) {
			t.Errorf("ancestors=%v cycle=%v", got, cyc)
		}
		if len(node.Cycles(parents)) != 0 {
			t.Error("no cycles expected")
		}
	})

	t.Run("A cycle in parents is reported and not followed", func(t *testing.T) {
		parents := map[string]string{"a": "b", "b": "a", "c": "a", "self": "self", "d": ""}
		want := [][]string{{"a", "b"}, {"self"}}
		if got := node.Cycles(parents); !reflect.DeepEqual(got, want) {
			t.Errorf("cycles = %v", got)
		}
		got, cyc := node.Ancestors(parents, "c")
		if !cyc || !reflect.DeepEqual(got, []string{"a", "b"}) {
			t.Errorf("ancestors=%v cycle=%v", got, cyc)
		}
		if got, cyc := node.Ancestors(parents, "a"); !cyc || !reflect.DeepEqual(got, []string{"b"}) {
			t.Errorf("ancestors=%v cycle=%v", got, cyc)
		}
	})
}
