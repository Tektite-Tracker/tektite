package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/tektite-tracker/tektite/internal/service"
)

const (
	latestProtocol = "2025-06-18"
	codeParse      = -32700
	codeMethod     = -32601
	codeParams     = -32602
	codeRequest    = -32600
)

var supportedProtocols = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

type Options struct {
	Name     string
	Version  string
	Open     func() (*service.Service, error)
	AllowRun bool
}

type Server struct {
	opts  Options
	tools []tool
	index map[string]tool
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

func New(opts Options) *Server {
	s := &Server{opts: opts, index: map[string]tool{}}
	s.tools = s.buildTools()
	for _, t := range s.tools {
		s.index[t.name] = t
	}
	return s
}

func (s *Server) Serve(in io.Reader, out io.Writer) error {
	r := bufio.NewReader(in)
	for {
		line, err := r.ReadBytes('\n')
		if trimmed := bytes.TrimSpace(line); len(trimmed) > 0 {
			if reply := s.Handle(trimmed); reply != nil {
				if _, werr := out.Write(append(reply, '\n')); werr != nil {
					return fmt.Errorf("mcp: write: %w", werr)
				}
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("mcp: read: %w", err)
		}
	}
}

func (s *Server) Handle(line []byte) []byte {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return encode(response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{Code: codeParse, Message: "parse error: " + err.Error()}})
	}
	if req.Method == "" {
		return encode(response{JSONRPC: "2.0", ID: idOrNull(req.ID), Error: &rpcError{Code: codeRequest, Message: "invalid request: method is missing"}})
	}
	if len(req.ID) == 0 {
		return nil
	}
	res := response{JSONRPC: "2.0", ID: req.ID}
	result, rerr := s.dispatch(req)
	if rerr != nil {
		res.Error = rerr
	} else {
		res.Result = result
	}
	return encode(res)
}

func idOrNull(id json.RawMessage) json.RawMessage {
	if len(id) == 0 {
		return json.RawMessage("null")
	}
	return id
}

func encode(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		return []byte(`{"jsonrpc":"2.0","id":null,"error":{"code":-32603,"message":"internal error"}}`)
	}
	return data
}

func (s *Server) dispatch(req request) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		return s.initialize(req.Params), nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": s.toolList()}, nil
	case "tools/call":
		return s.call(req.Params)
	}
	return nil, &rpcError{Code: codeMethod, Message: fmt.Sprintf("method %q not found", req.Method)}
}

func (s *Server) initialize(params json.RawMessage) map[string]any {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(params, &p)
	version := latestProtocol
	for _, v := range supportedProtocols {
		if v == p.ProtocolVersion {
			version = v
		}
	}
	return map[string]any{
		"protocolVersion": version,
		"capabilities":    map[string]any{"tools": map[string]any{}},
		"serverInfo":      map[string]any{"name": s.opts.Name, "version": s.opts.Version},
	}
}
