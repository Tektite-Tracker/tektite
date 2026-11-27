package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tektite-tracker/tektite/internal/store"
	"github.com/tektite-tracker/tektite/parse"
)

const (
	inlineLimit = 4096
	tailLines   = 20
)

type Executor func(command, dir string) ([]byte, int, error)

func ShellExecutor(command, dir string) ([]byte, int, error) {
	c := exec.Command("sh", "-c", command)
	c.Dir = dir
	out, err := c.CombinedOutput()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return out, 0, nil
	case errors.As(err, &exit):
		return out, exit.ExitCode(), nil
	}
	return nil, 0, fmt.Errorf("run %q: %w", command, err)
}

func (s *Service) execute(command string) ([]byte, int, error) {
	run := s.Exec
	if run == nil {
		run = ShellExecutor
	}
	return run(command, filepath.Dir(s.Repo.Dir()))
}

func (s *Service) capture(in *AttachInput) ([]byte, error) {
	if in.Run == "" {
		if len(in.Data) == 0 {
			return nil, usageError("the evidence is empty")
		}
		return in.Data, nil
	}
	if len(in.Data) > 0 || in.Command != "" {
		return nil, usageError("--run cannot be combined with --file or --command")
	}
	out, code, err := s.execute(in.Run)
	if err != nil {
		return nil, err
	}
	in.Command, in.ExitCode, in.Data = in.Run, &code, out
	return out, nil
}

func (s *Service) storeOutput(in AttachInput, out []byte) (blob, string, error) {
	if in.Run == "" {
		b, err := s.storeBlob(out, in.Ext)
		return b, "", err
	}
	if len(out) <= inlineLimit {
		sum := sha256.Sum256(out)
		return blob{hash: hex.EncodeToString(sum[:])}, fenced(string(out)), nil
	}
	b, err := s.storeBlob(out, ".txt")
	if err != nil {
		return blob{}, "", err
	}
	body := fmt.Sprintf("The output is %d bytes, so the full text is stored as `blobs/%s`. The last %d lines:\n\n%s", len(out), b.name, tailLines, fenced(lastLines(string(out), tailLines)))
	return b, body, nil
}

func lastLines(text string, n int) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func fenced(text string) string {
	fence := "```"
	for strings.Contains(text, fence) {
		fence += "`"
	}
	return fence + "\n" + strings.TrimRight(text, "\n") + "\n" + fence + "\n"
}

type RerunInput struct {
	Ref   string
	Actor string
}

func (s *Service) Rerun(in RerunInput) (*Attached, error) {
	snap, err := s.load()
	if err != nil {
		return nil, err
	}
	nf, err := snap.Resolve(in.Ref)
	if err != nil {
		return nil, fmt.Errorf("rerun: %w", err)
	}
	if nf.Node.Kind != parse.KindEvidence {
		return nil, usageError("%s is a %s, not an evidence node", nf.Ref, nf.Node.Kind)
	}
	ev := store.EvidenceOfNode(nf)
	if ev.Command == "" {
		return nil, usageError("%s has no recorded command, so it cannot be re-run", nf.Ref)
	}
	target := nf.ParentOrRaw()
	if list, ok := nf.Node.Fields["evidences"].([]any); ok && len(list) > 0 {
		target = parse.StringOf(list[0])
	}
	return s.AttachEvidence(AttachInput{Ref: target, Run: ev.Command, Claim: ev.Claim, Actor: in.Actor, Supersedes: nf.Ref})
}
