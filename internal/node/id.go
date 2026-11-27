package node

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

const (
	timeChars   = 10
	randomChars = 16
	maxMillis   = int64(1)<<48 - 1
)

var (
	ErrTimeRange = errors.New("time is outside the ULID range")
	ErrOverflow  = errors.New("ULID entropy overflow within one millisecond")
)

type Generator struct {
	mu     sync.Mutex
	r      io.Reader
	millis int64
	digits [randomChars]byte
	primed bool
}

func NewGenerator(r io.Reader) *Generator {
	return &Generator{r: r}
}

func (g *Generator) New(t time.Time) (string, error) {
	ms := t.UnixMilli()
	if ms < 0 || ms > maxMillis {
		return "", ErrTimeRange
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.primed && ms <= g.millis {
		ms = g.millis
		if !increment(&g.digits) {
			return "", ErrOverflow
		}
	} else {
		d, err := randomDigits(g.r)
		if err != nil {
			return "", err
		}
		g.digits = d
	}
	g.millis, g.primed = ms, true
	return encode(ms, g.digits), nil
}

func NewID(t time.Time, r io.Reader) (string, error) {
	ms := t.UnixMilli()
	if ms < 0 || ms > maxMillis {
		return "", ErrTimeRange
	}
	d, err := randomDigits(r)
	if err != nil {
		return "", err
	}
	return encode(ms, d), nil
}

func randomDigits(r io.Reader) ([randomChars]byte, error) {
	var d [randomChars]byte
	buf := make([]byte, randomChars)
	if _, err := io.ReadFull(r, buf); err != nil {
		return d, fmt.Errorf("read entropy: %w", err)
	}
	for i, b := range buf {
		d[i] = b & 31
	}
	return d, nil
}

func increment(d *[randomChars]byte) bool {
	for i := randomChars - 1; i >= 0; i-- {
		if d[i] < 31 {
			d[i]++
			return true
		}
		d[i] = 0
	}
	return false
}

func encode(ms int64, d [randomChars]byte) string {
	out := make([]byte, timeChars+randomChars)
	v := uint64(ms)
	for i := timeChars - 1; i >= 0; i-- {
		out[i] = alphabet[v&31]
		v >>= 5
	}
	for i, x := range d {
		out[timeChars+i] = alphabet[x]
	}
	return string(out)
}
