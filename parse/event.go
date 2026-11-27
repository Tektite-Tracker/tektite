package parse

import (
	"fmt"
	"io"
	"path"
	"strings"
	"time"
)

type Event struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Actor      string         `json:"actor"`
	At         time.Time      `json:"at"`
	Fields     map[string]any `json:"fields"`
	FieldOrder []string       `json:"field_order"`
	Body       string         `json:"body"`
	Known      bool           `json:"known"`
	Valid      bool           `json:"valid"`
}

var EventRequiredFields = map[string][]string{
	"set-status": {"status"},
	"claim":      {"ttl"},
	"heartbeat":  nil,
	"release":    nil,
	"note":       nil,
	"link":       {"rel", "target"},
	"unlink":     {"rel", "target"},
	"revision":   {"number"},
	"attach":     {"evidence"},
	"close":      {"block"},
	"approve":    {"revision"},
}

var eventCore = map[string]bool{"type": true, "actor": true, "at": true}

func ParseEvent(name string, src []byte) (*Event, []Diagnostic) {
	src = normalize(src)
	diags := []Diagnostic{}
	e := &Event{
		ID:         strings.TrimSuffix(path.Base(name), path.Ext(name)),
		Fields:     map[string]any{},
		FieldOrder: []string{},
		Valid:      true,
	}
	invalid := func(d Diagnostic) {
		diags = append(diags, d)
		e.Valid = false
	}

	sp := splitFrontmatter(src)
	e.Body = strings.TrimSpace(string(sp.body))
	if sp.unterminat {
		invalid(diag(CodeUnterminatedFrontmtr, "frontmatter is not closed by a --- line", 1, ""))
		return e, diags
	}
	if !sp.present {
		invalid(diag(CodeMissingEventType, "event has no frontmatter", 0, "type"))
		return e, diags
	}
	fm, line, err := parseFrontmatter(sp.yaml)
	if err != nil {
		e.Body = strings.TrimSpace(string(src))
		invalid(diag(CodeMalformedFrontmatter, "invalid YAML in frontmatter: "+err.Error(), line, ""))
		return e, diags
	}

	duplicateKeyDiags(fm, &diags)
	if !IsULID(e.ID) {
		diags = append(diags, diag(CodeEventNameNotULID, fmt.Sprintf("event file name %q is not a ULID, ordering falls back to at", e.ID), 0, ""))
	}
	e.Type = stringField(fm, "type", &diags)
	e.Actor = stringField(fm, "actor", &diags)
	if e.Type == "" {
		invalid(diag(CodeMissingEventType, "event has no type", 0, "type"))
	}
	if e.Actor == "" {
		diags = append(diags, diag(CodeMissingActor, "event has no actor", 0, "actor"))
	}
	switch at := fm.fields["at"].(type) {
	case time.Time:
		e.At = at
	case string:
		t, err := time.Parse(time.RFC3339, at)
		if err != nil {
			invalid(diag(CodeInvalidEventTime, fmt.Sprintf("at %q is not an RFC 3339 time", at), 0, "at"))
		} else {
			e.At = t
		}
	default:
		invalid(diag(CodeInvalidEventTime, "event has no valid at time", 0, "at"))
	}

	for _, k := range fm.keys {
		if !eventCore[k] {
			e.Fields[k] = fm.fields[k]
			e.FieldOrder = append(e.FieldOrder, k)
		}
	}

	required, known := EventRequiredFields[e.Type]
	e.Known = known
	if e.Type != "" && !known {
		diags = append(diags, diag(CodeUnknownEventType, fmt.Sprintf("unknown event type %q is kept in history", e.Type), 0, "type"))
	}
	for _, f := range required {
		if v, ok := e.Fields[f]; !ok || v == nil || v == "" {
			invalid(diag(CodeMissingRequiredField, fmt.Sprintf("%s event is missing required field %q", e.Type, f), 0, f))
		}
	}
	return e, diags
}

func ParseEventReader(name string, r io.Reader) (*Event, []Diagnostic, error) {
	src, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, fmt.Errorf("read source: %w", err)
	}
	e, d := ParseEvent(name, src)
	return e, d, nil
}
