package node

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

type Field struct {
	Key   string
	Value any
}

func Render(fields []Field, body string) ([]byte, error) {
	m := &yaml.Node{Kind: yaml.MappingNode}
	for _, f := range fields {
		k := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: f.Key}
		v := &yaml.Node{}
		if err := v.Encode(f.Value); err != nil {
			return nil, fmt.Errorf("encode field %q: %w", f.Key, err)
		}
		m.Content = append(m.Content, k, v)
	}
	var buf bytes.Buffer
	buf.WriteString("---\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if len(fields) > 0 {
		if err := enc.Encode(m); err != nil {
			return nil, fmt.Errorf("encode frontmatter: %w", err)
		}
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("encode frontmatter: %w", err)
	}
	buf.WriteString("---\n")
	buf.WriteString(body)
	return buf.Bytes(), nil
}
