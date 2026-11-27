package service

import (
	"fmt"
	"strings"

	"github.com/tektite-tracker/tektite/internal/node"
	"github.com/tektite-tracker/tektite/internal/store"
	"github.com/tektite-tracker/tektite/parse"
)

type CloseGapInput struct {
	Ref      string
	Evidence string
	Actor    string
}

func (s *Service) CloseGap(in CloseGapInput) (*Appended, error) {
	query, blockID, ok := strings.Cut(in.Ref, "#")
	blockID = strings.TrimSpace(blockID)
	if !ok || blockID == "" {
		return nil, usageError("expected <ref>#<block-id>, got %q", in.Ref)
	}
	if strings.TrimSpace(in.Evidence) == "" {
		return nil, usageError("evidence must not be empty")
	}
	return s.appendEvent(eventInput{typ: parse.EventClose, query: query, actorFlag: in.Actor, prepare: func(c context) (step, error) {
		id, err := openGapID(c, blockID)
		if err != nil {
			return step{}, err
		}
		ev, err := c.snap.Resolve(in.Evidence)
		if err != nil {
			return step{}, fmt.Errorf("evidence: %w", err)
		}
		if err := checkEvidenceCloses(ev, c.nf.Ref); err != nil {
			return step{}, err
		}
		return step{fields: []node.Field{{Key: "block", Value: id}, {Key: "evidence", Value: ev.Ref}}}, nil
	}})
}

func openGapID(c context, blockID string) (string, error) {
	closed := c.snap.ClosedBlocks(c.nf.Ref)
	for _, b := range c.nf.Node.Blocks {
		if b.ID == "" || !strings.EqualFold(b.ID, blockID) {
			continue
		}
		switch {
		case !b.Gap:
			return "", usageError("block %s of %s is not a gap", b.ID, c.nf.Ref)
		case b.Checked:
			return "", usageError("gap %s of %s is already checked off", b.ID, c.nf.Ref)
		}
		if _, done := closed[strings.ToLower(b.ID)]; done {
			return "", usageError("gap %s of %s is already closed", b.ID, c.nf.Ref)
		}
		return b.ID, nil
	}
	return "", usageError("%s has no block %q", c.nf.Ref, blockID)
}

func checkEvidenceCloses(ev *store.NodeFile, target string) error {
	if ev.Node.Kind != parse.KindEvidence {
		return usageError("%s is a %s, not an evidence node", ev.Ref, ev.Node.Kind)
	}
	targets, _ := ev.Node.Fields["evidences"].([]any)
	aims := ev.ParentOrRaw() == target
	for _, t := range targets {
		aims = aims || parse.StringOf(t) == target
	}
	if !aims {
		return usageError("%s does not target %s", ev.Ref, target)
	}
	if e := store.EvidenceOfNode(ev); e.ExitCode != nil && *e.ExitCode != 0 {
		return usageError("%s has exit code %d, so it cannot close a gap", ev.Ref, *e.ExitCode)
	}
	return nil
}
