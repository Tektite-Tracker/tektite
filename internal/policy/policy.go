package policy

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tektite-tracker/tektite/internal/store"
	"github.com/tektite-tracker/tektite/parse"
)

const (
	FileName = "policy.md"
	Version  = 1

	RuleApproval = "plan-approval"
	RuleEvidence = "passing-evidence"
	RuleNoGaps   = "no-open-gaps"
)

var rules = map[string]bool{RuleApproval: true, RuleEvidence: true, RuleNoGaps: true}

var hints = map[string]string{
	RuleApproval: "ask a human to run `tektite approve` on the plan",
	RuleEvidence: "attach passing evidence with `tektite attach-evidence`",
	RuleNoGaps:   "close the open gaps",
}

func Hint(rule string) string {
	return hints[rule]
}

type Gate struct {
	Name   string   `yaml:"name" json:"name"`
	Rule   string   `yaml:"rule" json:"rule"`
	Kinds  []string `yaml:"kinds" json:"kinds,omitempty"`
	Labels []string `yaml:"labels" json:"labels,omitempty"`
}

type Policy struct {
	Version int    `yaml:"version" json:"version"`
	Gates   []Gate `yaml:"gates" json:"gates"`
	Path    string `yaml:"-" json:"path"`
}

type MalformedError struct {
	Path string
	Line int
	Msg  string
}

func (e *MalformedError) Error() string {
	return fmt.Sprintf("policy %s is malformed: %s", e.Path, e.Msg)
}

func Path(r *store.Repo) string {
	return filepath.Join(r.Dir(), FileName)
}

func Load(r *store.Repo) (*Policy, error) {
	rel := r.Rel(Path(r))
	src, err := os.ReadFile(Path(r))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read policy: %w", err)
	}
	p, perr := Parse(src)
	if perr != nil {
		perr.Path = rel
		return nil, perr
	}
	p.Path = rel
	return p, nil
}

var yamlLine = regexp.MustCompile(`line (\d+)`)

func Parse(src []byte) (*Policy, *MalformedError) {
	raw, _, present := parse.SplitFrontmatter(src)
	if !present {
		return nil, &MalformedError{Msg: "the file has no complete frontmatter block", Line: 1}
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	var p Policy
	if err := dec.Decode(&p); err != nil {
		line := 0
		if m := yamlLine.FindStringSubmatch(err.Error()); m != nil {
			line, _ = strconv.Atoi(m[1])
			line++
		}
		return nil, &MalformedError{Msg: strings.TrimPrefix(err.Error(), "yaml: "), Line: line}
	}
	if p.Version != Version {
		return nil, &MalformedError{Msg: fmt.Sprintf("version must be %d, got %d", Version, p.Version), Line: 2}
	}
	seen := map[string]bool{}
	for i, g := range p.Gates {
		switch {
		case strings.TrimSpace(g.Name) == "":
			return nil, &MalformedError{Msg: fmt.Sprintf("gate %d has no name", i+1)}
		case !rules[g.Rule]:
			return nil, &MalformedError{Msg: fmt.Sprintf("gate %q has unknown rule %q, expected one of %s, %s, %s", g.Name, g.Rule, RuleApproval, RuleEvidence, RuleNoGaps)}
		case seen[g.Name]:
			return nil, &MalformedError{Msg: fmt.Sprintf("gate name %q is used twice", g.Name)}
		}
		seen[g.Name] = true
	}
	return &p, nil
}

func (g Gate) Applies(nf *store.NodeFile) bool {
	if len(g.Kinds) > 0 && !containsTrimmed(g.Kinds, nf.Node.Kind) {
		return false
	}
	if len(g.Labels) > 0 {
		for _, l := range nf.Labels() {
			if containsTrimmed(g.Labels, l) {
				return true
			}
		}
		return false
	}
	return true
}

func containsTrimmed(list []string, s string) bool {
	for _, x := range list {
		if strings.TrimSpace(x) == s {
			return true
		}
	}
	return false
}
