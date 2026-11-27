package node

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/tektite-tracker/tektite/parse"
)

const (
	MinHashLen = 3
	maxSlugLen = 40
)

var (
	ErrNotFound = errors.New("no node matches the reference")
	ErrTooShort = errors.New("reference hash is shorter than the minimum")
)

type AmbiguousError struct {
	Query   string
	Matches []string
}

func (e *AmbiguousError) Error() string {
	return fmt.Sprintf("reference %q is ambiguous: %s", e.Query, strings.Join(e.Matches, ", "))
}

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
	return s
}

func HashOf(ref string) string {
	if i := strings.LastIndexByte(ref, '-'); i >= 0 {
		return ref[i+1:]
	}
	return ref
}

func NewRef(title, id string, existing []string) (string, error) {
	if !parse.IsULID(id) {
		return "", fmt.Errorf("id %q is not a ULID", id)
	}
	lower := strings.ToLower(id)
	taken := make([]string, len(existing))
	for i, ref := range existing {
		taken[i] = strings.ToLower(HashOf(ref))
	}
	for n := MinHashLen; n <= len(lower); n++ {
		h := lower[len(lower)-n:]
		if !collides(h, taken) {
			if slug := Slug(title); slug != "" {
				return slug + "-" + h, nil
			}
			return h, nil
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

func Resolve(refs []string, query string) (string, error) {
	q := strings.ToLower(HashOf(strings.TrimSpace(query)))
	if len(q) < MinHashLen {
		return "", ErrTooShort
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
	slices.Sort(matches)
	return "", &AmbiguousError{Query: query, Matches: matches}
}
