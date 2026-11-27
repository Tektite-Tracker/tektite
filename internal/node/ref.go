package node

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	MinHashLen = 3
	maxSlugLen = 40
)

var ErrNotFound = errors.New("no node matches the reference")

// AmbiguousError reports a reference that matches more than one node.
type AmbiguousError struct {
	Query   string
	Matches []string
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("reference %q is ambiguous: %s", e.Query, strings.Join(e.Matches, ", "))
}

// Slug derives the fixed slug part of a reference from a title.
func Slug(title string) string {
	var b strings.Builder
	pendingDash := false
	for _, r := range strings.ToLower(title) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if pendingDash && b.Len() > 0 {
				b.WriteByte('-')
			}
			pendingDash = false
			b.WriteRune(r)
			continue
		}
		pendingDash = true
	}
	s := b.String()
	if len(s) > maxSlugLen {
		s = strings.TrimRight(s[:maxSlugLen], "-")
	}
	if s == "" {
		return "node"
	}
	return s
}

// HashOf returns the hash part of a reference or bare hash.
func HashOf(ref string) string {
	if i := strings.LastIndexByte(ref, '-'); i >= 0 {
		return ref[i+1:]
	}
	return ref
}

// NewRef builds slug-hash for a node, lengthening the hash from the end of
// its id until no existing reference has an equal hash or a hash ending in it.
func NewRef(title, id string, existing []string) (string, error) {
	lower := strings.ToLower(id)
	if len(lower) < MinHashLen {
		return "", fmt.Errorf("id %q is too short to derive a hash", id)
	}
	taken := make([]string, len(existing))
	for i, ref := range existing {
		taken[i] = strings.ToLower(HashOf(ref))
	}
	for n := MinHashLen; n <= len(lower); n++ {
		h := lower[len(lower)-n:]
		if !collides(h, taken) {
			return Slug(title) + "-" + h, nil
		}
	}
	return "", fmt.Errorf("no unique hash for id %q", id)
}

func collides(h string, taken []string) bool {
	for _, t := range taken {
		if strings.HasSuffix(t, h) {
			return true
		}
	}
	return false
}

// Resolve finds the node a reference points to. Only the hash part counts, so
// a stale slug still resolves. An exact hash wins over a suffix match, and
// several suffix matches are rejected as ambiguous.
func Resolve(refs []string, query string) (string, error) {
	q := strings.ToLower(HashOf(strings.TrimSpace(query)))
	if q == "" {
		return "", ErrNotFound
	}
	var exact, suffix []string
	for _, ref := range refs {
		h := strings.ToLower(HashOf(ref))
		switch {
		case h == q:
			exact = append(exact, ref)
		case strings.HasSuffix(h, q):
			suffix = append(suffix, ref)
		}
	}
	matches := exact
	if len(matches) == 0 {
		matches = suffix
	}
	switch len(matches) {
	case 0:
		return "", ErrNotFound
	case 1:
		return matches[0], nil
	}
	sort.Strings(matches)
	return "", &AmbiguousError{Query: query, Matches: matches}
}
