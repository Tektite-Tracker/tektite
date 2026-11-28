package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/policy"
	"github.com/tektite-tracker/tektite/internal/service"
	"github.com/tektite-tracker/tektite/internal/store"
	"github.com/tektite-tracker/tektite/parse"
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
	gitLog      func(since string, paths []string) ([]string, error)
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

var serviceExit = map[service.Kind]int{
	service.KindInternal: exitError,
	service.KindUsage:    exitUsage,
	service.KindNotFound: exitNotFound,
	service.KindConflict: exitConflict,
	service.KindPolicy:   exitPolicy,
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
	if errors.As(err, &ce) {
		return ce
	}
	se := service.Classify(err)
	return &cliError{code: serviceExit[se.Kind], msg: se.Msg, hint: se.Hint, gates: se.Gates}
}

func run(args []string, e env) int {
	if e.gitUser == nil {
		e.gitUser = func() string { return "" }
	}
	if e.gitHead == nil {
		e.gitHead = func() string { return "" }
	}
	if e.gitDirty == nil {
		e.gitDirty = func() bool { return false }
	}
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
	a.printError(ce, a.json || jsonRequested(args))
	return ce.code
}

func jsonRequested(args []string) bool {
	requested := false
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--json" {
			requested = true
		} else if v, ok := strings.CutPrefix(arg, "--json="); ok {
			b, err := strconv.ParseBool(v)
			requested = err == nil && b
		}
	}
	return requested
}

type errorResult struct {
	Code  int             `json:"code"`
	Error string          `json:"error"`
	Gates []policy.Result `json:"gates,omitempty"`
	Hint  string          `json:"hint"`
}

func (a *app) printError(ce *cliError, asJSON bool) {
	line := "tektite: " + ce.msg
	if ce.hint != "" {
		line += "\nhint: " + ce.hint
	}
	a.printf(a.err, "%s\n", line)
	if asJSON {
		_ = a.writeJSON(errorResult{Code: ce.code, Error: ce.msg, Gates: ce.gates, Hint: ce.hint})
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

func warningTexts(ds []parse.Diagnostic) []string {
	if ds == nil {
		return nil
	}
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		if d.Code == "" {
			out = append(out, d.Message)
			continue
		}
		out = append(out, d.Code+": "+d.Message)
	}
	return out
}

type gitFuncs struct {
	user  func() string
	head  func() string
	dirty func() bool
	log   func(since string, paths []string) ([]string, error)
}

func (g gitFuncs) User() string { return g.user() }
func (g gitFuncs) Head() string { return g.head() }
func (g gitFuncs) Dirty() bool  { return g.dirty() }

func (g gitFuncs) CommitsSince(since string, paths []string) ([]string, error) {
	if g.log == nil {
		return nil, nil
	}
	return g.log(since, paths)
}

func (a *app) svc() (*service.Service, error) {
	r, err := a.repo()
	if err != nil {
		return nil, err
	}
	return &service.Service{
		Repo:   r,
		Now:    a.now,
		Rand:   a.rand,
		Getenv: a.getenv,
		Git:    gitFuncs{user: a.gitUser, head: a.gitHead, dirty: a.gitDirty, log: a.gitLog},
	}, nil
}

func (a *app) emit(v any, text func()) error {
	if a.json {
		return a.writeJSON(v)
	}
	text()
	return nil
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
		Use:           "tektite",
		Version:       version,
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
		a.primeCmd(),
		a.docsCmd(),
		a.importCmd(),
		a.newCmd(),
		a.showCmd(),
		a.lsCmd(),
		a.checkCmd(),
		a.fmtCmd(),
		a.gcCmd(),
		a.archiveCmd(),
		a.unarchiveCmd(),
		a.schemaCmd(),
		a.setStatusCmd(),
		a.claimCmd(),
		a.leaseCmd(parse.EventHeartbeat, "Extend your lease on a node"),
		a.leaseCmd(parse.EventRelease, "Release your lease on a node"),
		a.nextCmd(),
		a.contextCmd(),
		a.attachEvidenceCmd(),
		a.rerunCmd(),
		a.reviseCmd(),
		a.revisionsCmd(),
		a.diffCmd(),
		a.approveCmd(),
		a.policyCmd(),
		a.noteCmd(),
		a.mcpCmd(),
		a.closeGapCmd(),
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
