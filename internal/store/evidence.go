package store

import (
	"cmp"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/tektite-tracker/tektite/parse"
)

type Evidence struct {
	Node        *NodeFile
	Claim       string
	Actor       string
	At          time.Time
	Commit      string
	Dirty       bool
	Command     string
	ExitCode    *int
	Attachments []string
}

func EvidenceOfNode(nf *NodeFile) Evidence {
	f := nf.Node.Fields
	ev := Evidence{Node: nf}
	if v, ok := f["claim"]; ok && v != nil {
		ev.Claim = parse.StringOf(v)
	}
	if atts, ok := f["attachments"].([]any); ok {
		for _, a := range atts {
			if s := parse.StringOf(a); s != "" {
				ev.Attachments = append(ev.Attachments, s)
			}
		}
	}
	prov, _ := f["provenance"].(map[string]any)
	str := func(k string) string {
		if v, ok := prov[k]; ok && v != nil {
			return parse.StringOf(v)
		}
		return ""
	}
	ev.Actor, ev.Commit, ev.Command = str("actor"), str("commit"), str("command")
	ev.Dirty, _ = prov["dirty"].(bool)
	switch at := prov["at"].(type) {
	case time.Time:
		ev.At = at
	case string:
		ev.At, _ = time.Parse(time.RFC3339, at)
	}
	if v, ok := prov["exit_code"]; ok && v != nil {
		if n, ok := parse.IntOf(v); ok {
			ev.ExitCode = &n
		}
	}
	return ev
}

func BlobName(attachment string) string {
	a := strings.TrimSpace(attachment)
	if i := strings.Index(a, "blobs/"); i >= 0 {
		return path.Base(a[i:])
	}
	return ""
}

func (s *Snapshot) EvidenceFor(g *Graph, ref string) []Evidence {
	seen := map[string]bool{}
	var out []Evidence
	add := func(nf *NodeFile) {
		if nf != nil && nf.Ref != ref && !seen[nf.Ref] {
			seen[nf.Ref] = true
			out = append(out, EvidenceOfNode(nf))
		}
	}
	for _, nf := range s.Nodes {
		if nf.Node.Kind != parse.KindEvidence {
			continue
		}
		if nf.ParentRef == ref {
			add(nf)
			continue
		}
		for _, l := range g.Out[nf.Ref] {
			if t, err := s.Resolve(l.Target); err == nil && t.Ref == ref && l.Rel == "evidences" {
				add(nf)
				break
			}
		}
	}
	for _, e := range s.EventsOf(ref) {
		if e.Valid && e.Type == parse.EventAttach {
			if t, err := s.Resolve(e.String("evidence")); err == nil {
				add(t)
			}
		}
	}
	slices.SortStableFunc(out, func(a, b Evidence) int {
		return cmp.Or(b.At.Compare(a.At), cmp.Compare(a.Node.Ref, b.Node.Ref))
	})
	return out
}
