package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tektite-tracker/tektite/parse"
)

const (
	callTimeout = 60 * time.Second
	longTitle   = 100
)

var (
	siteRe     = regexp.MustCompile(`tektite/([A-Za-z0-9_./-]+\.go:\d+)`)
	refShapeRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*-[a-z0-9]{3,}$`)
)

type tally map[string]int

func (t tally) add(k string) {
	t[k]++
}

func (t tally) lines(w io.Writer, indent string) {
	keys := make([]string, 0, len(t))
	for k := range t {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		_, _ = fmt.Fprintf(w, "%s%s: %d\n", indent, k, t[k])
	}
}

type report struct {
	files, skippedDirs int
	bytes, maxBytes    int
	parseNs, maxNs     int64
	codes              tally
	shape              tally
	panics             tally
	cli                tally
	cliSites           tally
	timings            map[string]time.Duration
}

func newReport() *report {
	return &report{codes: tally{}, shape: tally{}, panics: tally{}, cli: tally{}, cliSites: tally{}, timings: map[string]time.Duration{}}
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, out, errw io.Writer) int {
	flags := flag.NewFlagSet("realnotes", flag.ContinueOnError)
	flags.SetOutput(errw)
	tkt := flags.String("tkt", "bin/tkt", "path to the tkt binary")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() == 0 {
		_, _ = fmt.Fprintln(out, "realnotes: no path given, skipping")
		return 0
	}
	exit := 0
	for i, dir := range flags.Args() {
		_, _ = fmt.Fprintf(out, "== folder %d ==\n", i+1)
		rep, err := analyse(dir, *tkt)
		if err != nil {
			_, _ = fmt.Fprintf(out, "error: %s\n", classifyErr(err))
			exit = 1
			continue
		}
		rep.print(out)
	}
	return exit
}

func classifyErr(err error) string {
	switch {
	case os.IsNotExist(err):
		return "folder does not exist"
	case os.IsPermission(err):
		return "folder is not readable"
	}
	return "analysis failed"
}

func analyse(dir, tkt string) (*report, error) {
	files, skipped, err := collect(dir)
	if err != nil {
		return nil, err
	}
	rep := newReport()
	rep.files, rep.skippedDirs = len(files), skipped
	contents := make([][]byte, 0, len(files))
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			rep.shape.add("unreadable")
			continue
		}
		contents = append(contents, src)
		rep.parseOne(src)
	}
	rep.cliPhase(contents, tkt)
	return rep, nil
}

func collect(root string) ([]string, int, error) {
	if _, err := os.Stat(root); err != nil {
		return nil, 0, fmt.Errorf("stat root: %w", err)
	}
	var files []string
	skipped := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != root && strings.HasPrefix(d.Name(), ".") {
				skipped++
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() && strings.EqualFold(filepath.Ext(d.Name()), ".md") && !strings.HasPrefix(d.Name(), ".") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, 0, fmt.Errorf("walk root: %w", err)
	}
	sort.Strings(files)
	return files, skipped, nil
}

func sites(stack string) []string {
	var out []string
	for _, m := range siteRe.FindAllStringSubmatch(stack, -1) {
		if !strings.HasPrefix(m[1], "scripts/realnotes") {
			out = append(out, m[1])
		}
	}
	return out
}

func (r *report) parseOne(src []byte) {
	r.bytes += len(src)
	r.maxBytes = max(r.maxBytes, len(src))
	start := time.Now()
	n, diags, panicked := safeParse(src)
	ns := time.Since(start).Nanoseconds()
	r.parseNs += ns
	r.maxNs = max(r.maxNs, ns)
	if panicked != "" {
		for _, s := range sites(panicked) {
			r.panics.add(s)
		}
		r.panics.add("total")
		return
	}
	for _, d := range diags {
		r.codes.add(d.Code)
	}
	r.shapeOf(src, n)
}

func safeParse(src []byte) (n *parse.Node, diags []parse.Diagnostic, panicked string) {
	defer func() {
		if rec := recover(); rec != nil {
			panicked = string(debug.Stack())
		}
	}()
	n, diags = parse.ParseNode("note.md", src)
	return n, diags, ""
}

func (r *report) shapeOf(src []byte, n *parse.Node) {
	r.shape.add("nodes parsed")
	if !utf8.Valid(src) {
		r.shape.add("invalid utf8")
	}
	if bytes.HasPrefix(src, []byte("\xef\xbb\xbf")) {
		r.shape.add("bom")
	}
	if bytes.Contains(src, []byte("\r\n")) {
		r.shape.add("crlf")
	}
	if len(bytes.TrimSpace(src)) == 0 {
		r.shape.add("blank file")
	}
	if n.Frontmatter {
		r.shape.add("frontmatter present")
	}
	if n.ID != "" {
		r.shape.add("has id")
	}
	if n.Parent != "" {
		r.shape.add("has parent")
	}
	if len(n.Blocks) > 0 {
		r.shape.add("has checklist blocks")
	}
	if len(n.Links) > 0 {
		r.shape.add("has wikilinks")
	}
	for _, l := range n.Links {
		r.shape.add("wikilinks total")
		switch {
		case refShapeRe.MatchString(l.Target):
			r.shape.add("wikilink shaped like a ref")
		case strings.ContainsAny(l.Target, " ABCDEFGHIJKLMNOPQRSTUVWXYZ/"):
			r.shape.add("wikilink shaped like a title or path")
		default:
			r.shape.add("wikilink other shape")
		}
	}
	title := n.Title
	switch {
	case strings.TrimSpace(title) == "":
		r.shape.add("title empty")
	case strings.ContainsAny(title, "\n\r"):
		r.shape.add("title has newline")
	}
	if strings.ContainsAny(title, "*`[]<>") || strings.HasPrefix(title, "#") {
		r.shape.add("title has markup")
	}
	if utf8.RuneCountInString(title) > longTitle {
		r.shape.add("title over 100 chars")
	}
	if strings.TrimSpace(title) != title {
		r.shape.add("title has outer whitespace")
	}
	for k := range n.Fields {
		if k == "tags" || k == "aliases" || k == "date" || k == "created" {
			r.shape.add("common field " + k)
		}
	}
}

type tktResult struct {
	stdout []byte
	code   int
	panic  bool
}

func (r *report) tkt(bin, dir string, args ...string) tktResult {
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	start := time.Now()
	err := cmd.Run()
	r.timings[args[0]] += time.Since(start)
	res := tktResult{stdout: out.Bytes()}
	if err != nil {
		res.code = -1
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			res.code = ee.ExitCode()
		}
		if ctx.Err() != nil {
			res.code = -2
		}
	}
	if s := errb.String(); strings.Contains(s, "goroutine ") && strings.Contains(s, "panic") {
		res.panic = true
		for _, site := range sites(s) {
			r.cliSites.add(site)
		}
	}
	return res
}

func (r *report) cliPhase(contents [][]byte, bin string) {
	tmp, err := os.MkdirTemp("", "realnotes-*")
	if err != nil {
		r.cli.add("temp dir failed")
		return
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	nodes := filepath.Join(tmp, ".tektite", "nodes")
	if err := os.MkdirAll(nodes, 0o755); err != nil {
		r.cli.add("temp dir failed")
		return
	}
	for i, src := range contents {
		if err := os.WriteFile(filepath.Join(nodes, fmt.Sprintf("n-%05x.md", i)), src, 0o600); err != nil {
			r.cli.add("temp write failed")
			return
		}
	}
	r.runCheck(bin, tmp)
	refs := r.runLs(bin, tmp)
	r.runFmt(bin, tmp)
	r.runFmtWrite(bin, tmp, contents)
	r.runShow(bin, tmp, refs)
}

func (r *report) note(name string, res tktResult) {
	r.cli[name+" exit "+fmt.Sprint(res.code)]++
	if res.panic {
		r.cli[name+" panics"]++
	}
}

func (r *report) runCheck(bin, dir string) {
	res := r.tkt(bin, dir, "check", "--json")
	r.note("check", res)
	var rep struct {
		Problems []struct {
			Code     string `json:"code"`
			Severity string `json:"severity"`
		} `json:"problems"`
	}
	if json.Unmarshal(res.stdout, &rep) != nil {
		r.cli["check output not json"]++
		return
	}
	for _, p := range rep.Problems {
		r.cli["check problem "+p.Code]++
		r.cli["check severity "+p.Severity]++
	}
}

func (r *report) runLs(bin, dir string) []string {
	res := r.tkt(bin, dir, "ls", "--json")
	r.note("ls", res)
	var list []struct {
		Ref    string `json:"ref"`
		Kind   string `json:"kind"`
		Status string `json:"status"`
	}
	if json.Unmarshal(res.stdout, &list) != nil {
		r.cli["ls output not json"]++
		return nil
	}
	kinds := map[string]bool{}
	refs := []string{}
	for _, it := range list {
		kinds[it.Kind] = true
		refs = append(refs, it.Ref)
		r.cli["ls status "+it.Status]++
	}
	r.cli["ls nodes"] = len(list)
	r.cli["ls distinct kinds"] = len(kinds)
	return refs
}

func (r *report) runFmt(bin, dir string) {
	res := r.tkt(bin, dir, "fmt", "--dry-run", "--json")
	r.note("fmt", res)
	var items []struct {
		Changes []string `json:"changes"`
		Skipped string   `json:"skipped"`
	}
	if json.Unmarshal(res.stdout, &items) != nil {
		r.cli["fmt output not json"]++
		return
	}
	for _, it := range items {
		if it.Skipped != "" {
			r.cli["fmt skipped"]++
			continue
		}
		for _, c := range it.Changes {
			r.cli["fmt change "+c]++
		}
	}
}

func (r *report) runFmtWrite(bin, dir string, contents [][]byte) {
	res := r.tkt(bin, dir, "fmt", "--json")
	r.note("fmt write", res)
	again := r.tkt(bin, dir, "fmt", "--dry-run", "--json")
	var items []struct {
		Changes []string `json:"changes"`
	}
	if json.Unmarshal(again.stdout, &items) == nil && len(items) > 0 {
		r.cli["fmt second run still changing"] = len(items)
	}
	for i, src := range contents {
		before, _, had := parse.SplitFrontmatter(src)
		if !had {
			continue
		}
		out, err := os.ReadFile(filepath.Join(dir, ".tektite", "nodes", fmt.Sprintf("n-%05x.md", i)))
		if err != nil {
			continue
		}
		after, _, _ := parse.SplitFrontmatter(out)
		lost := false
		for _, line := range strings.Split(string(before), "\n") {
			t := strings.TrimSpace(line)
			if t == "" || strings.HasPrefix(t, "id:") || strings.HasPrefix(t, "kind:") || strings.HasPrefix(t, "title:") || strings.HasPrefix(t, "parent:") {
				continue
			}
			if !strings.Contains(string(after), t) {
				lost = true
			}
		}
		if lost {
			r.cli["fmt files with a frontmatter line lost"]++
		}
	}
}

func (r *report) runShow(bin, dir string, refs []string) {
	for _, ref := range refs {
		res := r.tkt(bin, dir, "show", ref, "--json")
		r.note("show", res)
	}
}

func (r *report) print(w io.Writer) {
	p := func(format string, a ...any) { _, _ = fmt.Fprintf(w, format, a...) }
	p("files: %d\nskipped hidden dirs: %d\nbytes: %d (largest %d)\n", r.files, r.skippedDirs, r.bytes, r.maxBytes)
	p("parse total: %d ms, slowest file: %d ms\n", r.parseNs/1e6, r.maxNs/1e6)
	p("parser panics: %d\n", r.panics["total"])
	r.panics.lines(w, "  site ")
	p("parser diagnostics by code:\n")
	r.codes.lines(w, "  ")
	p("shape:\n")
	r.shape.lines(w, "  ")
	p("tkt:\n")
	r.cli.lines(w, "  ")
	r.cliSites.lines(w, "  panic site ")
	names := make([]string, 0, len(r.timings))
	for k := range r.timings {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		p("  time %s: %d ms\n", k, r.timings[k].Milliseconds())
	}
}
