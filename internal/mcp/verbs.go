package mcp

import (
	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/internal/service"
	"github.com/tektite-tracker/tektite/internal/view"
	"github.com/tektite-tracker/tektite/parse"
)

var (
	refParam      = param{"ref", "string", "node reference: slug-hash, a hash suffix of at least 3 characters, or a full id", true}
	actorParam    = param{"actor", "string", "who is acting, such as agent:claude-opus/s-4f2a; defaults to TKT_ACTOR", false}
	revisionParam = param{"revision", "integer", "plan revision the work is built against", false}
)

type refArgs struct {
	Ref string `json:"ref"`
}

type refActorArgs struct {
	Ref   string `json:"ref"`
	Actor string `json:"actor"`
}

type linkArgs struct {
	Ref    string `json:"ref"`
	Rel    string `json:"rel"`
	Target string `json:"target"`
	Actor  string `json:"actor"`
}

func (s *Server) buildTools() []tool {
	tools := s.readTools()
	tools = append(tools, s.writeTools()...)
	tools = append(tools, s.leaseTools()...)
	return append(tools, s.evidenceTools()...)
}

func (s *Server) readTools() []tool {
	type contextArgs struct {
		Ref    string `json:"ref"`
		Budget int    `json:"budget"`
	}
	type checkArgs struct {
		Stale bool `json:"stale"`
	}
	return []tool{
		{"tektite_show", "Show a node with its state, links, evidence, notes and history.", []param{refParam}, bind(func(svc *service.Service, in refArgs) (any, error) {
			return svc.Show(in.Ref)
		})},
		{"tektite_ls", "List nodes, optionally filtered by kind, status or subtree.", []param{
			{"kind", "string", "only nodes of this kind", false},
			{"status", "string", "only nodes with this status", false},
			{"under", "string", "only descendants of this node", false},
			{"all", "boolean", "include superseded nodes", false},
		}, bind(func(svc *service.Service, in struct {
			Kind   string `json:"kind"`
			Status string `json:"status"`
			Under  string `json:"under"`
			All    bool   `json:"all"`
		},
		) (any, error) {
			return svc.List(service.ListInput{Kind: in.Kind, Status: in.Status, Under: in.Under, All: in.All})
		})},
		{"tektite_next", "Return the best nodes to work on next, optionally claiming the first one.", []param{
			{"kind", "string", "only nodes of this kind, empty for any, ticket by default", false},
			actorParam,
			{"limit", "integer", "how many nodes to return, 1 by default", false},
			{"claim", "boolean", "claim the first node in the same call", false},
			{"ttl", "string", "lease duration with claim, such as 30m", false},
		}, bind(s.next)},
		{"tektite_context", "Build a prompt-ready context bundle for a node within a token budget.", []param{refParam, {"budget", "integer", "token budget, defaults to the repository config", false}}, bind(func(svc *service.Service, in contextArgs) (any, error) {
			b, err := svc.Context(in.Ref, in.Budget)
			if err != nil {
				return nil, err
			}
			return b.Markdown(), nil
		})},
		{"tektite_check", "Report problems in the repository.", []param{{"stale", "boolean", "also report evidence that later commits may have outdated", false}}, bind(func(svc *service.Service, in checkArgs) (any, error) {
			return svc.Check(in.Stale)
		})},
		{"tektite_policy_explain", "List the policy gates that apply to a node and whether each is met.", []param{refParam}, bind(func(svc *service.Service, in refArgs) (any, error) {
			return svc.Explain(in.Ref)
		})},
	}
}

type nextResult struct {
	Items []view.NextItem   `json:"items"`
	Claim *service.Appended `json:"claim"`
}

func (s *Server) next(svc *service.Service, in struct {
	Kind  *string `json:"kind"`
	Actor string  `json:"actor"`
	Limit int     `json:"limit"`
	Claim bool    `json:"claim"`
	TTL   string  `json:"ttl"`
},
) (any, error) {
	kind := parse.KindTicket
	if in.Kind != nil {
		kind = *in.Kind
	}
	limit := max(in.Limit, 1)
	ttl := event.DefaultTTL
	if in.TTL != "" {
		d, ok := event.ParseTTL(in.TTL)
		if !ok {
			return nil, &service.Error{Kind: service.KindUsage, Msg: "ttl must be a positive duration such as 30m"}
		}
		ttl = d
	}
	list, _, err := svc.Next(kind, in.Actor)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, &service.Error{Kind: service.KindNotFound, Msg: "nothing is available", Hint: "create a ticket with `tektite new ticket` or release a claim"}
	}
	list = list[:min(limit, len(list))]
	out := nextResult{Items: []view.NextItem{}}
	if in.Claim {
		claimed, err := svc.Claim(service.ClaimInput{Ref: list[0].Ref, Actor: in.Actor, TTL: ttl})
		if err != nil {
			return nil, err
		}
		out.Claim = claimed
	}
	for _, nf := range list {
		out.Items = append(out.Items, view.NewNextItem(nf))
	}
	return out, nil
}

func (s *Server) writeTools() []tool {
	return []tool{
		{"tektite_new", "Create a node.", []param{
			{"kind", "string", "node kind such as ticket, plan or gap", true},
			{"title", "string", "node title", true},
			{"parent", "string", "reference of the parent node", false},
			{"body", "string", "body text", false},
			{"set", "array", "extra frontmatter fields as key=value strings", false},
		}, bind(func(svc *service.Service, in struct {
			Kind   string   `json:"kind"`
			Title  string   `json:"title"`
			Parent string   `json:"parent"`
			Body   *string  `json:"body"`
			Set    []string `json:"set"`
		},
		) (any, error) {
			return svc.Create(service.CreateInput{Kind: in.Kind, Title: in.Title, Parent: in.Parent, Body: in.Body, Sets: in.Set})
		})},
		{"tektite_set_status", "Set the status of a node. Done may be gated by policy.", []param{
			refParam,
			{"status", "string", "open, active, blocked, review, done or dropped", true},
			actorParam,
			{"override", "string", "reason for overriding a gate, humans only", false},
			revisionParam,
		}, bind(func(svc *service.Service, in struct {
			Ref      string `json:"ref"`
			Status   string `json:"status"`
			Actor    string `json:"actor"`
			Override string `json:"override"`
			Revision int    `json:"revision"`
		},
		) (any, error) {
			return svc.SetStatus(service.SetStatusInput{Ref: in.Ref, Status: in.Status, Actor: in.Actor, Override: in.Override, Revision: in.Revision})
		})},
		{"tektite_note", "Add a note to a node.", []param{refParam, {"text", "string", "the note", true}, actorParam}, bind(func(svc *service.Service, in struct {
			Ref   string `json:"ref"`
			Text  string `json:"text"`
			Actor string `json:"actor"`
		},
		) (any, error) {
			return svc.Note(service.NoteInput{Ref: in.Ref, Text: in.Text, Actor: in.Actor})
		})},
		{"tektite_link", "Add a typed link to a node.", linkParams(), bind(func(svc *service.Service, in linkArgs) (any, error) {
			return svc.Link(service.LinkInput{Verb: parse.EventLink, Ref: in.Ref, Rel: in.Rel, Target: in.Target, Actor: in.Actor})
		})},
		{"tektite_unlink", "Remove a typed link from a node.", linkParams(), bind(func(svc *service.Service, in linkArgs) (any, error) {
			return svc.Link(service.LinkInput{Verb: parse.EventUnlink, Ref: in.Ref, Rel: in.Rel, Target: in.Target, Actor: in.Actor})
		})},
		{"tektite_approve", "Approve the current revision of a plan. Only a human actor can.", []param{refParam, actorParam, revisionParam}, bind(func(svc *service.Service, in struct {
			Ref      string `json:"ref"`
			Actor    string `json:"actor"`
			Revision int    `json:"revision"`
		},
		) (any, error) {
			return svc.Approve(service.ApproveInput{Ref: in.Ref, Actor: in.Actor, Revision: in.Revision})
		})},
		{"tektite_revise", "Record a new revision of a plan with a full body snapshot.", []param{refParam, {"body", "string", "the new full body", true}, actorParam}, bind(func(svc *service.Service, in struct {
			Ref   string `json:"ref"`
			Body  string `json:"body"`
			Actor string `json:"actor"`
		},
		) (any, error) {
			return svc.Revise(service.ReviseInput{Ref: in.Ref, Body: in.Body, Actor: in.Actor})
		})},
		{"tektite_close_gap", "Close a gap block of a node with passing evidence.", []param{
			{"ref", "string", "node reference followed by # and the block id, such as auth-retry-3fa#01HZ3KB2", true},
			{"evidence", "string", "evidence node that proves the gap is closed", true},
			actorParam,
		}, bind(func(svc *service.Service, in struct {
			Ref      string `json:"ref"`
			Evidence string `json:"evidence"`
			Actor    string `json:"actor"`
		},
		) (any, error) {
			return svc.CloseGap(service.CloseGapInput{Ref: in.Ref, Evidence: in.Evidence, Actor: in.Actor})
		})},
	}
}

func linkParams() []param {
	return []param{refParam, {"rel", "string", "link type such as implements, blocks, evidences, supersedes or derived-from", true}, {"target", "string", "reference of the target node", true}, actorParam}
}

func (s *Server) leaseTools() []tool {
	lease := func(verb, desc string) tool {
		return tool{"tektite_" + verb, desc, []param{refParam, actorParam}, bind(func(svc *service.Service, in refActorArgs) (any, error) {
			return svc.Lease(service.LeaseInput{Verb: verb, Ref: in.Ref, Actor: in.Actor})
		})}
	}
	return []tool{
		{"tektite_claim", "Claim a node with an expiring lease.", []param{refParam, actorParam, {"ttl", "string", "lease duration such as 30m", false}, revisionParam}, bind(func(svc *service.Service, in struct {
			Ref      string `json:"ref"`
			Actor    string `json:"actor"`
			TTL      string `json:"ttl"`
			Revision int    `json:"revision"`
		},
		) (any, error) {
			ttl := event.DefaultTTL
			if in.TTL != "" {
				d, ok := event.ParseTTL(in.TTL)
				if !ok {
					return nil, &service.Error{Kind: service.KindUsage, Msg: "ttl must be a positive duration such as 30m"}
				}
				ttl = d
			}
			return svc.Claim(service.ClaimInput{Ref: in.Ref, Actor: in.Actor, TTL: ttl, Revision: in.Revision})
		})},
		lease(parse.EventHeartbeat, "Extend your lease on a node."),
		lease(parse.EventRelease, "Release your lease on a node."),
	}
}
