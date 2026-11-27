package mcp

import (
	"github.com/tektite-tracker/tektite/internal/service"
)

func (s *Server) runDisabled() error {
	return &service.Error{Kind: service.KindPolicy, Msg: "running commands is disabled", Hint: "start the server with `tkt mcp --allow-run` to enable it"}
}

func (s *Server) evidenceTools() []tool {
	return []tool{
		{"tkt_attach_evidence", "Attach text, or the output of a command, as evidence to a node.", []param{
			refParam,
			{"claim", "string", "what the evidence proves", true},
			{"text", "string", "the evidence text", false},
			{"run", "string", "a shell command to run and store the output of, needs the server to allow it", false},
			{"command", "string", "command that produced the text", false},
			{"exit_code", "integer", "exit code of that command", false},
			actorParam,
			{"allow_large", "boolean", "store a text above the size cap", false},
		}, bind(func(svc *service.Service, in struct {
			Ref        string `json:"ref"`
			Claim      string `json:"claim"`
			Text       string `json:"text"`
			Run        string `json:"run"`
			Command    string `json:"command"`
			ExitCode   *int   `json:"exit_code"`
			Actor      string `json:"actor"`
			AllowLarge bool   `json:"allow_large"`
		},
		) (any, error) {
			if in.Run != "" && !s.opts.AllowRun {
				return nil, s.runDisabled()
			}
			if in.Run == "" && in.Text == "" {
				return nil, &service.Error{Kind: service.KindUsage, Msg: "pass text or run"}
			}
			return svc.AttachEvidence(service.AttachInput{Ref: in.Ref, Data: []byte(in.Text), Ext: ".txt", Claim: in.Claim, Command: in.Command, ExitCode: in.ExitCode, Actor: in.Actor, AllowLarge: in.AllowLarge, Run: in.Run})
		})},
		{"tkt_rerun", "Run the recorded command of an evidence node again and record a superseding evidence node.", []param{refParam, actorParam}, bind(func(svc *service.Service, in refActorArgs) (any, error) {
			if !s.opts.AllowRun {
				return nil, s.runDisabled()
			}
			return svc.Rerun(service.RerunInput{Ref: in.Ref, Actor: in.Actor})
		})},
	}
}
