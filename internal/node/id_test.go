package node_test

import (
	"bytes"
	"errors"
	"math/rand"
	"testing"
	"time"

	"github.com/tektite-tracker/tektite/internal/node"
	"github.com/tektite-tracker/tektite/parse"
)

func TestID(t *testing.T) {
	t.Run("Identity is a ULID", func(t *testing.T) {
		at := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
		a, err := node.NewID(at, bytes.NewReader(bytes.Repeat([]byte{1}, 16)))
		if err != nil || !parse.IsULID(a) {
			t.Fatalf("id=%q err=%v", a, err)
		}
		b, _ := node.NewID(at.Add(time.Second), bytes.NewReader(bytes.Repeat([]byte{1}, 16)))
		if a >= b {
			t.Errorf("ids should sort by time: %s %s", a, b)
		}
		if _, err := node.NewID(at, bytes.NewReader(nil)); err == nil {
			t.Error("short entropy should fail")
		}
		if _, err := node.NewID(time.UnixMilli(-1), bytes.NewReader(make([]byte, 16))); !errors.Is(err, node.ErrTimeRange) {
			t.Errorf("negative time: %v", err)
		}
		if _, err := node.NewID(time.UnixMilli(1<<48), bytes.NewReader(make([]byte, 16))); !errors.Is(err, node.ErrTimeRange) {
			t.Errorf("overflowing time: %v", err)
		}
	})

	t.Run("Ids created in the same millisecond sort in creation order", func(t *testing.T) {
		at := time.Date(2026, 11, 27, 10, 0, 0, 0, time.UTC)
		g := node.NewGenerator(rand.New(rand.NewSource(1)))
		prev := ""
		for i := 0; i < 500; i++ {
			when := at
			if i%7 == 0 {
				when = at.Add(-time.Second)
			}
			id, err := g.New(when)
			if err != nil || !parse.IsULID(id) || id <= prev {
				t.Fatalf("i=%d id=%q prev=%q err=%v", i, id, prev, err)
			}
			prev = id
		}
		full := node.NewGenerator(bytes.NewReader(bytes.Repeat([]byte{31}, 16)))
		if _, err := full.New(at); err != nil {
			t.Fatal(err)
		}
		if _, err := full.New(at); !errors.Is(err, node.ErrOverflow) {
			t.Errorf("overflow: %v", err)
		}
	})
}
