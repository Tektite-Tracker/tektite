package node

import (
	"fmt"
	"io"
	"time"
)

const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

const (
	timeChars   = 10
	randomChars = 16
)

// NewID returns a ULID for t using entropy from r.
func NewID(t time.Time, r io.Reader) (string, error) {
	out := make([]byte, timeChars+randomChars)
	ms := uint64(t.UnixMilli())
	for i := timeChars - 1; i >= 0; i-- {
		out[i] = alphabet[ms&31]
		ms >>= 5
	}
	buf := make([]byte, randomChars)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", fmt.Errorf("read entropy: %w", err)
	}
	for i, b := range buf {
		out[timeChars+i] = alphabet[b&31]
	}
	return string(out), nil
}
