package mcp

import (
	"encoding/json"

	"github.com/tektite-tracker/tektite/internal/service"
)

type param struct {
	name     string
	typ      string
	desc     string
	required bool
}

type tool struct {
	name   string
	desc   string
	params []param
	run    func(svc *service.Service, args json.RawMessage) (any, error)
}

func (t tool) schema() map[string]any {
	props := map[string]any{}
	required := []string{}
	for _, p := range t.params {
		prop := map[string]any{"type": p.typ, "description": p.desc}
		if p.typ == "array" {
			prop["items"] = map[string]any{"type": "string"}
		}
		props[p.name] = prop
		if p.required {
			required = append(required, p.name)
		}
	}
	schema := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func (s *Server) toolList() []map[string]any {
	out := make([]map[string]any, 0, len(s.tools))
	for _, t := range s.tools {
		out = append(out, map[string]any{"name": t.name, "description": t.desc, "inputSchema": t.schema()})
	}
	return out
}

type toolResult struct {
	Content []map[string]string `json:"content"`
	IsError bool                `json:"isError"`
}

func textResult(text string, isError bool) toolResult {
	return toolResult{Content: []map[string]string{{"type": "text", "text": text}}, IsError: isError}
}

type failure struct {
	Error string               `json:"error"`
	Kind  string               `json:"kind"`
	Hint  string               `json:"hint,omitempty"`
	Gates []service.GateResult `json:"gates,omitempty"`
}

func failureResult(err error) toolResult {
	se := service.Classify(err)
	data, _ := json.MarshalIndent(failure{Error: se.Msg, Kind: se.Kind.String(), Hint: se.Hint, Gates: se.Gates}, "", "  ")
	return textResult(string(data), true)
}

func (s *Server) call(params json.RawMessage) (any, *rpcError) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &rpcError{Code: codeParams, Message: "invalid params: " + err.Error()}
	}
	t, ok := s.index[p.Name]
	if !ok {
		return nil, &rpcError{Code: codeParams, Message: "unknown tool " + p.Name}
	}
	if len(p.Arguments) == 0 {
		p.Arguments = json.RawMessage("{}")
	}
	svc, err := s.opts.Open()
	if err != nil {
		return failureResult(err), nil
	}
	out, err := t.run(svc, p.Arguments)
	if err != nil {
		return failureResult(err), nil
	}
	if text, ok := out.(string); ok {
		return textResult(text, false), nil
	}
	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return failureResult(err), nil
	}
	return textResult(string(data), false), nil
}

func bind[T any](fn func(svc *service.Service, in T) (any, error)) func(*service.Service, json.RawMessage) (any, error) {
	return func(svc *service.Service, raw json.RawMessage) (any, error) {
		var in T
		if err := json.Unmarshal(raw, &in); err != nil {
			return nil, &service.Error{Kind: service.KindUsage, Msg: "invalid arguments: " + err.Error()}
		}
		return fn(svc, in)
	}
}
