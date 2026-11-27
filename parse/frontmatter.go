package parse

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"

	"gopkg.in/yaml.v3"
)

type split struct {
	present    bool
	yaml       []byte
	body       []byte
	bodyLine   int
	unterminat bool
}

func normalize(src []byte) []byte {
	src = bytes.TrimPrefix(src, []byte("\xef\xbb\xbf"))
	if bytes.Contains(src, []byte("\r\n")) {
		src = bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
	}
	return src
}

func splitFrontmatter(src []byte) split {
	if bytes.Equal(src, []byte("---")) {
		return split{body: src, bodyLine: 1, unterminat: true}
	}
	if !bytes.HasPrefix(src, []byte("---\n")) {
		return split{body: src, bodyLine: 1}
	}
	rest := src[4:]
	line := 2
	offset := 0
	for offset <= len(rest) {
		end := bytes.IndexByte(rest[offset:], '\n')
		var text []byte
		next := len(rest)
		if end < 0 {
			text = rest[offset:]
		} else {
			text = rest[offset : offset+end]
			next = offset + end + 1
		}
		if string(bytes.TrimRight(text, " \t")) == "---" {
			return split{present: true, yaml: rest[:offset], body: rest[next:], bodyLine: line + 1}
		}
		if end < 0 {
			break
		}
		offset = next
		line++
	}
	return split{body: src, bodyLine: 1, unterminat: true}
}

type frontmatter struct {
	keys   []string
	fields map[string]any
	dups   []string
}

var yamlLine = regexp.MustCompile(`line (\d+)`)

func parseFrontmatter(raw []byte) (frontmatter, int, error) {
	fm := frontmatter{fields: map[string]any{}}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return fm, errorLine(err), err
	}
	if doc.Kind == 0 {
		return fm, 0, nil
	}
	root := doc.Content[0]
	if root.Kind == yaml.ScalarNode && root.Tag == "!!null" {
		return fm, 0, nil
	}
	if root.Kind != yaml.MappingNode {
		return fm, root.Line + 1, fmt.Errorf("frontmatter must be a mapping")
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		key := root.Content[i].Value
		var value any
		if err := root.Content[i+1].Decode(&value); err != nil {
			return fm, root.Content[i+1].Line + 1, fmt.Errorf("decode field %q: %w", key, err)
		}
		if _, dup := fm.fields[key]; dup {
			fm.dups = append(fm.dups, key)
		} else {
			fm.keys = append(fm.keys, key)
		}
		fm.fields[key] = value
	}
	return fm, 0, nil
}

func errorLine(err error) int {
	m := yamlLine.FindStringSubmatch(err.Error())
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n + 1
}

func stringField(fm frontmatter, name string, diags *[]Diagnostic) string {
	v, ok := fm.fields[name]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case int, int64, uint64, float64, bool:
		return fmt.Sprint(t)
	}
	*diags = append(*diags, diag(CodeUnexpectedFieldType, fmt.Sprintf("field %q must be a string", name), 0, name))
	return ""
}

func duplicateKeyDiags(fm frontmatter, diags *[]Diagnostic) {
	for _, k := range fm.dups {
		*diags = append(*diags, diag(CodeDuplicateKey, fmt.Sprintf("frontmatter key %q is repeated, the last value wins", k), 0, k))
	}
}

// SplitFrontmatter separates the YAML frontmatter from the body of a node or event file.
// It reports false when there is no complete frontmatter block, and then the body is the
// whole normalized source.
func SplitFrontmatter(src []byte) ([]byte, []byte, bool) {
	sp := splitFrontmatter(normalize(src))
	return sp.yaml, sp.body, sp.present
}
