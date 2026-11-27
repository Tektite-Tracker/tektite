package parse_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/tektite-tracker/tektite/parse"
)

var update = flag.Bool("update", false, "rewrite golden files")

type nodeGolden struct {
	Node        *parse.Node        `json:"node"`
	Diagnostics []parse.Diagnostic `json:"diagnostics"`
}

type eventGolden struct {
	Event       *parse.Event       `json:"event"`
	Diagnostics []parse.Diagnostic `json:"diagnostics"`
}

func readCase(t *testing.T, kind, dir string) []byte {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "testdata", kind, dir, "input.md"))
	if err != nil {
		t.Fatal(err)
	}
	return src
}

func compareGolden(t *testing.T, kind, dir string, got any) {
	t.Helper()
	data, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	golden := filepath.Join("..", "testdata", kind, dir, "golden.json")
	if *update {
		if err := os.WriteFile(golden, data, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, want) {
		t.Errorf("golden mismatch for %s/%s\n got:\n%s\nwant:\n%s", kind, dir, data, want)
	}
}

func goldenNodeNamed(t *testing.T, dir, name string) (*parse.Node, []parse.Diagnostic) {
	t.Helper()
	n, d := parse.ParseNode(name, readCase(t, "node", dir))
	compareGolden(t, "node", dir, nodeGolden{n, d})
	return n, d
}

func goldenNode(t *testing.T, dir string) (*parse.Node, []parse.Diagnostic) {
	t.Helper()
	return goldenNodeNamed(t, dir, "input.md")
}

func goldenEvent(t *testing.T, dir string) (*parse.Event, []parse.Diagnostic) {
	t.Helper()
	e, d := parse.ParseEvent("01HZ3M0A1B2C3D4E5F6G7H8J9K.md", readCase(t, "event", dir))
	compareGolden(t, "event", dir, eventGolden{e, d})
	return e, d
}

func hasCode(d []parse.Diagnostic, code string) bool {
	for _, x := range d {
		if x.Code == code {
			return true
		}
	}
	return false
}
