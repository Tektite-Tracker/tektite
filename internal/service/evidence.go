package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/tektite-tracker/tektite/internal/check"
	"github.com/tektite-tracker/tektite/internal/node"
	"github.com/tektite-tracker/tektite/internal/store"
	"github.com/tektite-tracker/tektite/parse"
)

const shortSHA = 7

type AttachInput struct {
	Ref        string
	Data       []byte
	Ext        string
	Claim      string
	Command    string
	ExitCode   *int
	Actor      string
	AllowLarge bool
	Run        string
	Supersedes string
}

type Attached struct {
	Evidence string             `json:"evidence"`
	Blob     string             `json:"blob"`
	Hash     string             `json:"hash"`
	Stored   bool               `json:"stored"`
	Parent   string             `json:"parent"`
	Event    string             `json:"event"`
	Commit   string             `json:"commit"`
	Warnings []parse.Diagnostic `json:"warnings"`
}

type blob struct {
	name   string
	hash   string
	stored bool
}

func (s *Service) AttachEvidence(in AttachInput) (*Attached, error) {
	snap, err := s.load()
	if err != nil {
		return nil, err
	}
	target, err := snap.Resolve(in.Ref)
	if err != nil {
		return nil, fmt.Errorf("attach-evidence: %w", err)
	}
	out, err := s.capture(&in)
	if err != nil {
		return nil, err
	}
	warnings, err := s.checkBlobSize(int64(len(in.Data)), in.AllowLarge)
	if err != nil {
		return nil, err
	}
	b, body, err := s.storeOutput(in, out)
	if err != nil {
		return nil, err
	}
	actor, actorWarn := s.ResolveActor(in.Actor)
	warnings = append(warnings, actorWarn...)
	at := s.Now()
	commit, prov, provWarn := s.provenance(in, actor, at)
	warnings = append(warnings, provWarn...)

	created, err := s.Repo.CreateNode(snap, at, s.Rand, store.NewNode{
		Kind:   parse.KindEvidence,
		Title:  in.Claim,
		Parent: target.Ref,
		Extra:  evidenceFields(in, target.Ref, b, prov),
		Body:   body,
	})
	if err != nil {
		return nil, fmt.Errorf("attach-evidence: %w", err)
	}
	ev, err := s.appendEvent(eventInput{typ: parse.EventAttach, query: target.Ref, actor: actor, prepare: func(context) (step, error) {
		return step{fields: []node.Field{{Key: "evidence", Value: created.Ref}}}, nil
	}})
	if err != nil {
		return nil, err
	}
	return &Attached{
		Evidence: created.Ref,
		Blob:     blobPath(b),
		Hash:     b.hash,
		Stored:   b.stored,
		Parent:   target.Ref,
		Event:    ev.Event,
		Commit:   commit,
		Warnings: warnings,
	}, nil
}

func (s *Service) checkBlobSize(size int64, allowLarge bool) ([]parse.Diagnostic, error) {
	cfg, err := s.Repo.Config()
	if err != nil {
		return nil, fmt.Errorf("attach-evidence: %w", err)
	}
	limit := cfg.BlobCapBytes()
	if size <= limit {
		return []parse.Diagnostic{}, nil
	}
	msg := fmt.Sprintf("the attachment is %.1f MB, over the %d MB cap, and every blob stays in the repository history", float64(size)/(1<<20), limit>>20)
	if !allowLarge {
		return nil, newError(KindUsage, "pass --allow-large to store it anyway, or raise blob_cap_mb in .tektite/config", "warning: %s", msg)
	}
	return []parse.Diagnostic{warning("", "%s", msg)}, nil
}

func blobPath(b blob) string {
	if b.name == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Join(".tektite", "blobs", b.name))
}

func evidenceFields(in AttachInput, target string, b blob, prov map[string]any) []node.Field {
	fields := []node.Field{
		{Key: "evidences", Value: []string{target}},
		{Key: "claim", Value: in.Claim},
	}
	if in.Supersedes != "" {
		fields = append(fields, node.Field{Key: "supersedes", Value: []string{in.Supersedes}})
	}
	if b.name != "" {
		fields = append(fields, node.Field{Key: "attachments", Value: []string{"blobs/" + b.name}})
	}
	return append(fields, node.Field{Key: "provenance", Value: prov})
}

func (s *Service) storeBlob(data []byte, ext string) (blob, error) {
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	name := hash + strings.ToLower(ext)
	_, stored, err := s.Repo.WriteBlob(name, data)
	if err != nil {
		return blob{}, fmt.Errorf("store blob: %w", err)
	}
	return blob{name: name, hash: hash, stored: stored}, nil
}

func (s *Service) provenance(in AttachInput, actor string, at time.Time) (string, map[string]any, []parse.Diagnostic) {
	warnings := []parse.Diagnostic{}
	prov := map[string]any{"actor": actor, "at": parse.FormatTime(at)}
	commit := ""
	if head := s.git().Head(); len(head) >= shortSHA {
		commit = head[:shortSHA]
	}
	if commit != "" {
		prov["commit"] = commit
		if s.git().Dirty() {
			prov["dirty"] = true
		}
	} else {
		warnings = append(warnings, warning(check.CodeEvidenceNoCommit, "no git commit is available, so the evidence cannot be tied to a code version"))
	}
	if in.Command != "" {
		prov["command"] = in.Command
	}
	if in.ExitCode != nil {
		prov["exit_code"] = *in.ExitCode
	}
	return commit, prov, warnings
}
