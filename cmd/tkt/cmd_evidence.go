package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/service"
)

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
	var file, command, claim, actor, runCmd string
	var exitCode int
	var allowLarge bool
	cmd := &cobra.Command{
		Use:   "attach-evidence <ref>",
		Short: "Attach a file or stdin as evidence to a node",
		Args:  exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(file) == "" && strings.TrimSpace(runCmd) == "" {
				return usage("--file or --run is required, use --file - to read stdin")
			}
			if strings.TrimSpace(file) != "" && strings.TrimSpace(runCmd) != "" {
				return usage("--run cannot be combined with --file or --command")
			}
			if strings.TrimSpace(claim) == "" {
				return usage("--claim is required: say what this evidence proves")
			}
			var codePtr *int
			if cmd.Flags().Changed("exit-code") {
				codePtr = &exitCode
			}
			return a.attachEvidence(args[0], file, strings.TrimSpace(claim), strings.TrimSpace(command), strings.TrimSpace(runCmd), codePtr, actor, allowLarge)
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "file to attach, or - to read stdin")
	cmd.Flags().StringVar(&claim, "claim", "", "what the evidence is meant to prove")
	cmd.Flags().StringVar(&runCmd, "run", "", "run a command and store its output as evidence")
	cmd.Flags().StringVar(&command, "command", "", "command that produced the evidence")
	cmd.Flags().IntVar(&exitCode, "exit-code", 0, "exit code of the command")
	addActorFlag(cmd, &actor)
	cmd.Flags().BoolVar(&allowLarge, "allow-large", false, "store a file above the size cap")
	return cmd
}

func (a *app) attachEvidence(query, file, claim, command, runCmd string, exitCode *int, actorFlag string, allowLarge bool) error {
	var data []byte
	var err error
	ext := ".txt"
	if runCmd != "" {
		return a.reportAttached(a.runAttach(service.AttachInput{Ref: query, Run: runCmd, Claim: claim, Command: command, ExitCode: exitCode, Actor: actorFlag, AllowLarge: allowLarge}))
	}
	if file == "-" {
		data, err = io.ReadAll(a.in)
	} else {
		data, err = a.readFile(file)
		ext = filepath.Ext(file)
	}
	if err != nil {
		return fmt.Errorf("read evidence: %w", err)
	}
	return a.reportAttached(a.runAttach(service.AttachInput{Ref: query, Data: data, Ext: ext, Claim: claim, Command: command, ExitCode: exitCode, Actor: actorFlag, AllowLarge: allowLarge}))
}

func (a *app) runAttach(in service.AttachInput) (*service.Attached, error) {
	svc, err := a.svc()
	if err != nil {
		return nil, err
	}
	return svc.AttachEvidence(in)
}

func (a *app) reportAttached(res *service.Attached, err error) error {
	if err != nil {
		return err
	}
	warnings := warningTexts(res.Warnings)
	out := attachResult{Evidence: res.Evidence, Blob: res.Blob, Hash: res.Hash, Stored: res.Stored, Parent: res.Parent, Event: res.Event, Commit: res.Commit, Warnings: warnings}
	return a.emit(out, func() {
		a.warn(warnings)
		a.printf(a.out, "%s\n", res.Evidence)
	})
}

func (a *app) rerunCmd() *cobra.Command {
	var actor string
	cmd := &cobra.Command{
		Use:   "rerun <ref>",
		Short: "Run the command of an evidence node again and record the result as new evidence",
		Args:  exactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			svc, err := a.svc()
			if err != nil {
				return err
			}
			return a.reportAttached(svc.Rerun(service.RerunInput{Ref: args[0], Actor: actor}))
		},
	}
	addActorFlag(cmd, &actor)
	return cmd
}
