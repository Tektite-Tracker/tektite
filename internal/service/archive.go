package service

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/internal/store"
	"github.com/tektite-tracker/tektite/parse"
)

type ArchiveInput struct {
	Refs      []string
	Done      bool
	OlderThan string
	DryRun    bool
}

type ArchivedNode struct {
	Ref    string   `json:"ref"`
	From   string   `json:"from"`
	To     string   `json:"to"`
	Events int      `json:"events"`
	Blobs  []string `json:"blobs"`
}

type SkippedNode struct {
	Ref    string `json:"ref"`
	Reason string `json:"reason"`
}

type ArchiveResult struct {
	Archived []ArchivedNode `json:"archived"`
	Skipped  []SkippedNode  `json:"skipped"`
	DryRun   bool           `json:"dry_run"`
}

type UnarchiveResult struct {
	Unarchived []ArchivedNode `json:"unarchived"`
	DryRun     bool           `json:"dry_run"`
}

var ageUnit = regexp.MustCompile(`^(\d+)([dw])$`)

func ParseAge(v string) (time.Duration, error) {
	v = strings.TrimSpace(v)
	if m := ageUnit.FindStringSubmatch(v); m != nil {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q", v)
		}
		day := 24 * time.Hour
		if m[2] == "w" {
			day *= 7
		}
		return time.Duration(n) * day, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("invalid duration %q", v)
	}
	return d, nil
}

func lastEventAt(snap *store.Snapshot, ref string) time.Time {
	var last time.Time
	for _, ev := range snap.EventsOf(ref) {
		if ev != nil && ev.Valid && ev.At.After(last) {
			last = ev.At
		}
	}
	return last
}

func (s *Service) Archive(in ArchiveInput) (*ArchiveResult, error) {
	switch {
	case len(in.Refs) > 0 && in.Done:
		return nil, usageError("pass references or --done, not both")
	case len(in.Refs) == 0 && !in.Done:
		return nil, usageError("nothing to archive, pass references or --done")
	case in.OlderThan != "" && !in.Done:
		return nil, usageError("--older-than needs --done")
	}
	var age time.Duration
	if in.OlderThan != "" {
		d, err := ParseAge(in.OlderThan)
		if err != nil {
			return nil, usageError("--older-than: %s", err)
		}
		age = d
	}
	snap, err := s.load()
	if err != nil {
		return nil, err
	}
	graph := snap.BuildGraph()
	res := &ArchiveResult{Archived: []ArchivedNode{}, Skipped: []SkippedNode{}, DryRun: in.DryRun}
	var picked []*store.NodeFile
	if in.Done {
		cutoff := s.Now().Add(-age)
		for _, nf := range snap.Nodes {
			if nf.Node.Kind == parse.KindEvidence || !event.Closed(graph.States[nf.Ref].Status) {
				continue
			}
			if in.OlderThan != "" && lastEventAt(snap, nf.Ref).After(cutoff) {
				continue
			}
			picked = append(picked, nf)
		}
	} else {
		seen := map[string]bool{}
		for _, q := range in.Refs {
			nf, err := snap.Resolve(q)
			if err != nil {
				return nil, fmt.Errorf("archive: %w", err)
			}
			if !seen[nf.Ref] {
				seen[nf.Ref] = true
				picked = append(picked, nf)
			}
		}
	}
	var eligible []*store.NodeFile
	var refusals []string
	for _, nf := range picked {
		if reason := archiveRefusal(snap, graph, nf); reason != "" {
			res.Skipped = append(res.Skipped, SkippedNode{Ref: nf.Ref, Reason: reason})
			refusals = append(refusals, nf.Ref+" "+reason)
			continue
		}
		eligible = append(eligible, nf)
	}
	if !in.Done && len(refusals) > 0 {
		return nil, newError(KindConflict, "only done or dropped nodes that nothing open depends on can be archived", "cannot archive: %s", strings.Join(refusals, "; "))
	}
	moves, nodes := archiveMoves(snap, eligible)
	for i, m := range moves {
		nodes[i].Blobs = append([]string{}, m.Blobs...)
	}
	res.Archived = nodes
	if in.DryRun || len(moves) == 0 {
		return res, nil
	}
	if err := s.Repo.Archive(moves); err != nil {
		return nil, fmt.Errorf("archive: %w", err)
	}
	return res, nil
}

func archiveRefusal(snap *store.Snapshot, g *store.Graph, nf *store.NodeFile) string {
	if nf.Archived {
		return "is already archived"
	}
	if st := g.States[nf.Ref].Status; !event.Closed(st) {
		return fmt.Sprintf("is %s, only done or dropped nodes can be archived", st)
	}
	for _, kid := range snap.Tree.Children[nf.Ref] {
		k, ok := snap.Get(kid)
		if !ok || k.Archived || k.Node.Kind == parse.KindEvidence {
			continue
		}
		if !event.Closed(g.States[kid].Status) {
			return "has an open child " + kid
		}
	}
	blocked := []string{}
	for ref, blockers := range g.Blockers {
		if slices.Contains(blockers, nf.Ref) && !event.Closed(g.States[ref].Status) {
			blocked = append(blocked, ref)
		}
	}
	if len(blocked) > 0 {
		slices.Sort(blocked)
		return "blocks the open node " + strings.Join(blocked, ", ")
	}
	return ""
}

func evidenceChildren(snap *store.Snapshot, ref string) []*store.NodeFile {
	var out []*store.NodeFile
	for _, kid := range snap.Tree.Children[ref] {
		if k, ok := snap.Get(kid); ok && k.Node.Kind == parse.KindEvidence {
			out = append(out, k)
		}
	}
	return out
}

func archiveMoves(snap *store.Snapshot, roots []*store.NodeFile) ([]store.Move, []ArchivedNode) {
	var set []*store.NodeFile
	inSet := map[string]bool{}
	for _, nf := range roots {
		for _, n := range append([]*store.NodeFile{nf}, evidenceChildren(snap, nf.Ref)...) {
			if !inSet[n.Ref] {
				inSet[n.Ref] = true
				set = append(set, n)
			}
		}
	}
	slices.SortFunc(set, func(a, b *store.NodeFile) int { return strings.Compare(a.Ref, b.Ref) })
	users := snap.BlobRefs()
	moves := []store.Move{}
	nodes := []ArchivedNode{}
	for _, nf := range set {
		moves = append(moves, store.Move{Ref: nf.Ref, Blobs: movableBlobs(nf, users, inSet)})
		nodes = append(nodes, ArchivedNode{Ref: nf.Ref, From: nf.Path, Events: len(snap.Events[nf.Ref])})
	}
	return finishPaths(moves, nodes, true)
}

func movableBlobs(nf *store.NodeFile, users map[string][]string, inSet map[string]bool) []string {
	blobs := []string{}
	for _, att := range store.EvidenceOfNode(nf).Attachments {
		name := store.BlobName(att)
		if name == "" || slices.Contains(blobs, name) {
			continue
		}
		shared := false
		for _, u := range users[name] {
			if !inSet[u] {
				shared = true
			}
		}
		if !shared {
			blobs = append(blobs, name)
		}
	}
	return blobs
}

func finishPaths(moves []store.Move, nodes []ArchivedNode, archiving bool) ([]store.Move, []ArchivedNode) {
	for i := range nodes {
		to := ".tektite/archive/nodes/" + nodes[i].Ref + ".md"
		from := ".tektite/nodes/" + nodes[i].Ref + ".md"
		if !archiving {
			from, to = to, from
		}
		nodes[i].From, nodes[i].To = from, to
	}
	return moves, nodes
}

func (s *Service) Unarchive(refs []string, dryRun bool) (*UnarchiveResult, error) {
	if len(refs) == 0 {
		return nil, usageError("nothing to unarchive, pass references")
	}
	snap, err := s.load()
	if err != nil {
		return nil, err
	}
	var set []*store.NodeFile
	inSet := map[string]bool{}
	var refusals []string
	for _, q := range refs {
		nf, err := snap.Resolve(q)
		if err != nil {
			return nil, fmt.Errorf("unarchive: %w", err)
		}
		if inSet[nf.Ref] {
			continue
		}
		inSet[nf.Ref] = true
		if !nf.Archived {
			refusals = append(refusals, nf.Ref+" is not archived")
			continue
		}
		set = append(set, nf)
		for _, ev := range evidenceChildren(snap, nf.Ref) {
			if ev.Archived && !inSet[ev.Ref] {
				inSet[ev.Ref] = true
				set = append(set, ev)
			}
		}
	}
	if len(refusals) > 0 {
		return nil, newError(KindConflict, "run `tektite ls --archived` to list archived nodes", "cannot unarchive: %s", strings.Join(refusals, "; "))
	}
	slices.SortFunc(set, func(a, b *store.NodeFile) int { return strings.Compare(a.Ref, b.Ref) })
	moves := []store.Move{}
	nodes := []ArchivedNode{}
	for _, nf := range set {
		blobs := []string{}
		for _, att := range store.EvidenceOfNode(nf).Attachments {
			if name := store.BlobName(att); name != "" && !slices.Contains(blobs, name) {
				blobs = append(blobs, name)
			}
		}
		moves = append(moves, store.Move{Ref: nf.Ref, Blobs: blobs})
		nodes = append(nodes, ArchivedNode{Ref: nf.Ref, Events: len(snap.Events[nf.Ref]), Blobs: blobs})
	}
	_, nodes = finishPaths(moves, nodes, false)
	res := &UnarchiveResult{Unarchived: nodes, DryRun: dryRun}
	if dryRun {
		return res, nil
	}
	if err := s.Repo.Unarchive(moves); err != nil {
		return nil, fmt.Errorf("unarchive: %w", err)
	}
	return res, nil
}
