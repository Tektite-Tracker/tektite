package service

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tektite-tracker/tektite/internal/node"
	"github.com/tektite-tracker/tektite/internal/store"
)

var (
	fieldKey   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)
	coreFields = map[string]bool{"id": true, "kind": true, "title": true, "parent": true}
)

type CreateInput struct {
	Kind   string
	Title  string
	Parent string
	Body   *string
	Edit   func(initial string) (string, error)
	Sets   []string
}

type Created struct {
	ID              string
	Kind            string
	MissingRequired []string
	Parent          string
	Path            string
	Ref             string
	Title           string
}

type setField struct {
	key   string
	value any
}

func parseSets(sets []string) ([]setField, error) {
	out := []setField{}
	for _, s := range sets {
		key, raw, ok := strings.Cut(s, "=")
		key = strings.TrimSpace(key)
		if !ok || !fieldKey.MatchString(key) {
			return nil, usageError("--set expects key=value, got %q", s)
		}
		if coreFields[key] {
			return nil, usageError("--set cannot change %q", key)
		}
		var v any
		if err := yaml.Unmarshal([]byte(raw), &v); err != nil || v == nil {
			v = raw
		}
		out = append(out, setField{key: key, value: v})
	}
	return out, nil
}

func (s *Service) Create(in CreateInput) (*Created, error) {
	kind, title := strings.TrimSpace(in.Kind), strings.TrimSpace(in.Title)
	if kind == "" || title == "" {
		return nil, usageError("kind and title must not be empty")
	}
	extra, err := parseSets(in.Sets)
	if err != nil {
		return nil, err
	}
	snap, err := s.load()
	if err != nil {
		return nil, err
	}
	schemas, err := s.Repo.LoadSchemas()
	if err != nil {
		return nil, fmt.Errorf("load schemas: %w", err)
	}
	parentRef := ""
	if strings.TrimSpace(in.Parent) != "" {
		p, err := snap.Resolve(in.Parent)
		if err != nil {
			return nil, fmt.Errorf("parent: %w", err)
		}
		parentRef = p.Ref
	}
	schema, hasSchema := schemas.For(kind)
	text := ""
	switch {
	case in.Body != nil:
		text = *in.Body
	case hasSchema:
		text = schema.Template
	}
	if in.Body == nil && in.Edit != nil {
		if text, err = in.Edit(text); err != nil {
			return nil, err
		}
	}

	fields := []node.Field{}
	present := map[string]bool{}
	for _, f := range extra {
		fields = append(fields, node.Field{Key: f.key, Value: f.value})
		present[f.key] = true
	}
	text = strings.TrimSpace(text)
	if text != "" {
		text += "\n"
	}
	created, err := s.Repo.CreateNode(snap, s.Now(), s.Rand, store.NewNode{Kind: kind, Title: title, Parent: parentRef, Extra: fields, Body: text})
	if err != nil {
		return nil, fmt.Errorf("new %s: %w", kind, err)
	}
	missing := []string{}
	if hasSchema {
		for _, f := range schema.Required {
			if !present[f] && !coreFields[f] {
				missing = append(missing, f)
			}
		}
	}
	return &Created{ID: created.ID, Kind: kind, MissingRequired: missing, Parent: parentRef, Path: created.Path, Ref: created.Ref, Title: title}, nil
}
