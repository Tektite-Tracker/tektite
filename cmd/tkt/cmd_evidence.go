package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/check"
	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/internal/node"
	"github.com/tektite-tracker/tektite/parse"
)

const shortSHA = 7

func gitDirty(dir string) func() bool {
	return func() bool {
		c := exec.Command("git", "status", "--porcelain", "--", ".", ":(exclude).tektite")
		c.Dir = dir
		out, err := c.Output()
		return err == nil && strings.TrimSpace(string(out)) != ""
	}
}

type attachResult struct {
	Evidence string   `json:"evidence"`
	Blob     string   `json:"blob"`
	Hash     string   `json:"hash"`
	Stored   bool     `json:"stored"`
	Parent   string   `json:"parent"`
	Event    string   `json:"event"`
	Commit   string   `json:"commit"`
	Warnings []string `json:"warnings"`
}

func (a *app) attachEvidenceCmd() *cobra.Command {
	var file, command, claim, actor string
	var exitCode int
	var allowLarge bool
	cmd := &cobra.Command{
		Use:   "attach-evidence <ref>",
		Short: "Attach a file or stdin as evidence to a node",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(file) == "" {
				return usage("--file is required, use - to read stdin")
			}
			if strings.TrimSpace(claim) == "" {
				return usage("--claim is required: say what this evidence proves")
			}
			var codePtr *int
			if cmd.Flags().Changed("exit-code") {
				codePtr = &exitCode
			}
			return a.attachEvidence(args[0], file, strings.TrimSpace(claim), strings.TrimSpace(command), codePtr, actor, allowLarge)
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "file to attach, or - to read stdin")
	cmd.Flags().StringVar(&claim, "claim", "", "what the evidence is meant to prove")
	cmd.Flags().StringVar(&command, "command", "", "command that produced the evidence")
	cmd.Flags().IntVar(&exitCode, "exit-code", 0, "exit code of the command")
	addActorFlag(cmd, &actor)
	cmd.Flags().BoolVar(&allowLarge, "allow-large", false, "store a file above the size cap")
	return cmd
}

func (a *app) attachEvidence(query, file, claim, command string, exitCode *int, actorFlag string, allowLarge bool) error {
	r, err := a.repo()
	if err != nil {
		return err
	}
	snap, err := r.Load()
	if err != nil {
		return fmt.Errorf("load repository: %w", err)
	}
	target, err := snap.Resolve(query)
	if err != nil {
		return fmt.Errorf("attach-evidence: %w", err)
	}
	cfg, err := r.Config()
	if err != nil {
		return fmt.Errorf("attach-evidence: %w", err)
	}

	var data []byte
	ext := ".txt"
	if file == "-" {
		data, err = io.ReadAll(a.in)
	} else {
		data, err = a.readFile(file)
		ext = strings.ToLower(filepath.Ext(file))
	}
	if err != nil {
		return fmt.Errorf("read evidence: %w", err)
	}
	if len(data) == 0 {
		return usage("the evidence is empty")
	}
	warnings := []string{}
	if size, limit := int64(len(data)), cfg.BlobCapBytes(); size > limit {
		msg := fmt.Sprintf("the attachment is %.1f MB, over the %d MB cap, and every blob stays in the repository history", float64(size)/(1<<20), limit>>20)
		if !allowLarge {
			return fail(exitUsage, "pass --allow-large to store it anyway, or raise blob_cap_mb in .tektite/config", "warning: %s", msg)
		}
		warnings = append(warnings, msg)
	}

	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	name := hash + ext
	_, stored, err := r.WriteBlob(name, data)
	if err != nil {
		return fmt.Errorf("store blob: %w", err)
	}

	actor, actorWarn := a.resolveActor(actorFlag)
	warnings = append(warnings, actorWarn...)
	at := a.now()
	commit := ""
	if a.gitHead != nil {
		if head := a.gitHead(); len(head) >= shortSHA {
			commit = head[:shortSHA]
		}
	}
	prov := map[string]any{"actor": actor, "at": parse.FormatTime(at)}
	if commit != "" {
		prov["commit"] = commit
		if a.gitDirty != nil && a.gitDirty() {
			prov["dirty"] = true
		}
	} else {
		warnings = append(warnings, check.CodeEvidenceNoCommit+": no git commit is available, so the evidence cannot be tied to a code version")
	}
	if command != "" {
		prov["command"] = command
	}
	if exitCode != nil {
		prov["exit_code"] = *exitCode
	}

	id, err := node.NewID(at, a.rand)
	if err != nil {
		return fmt.Errorf("new id: %w", err)
	}
	ref, err := node.NewRef(claim, id, snap.Refs())
	if err != nil {
		return fmt.Errorf("new reference: %w", err)
	}
	fields := []node.Field{
		{Key: "id", Value: id},
		{Key: "kind", Value: parse.KindEvidence},
		{Key: "title", Value: claim},
		{Key: "parent", Value: target.Ref},
		{Key: "evidences", Value: []string{target.Ref}},
		{Key: "claim", Value: claim},
		{Key: "attachments", Value: []string{"blobs/" + name}},
		{Key: "provenance", Value: prov},
	}
	src, err := node.Render(fields, "")
	if err != nil {
		return fmt.Errorf("render evidence: %w", err)
	}
	if _, err := r.WriteNode(ref, src); err != nil {
		return fmt.Errorf("create evidence node: %w", err)
	}
	res, err := a.writeEvent(actorFlag, target.Ref, &event.Draft{Type: parse.EventAttach, Fields: []node.Field{{Key: "evidence", Value: ref}}}, nil)
	if err != nil {
		return err
	}
	out := attachResult{Evidence: ref, Blob: filepath.ToSlash(filepath.Join(".tektite", "blobs", name)), Hash: hash, Stored: stored, Parent: target.Ref, Event: res.Event, Commit: commit, Warnings: warnings}
	if a.json {
		return a.writeJSON(out)
	}
	a.warn(warnings)
	a.printf(a.out, "%s\n", ref)
	return nil
}
