package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/node"
	"github.com/tektite-tracker/tektite/internal/policy"
	"github.com/tektite-tracker/tektite/internal/store"
)

const (
	exitOK       = 0
	exitError    = 1
	exitUsage    = 2
	exitNotFound = 3
	exitConflict = 4
	exitPolicy   = 5
)

type env struct {
	in          io.Reader
	out         io.Writer
	err         io.Writer
	cwd         string
	now         func() time.Time
	rand        io.Reader
	getenv      func(string) string
	interactive bool
	gitUser     func() string
	gitHead     func() string
	gitDirty    func() bool
}

type app struct {
	env
	json bool
	root string
}

type cliError struct {
	code   int
	msg    string
	hint   string
	silent bool
	gates  []policy.Result
}

func (e *cliError) Error() string {
	return e.msg
}

func fail(code int, hint, format string, args ...any) error {
	return &cliError{code: code, msg: fmt.Sprintf(format, args...), hint: hint}
}

func usage(format string, args ...any) error {
	return fail(exitUsage, "run with --help to see the usage", format, args...)
}

func exitSilently(code int) error {
	return &cliError{code: code, silent: true}
}

func classify(err error) *cliError {
	var ce *cliError
	var amb *node.AmbiguousError
	switch {
	case errors.As(err, &ce):
		return ce
	case errors.As(err, &amb):
		return &cliError{code: exitUsage, msg: err.Error(), hint: "use a longer hash to pick one"}
	case errors.Is(err, node.ErrNotFound):
		return &cliError{code: exitNotFound, msg: err.Error(), hint: "run `tkt ls` to list references"}
	case errors.Is(err, node.ErrTooShort):
		return &cliError{code: exitUsage, msg: err.Error(), hint: fmt.Sprintf("use at least %d hash characters", node.MinHashLen)}
	case errors.Is(err, store.ErrNoRepo):
		return &cliError{code: exitNotFound, msg: err.Error(), hint: "run `tkt init` to create one"}
	case errors.Is(err, store.ErrExists):
		return &cliError{code: exitConflict, msg: err.Error(), hint: "retry the command"}
	}
	return &cliError{code: exitError, msg: err.Error()}
}

func run(args []string, e env) int {
	a := &app{env: e}
	root := a.newRoot()
	root.SetArgs(args)
	root.SetOut(e.out)
	root.SetErr(e.err)
	root.SetIn(e.in)
	err := root.Execute()
	if err == nil {
		return exitOK
	}
	ce := classify(err)
	if ce.silent {
		return ce.code
	}
	a.printError(ce, a.json || slices.Contains(args, "--json"))
	return ce.code
}

func (a *app) printError(ce *cliError, asJSON bool) {
	line := "tkt: " + ce.msg
	if ce.hint != "" {
		line += "\nhint: " + ce.hint
	}
	a.printf(a.err, "%s\n", line)
	if asJSON {
		doc := map[string]any{"error": ce.msg, "code": ce.code, "hint": ce.hint}
		if ce.gates != nil {
			doc["gates"] = ce.gates
		}
		_ = a.writeJSON(doc)
	}
}

func (a *app) readFile(name string) ([]byte, error) {
	if !filepath.IsAbs(name) {
		name = filepath.Join(a.cwd, name)
	}
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	return data, nil
}

func (a *app) printf(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

func (a *app) warn(warnings []string) {
	for _, w := range warnings {
		a.printf(a.err, "warning: %s\n", w)
	}
}

func (a *app) writeJSON(v any) error {
	enc := json.NewEncoder(a.out)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("write json: %w", err)
	}
	return nil
}

func (a *app) repo() (*store.Repo, error) {
	if a.root != "" {
		r, err := store.Open(a.root)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", a.root, err)
		}
		return r, nil
	}
	r, err := store.Discover(a.cwd)
	if err != nil {
		return nil, fmt.Errorf("from %s: %w", a.cwd, err)
	}
	return r, nil
}

func (a *app) load() (*store.Repo, *store.Snapshot, error) {
	r, err := a.repo()
	if err != nil {
		return nil, nil, err
	}
	snap, err := r.Load()
	if err != nil {
		return nil, nil, fmt.Errorf("load repository: %w", err)
	}
	return r, snap, nil
}

func (a *app) newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "tkt",
		Short:         "Git-like, local-first work tracking for humans and agents",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usage("unknown command %q", args[0])
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	root.PersistentFlags().BoolVar(&a.json, "json", false, "print JSON instead of text")
	root.PersistentFlags().StringVar(&a.root, "root", "", "project directory that holds .tektite/")
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return usage("%s", err)
	})
	root.AddCommand(
		a.initCmd(),
		a.newCmd(),
		a.showCmd(),
		a.lsCmd(),
		a.checkCmd(),
		a.fmtCmd(),
		a.schemaCmd(),
		a.setStatusCmd(),
		a.claimCmd(),
		a.leaseCmd("heartbeat", "Extend your lease on a node", true),
		a.leaseCmd("release", "Release your lease on a node", false),
		a.nextCmd(),
		a.contextCmd(),
		a.attachEvidenceCmd(),
		a.reviseCmd(),
		a.revisionsCmd(),
		a.diffCmd(),
		a.approveCmd(),
		a.policyCmd(),
		a.noteCmd(),
		a.linkCmd("link", "Add a link to a node"),
		a.linkCmd("unlink", "Remove a link from a node"),
	)
	return root
}

func addActorFlag(cmd *cobra.Command, dst *string) {
	cmd.Flags().StringVar(dst, "actor", "", "actor recorded on the event")
}

func addRevisionFlag(cmd *cobra.Command, dst *int) {
	cmd.Flags().IntVar(dst, "revision", 0, "plan revision the work is built against")
}

func exactArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != n {
			return usage("%s takes %d argument(s), got %d", cmd.CommandPath(), n, len(args))
		}
		return nil
	}
}
