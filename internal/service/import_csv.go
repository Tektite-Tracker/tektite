package service

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
)

type csvTable struct {
	headers []string
	rows    [][]string
	idx     map[string][]int
}

func readCSVTable(path, tracker string) (*csvTable, error) {
	raw, err := readCapped(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, newError(KindNotFound, "pass --from to point at the "+tracker+" CSV export", "no %s export at %s", tracker, filepath.ToSlash(path))
		}
		return nil, fmt.Errorf("import-%s: %w", tracker, err)
	}
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	r := csv.NewReader(bytes.NewReader(raw))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	records, err := r.ReadAll()
	if err != nil {
		return nil, usageError("%s is not a readable CSV file: %v", filepath.ToSlash(path), err)
	}
	if len(records) == 0 {
		return nil, usageError("%s is empty", filepath.ToSlash(path))
	}
	t := &csvTable{headers: records[0], rows: records[1:], idx: map[string][]int{}}
	for i, h := range t.headers {
		key := strings.ToLower(strings.TrimSpace(h))
		t.idx[key] = append(t.idx[key], i)
	}
	return t, nil
}

func (t *csvTable) has(name string) bool {
	return len(t.idx[strings.ToLower(name)]) > 0
}

func (t *csvTable) require(tracker string, names ...string) error {
	missing := []string{}
	for _, n := range names {
		if !t.has(n) {
			missing = append(missing, n)
		}
	}
	if len(missing) > 0 {
		return usageError("not a %s CSV export, missing column %s", tracker, strings.Join(missing, ", "))
	}
	return nil
}

func (t *csvTable) all(row []string, name string) []string {
	out := []string{}
	for _, i := range t.idx[strings.ToLower(name)] {
		if i < len(row) {
			if v := strings.TrimSpace(row[i]); v != "" {
				out = append(out, v)
			}
		}
	}
	return out
}

func (t *csvTable) get(row []string, name string) string {
	if v := t.all(row, name); len(v) > 0 {
		return v[0]
	}
	return ""
}

var nonKey = regexp.MustCompile(`[^a-z0-9]+`)

func columnKey(header string) string {
	h := strings.TrimSpace(header)
	lower := strings.ToLower(h)
	if strings.HasPrefix(lower, "custom field (") && strings.HasSuffix(lower, ")") {
		lower = strings.TrimSuffix(strings.TrimPrefix(lower, "custom field ("), ")")
	}
	return strings.Trim(nonKey.ReplaceAllString(lower, "_"), "_")
}

func splitList(s string) []string {
	out := []string{}
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
