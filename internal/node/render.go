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

var coreKeys = []string{"id", "kind", "title", "parent"}

func Reformat(src []byte, core []Field, body string) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return nil, fmt.Errorf("parse frontmatter: %w", err)
	}
	type pair struct{ key, value *yaml.Node }
	var order []string
	pairs := map[string]*pair{}
	if len(doc.Content) > 0 && doc.Content[0].Kind == yaml.MappingNode {
		m := doc.Content[0]
		for i := 0; i+1 < len(m.Content); i += 2 {
			k := m.Content[i].Value
			if p, ok := pairs[k]; ok {
				p.value = m.Content[i+1]
				continue
			}
			pairs[k] = &pair{key: m.Content[i], value: m.Content[i+1]}
			order = append(order, k)
		}
	}
	out := &yaml.Node{Kind: yaml.MappingNode}
	for _, f := range core {
		if p, ok := pairs[f.Key]; ok && p.value.Kind == yaml.ScalarNode && p.value.Value != "" && p.value.Tag != "!!null" {
			out.Content = append(out.Content, p.key, p.value)
			continue
		}
		v := &yaml.Node{}
		if err := v.Encode(f.Value); err != nil {
			return nil, fmt.Errorf("encode field %q: %w", f.Key, err)
		}
		out.Content = append(out.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: f.Key}, v)
	}
	isCore := map[string]bool{}
	for _, k := range coreKeys {
		isCore[k] = true
	}
	for _, k := range order {
		if !isCore[k] {
			out.Content = append(out.Content, pairs[k].key, pairs[k].value)
		}
	}
	var buf bytes.Buffer
	buf.WriteString("---\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(out); err != nil {
		return nil, fmt.Errorf("encode frontmatter: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("encode frontmatter: %w", err)
	}
	buf.WriteString("---\n")
	buf.WriteString(body)
	return buf.Bytes(), nil
}
