package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tektite-tracker/tektite/internal/event"
	"github.com/tektite-tracker/tektite/parse"
)

const (
	facetStatus   = "status"
	facetPriority = "priority"
	facetType     = "type"
	priorityNone  = "none"
)

var facetOptions = map[string][]string{
	facetStatus:   {event.StatusOpen, event.StatusActive, event.StatusBlocked, event.StatusReview, event.StatusDone, event.StatusDropped},
	facetPriority: {"critical", "high", "medium", "low", priorityNone},
	facetType:     {parse.KindTicket, parse.KindPlan},
}

var facetNames = []string{facetStatus, facetPriority, facetType}

type ImportMapping struct {
	Status   map[string]string `yaml:"status,omitempty" json:"status,omitempty"`
	Priority map[string]string `yaml:"priority,omitempty" json:"priority,omitempty"`
	Type     map[string]string `yaml:"type,omitempty" json:"type,omitempty"`
}

func (m *ImportMapping) facet(name string) map[string]string {
	switch name {
	case facetStatus:
		return m.Status
	case facetPriority:
		return m.Priority
	default:
		return m.Type
	}
}

func (m *ImportMapping) set(facet, value, target string) {
	switch facet {
	case facetStatus:
		if m.Status == nil {
			m.Status = map[string]string{}
		}
		m.Status[value] = target
	case facetPriority:
		if m.Priority == nil {
			m.Priority = map[string]string{}
		}
		m.Priority[value] = target
	default:
		if m.Type == nil {
			m.Type = map[string]string{}
		}
		m.Type[value] = target
	}
}

func (m *ImportMapping) size() int {
	return len(m.Status) + len(m.Priority) + len(m.Type)
}

type ImportQuestion struct {
	Facet   string   `json:"facet"`
	Value   string   `json:"value"`
	Count   int      `json:"count"`
	Options []string `json:"options"`
	Guess   string   `json:"guess,omitempty"`
	Reason  string   `json:"reason,omitempty"`
}

type facetValue struct {
	facet, value                      string
	count                             int
	cats                              map[string]int
	started, completed, canceledCount int
}

func scanFacets(data *importData) []*facetValue {
	index := map[string]*facetValue{}
	order := []*facetValue{}
	add := func(facet, value string) *facetValue {
		if value == "" {
			return nil
		}
		key := facet + "\x00" + value
		v := index[key]
		if v == nil {
			v = &facetValue{facet: facet, value: value, cats: map[string]int{}}
			index[key] = v
			order = append(order, v)
		}
		v.count++
		return v
	}
	for _, is := range data.issues {
		if v := add(facetStatus, is.rawStatus); v != nil {
			if is.category != "" {
				v.cats[normalizeName(is.category)]++
			}
			switch {
			case !is.canceledAt.IsZero():
				v.canceledCount++
			case !is.completedAt.IsZero():
				v.completed++
			case !is.startedAt.IsZero():
				v.started++
			}
		}
		add(facetPriority, is.rawPriority)
		add(facetType, is.rawTyp)
	}
	return order
}

func normalizeName(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(s, "_", " "), "-", " "))), " ")
}

var statusCategories = map[string]string{
	"backlog": event.StatusOpen, "unstarted": event.StatusOpen, "to do": event.StatusOpen, "todo": event.StatusOpen, "new": event.StatusOpen,
	"started": event.StatusActive, "in progress": event.StatusActive, "indeterminate": event.StatusActive,
	"completed": event.StatusDone, "done": event.StatusDone, "complete": event.StatusDone,
	"canceled": event.StatusDropped, "cancelled": event.StatusDropped,
}

var categoryRefinements = map[string][]string{
	event.StatusOpen:    {event.StatusOpen, event.StatusBlocked},
	event.StatusActive:  {event.StatusActive, event.StatusReview, event.StatusBlocked},
	event.StatusDone:    {event.StatusDone, event.StatusDropped},
	event.StatusDropped: {event.StatusDropped},
}

type keywordRule struct {
	target string
	re     *regexp.Regexp
}

func keywordRules(table []struct{ target, pattern string }) []keywordRule {
	rules := make([]keywordRule, 0, len(table))
	for _, e := range table {
		rules = append(rules, keywordRule{e.target, regexp.MustCompile(`(^|\s)(` + e.pattern + `)(\s|$)`)})
	}
	return rules
}

var statusKeywords = keywordRules([]struct{ target, pattern string }{
	{event.StatusDropped, `cancel(l?ed)?|duplicate|won'?t( do| fix)?|wont( do| fix)?|obsolete|rejected|abandoned|invalid|declined|dropped`},
	{event.StatusDone, `done|complete(d)?|closed|resolved|shipped|released|merged|finished|deployed|fixed`},
	{event.StatusBlocked, `blocked|on hold|hold|waiting|paused|stuck|deferred|impeded`},
	{event.StatusReview, `(in )?review|code review|qa|testing|verification|awaiting approval|approval`},
	{event.StatusActive, `in progress|started|doing|developing|working|implementing|active|wip`},
	{event.StatusOpen, `backlog|todo|to do|triage|icebox|new|open|unstarted|ready|planned|selected|queued|ready for dev|to be done`},
})

func keywordStatus(value string) string {
	name := normalizeName(value)
	for _, r := range statusKeywords {
		if r.re.MatchString(name) {
			return r.target
		}
	}
	return ""
}

func proposeStatus(v *facetValue) (target, reason string, certain bool) {
	keyword := keywordStatus(v.value)
	if len(v.cats) == 1 {
		for cat, n := range v.cats {
			if base, ok := statusCategories[cat]; ok && n == v.count {
				if slices.Contains(categoryRefinements[base], keyword) {
					return keyword, "category " + cat + ", refined by the name", true
				}
				return base, "category " + cat, true
			}
		}
	}
	switch {
	case v.canceledCount == v.count:
		return event.StatusDropped, "every issue has a canceled time", true
	case v.completed == v.count:
		return event.StatusDone, "every issue has a completed time", true
	case v.started == v.count:
		if slices.Contains(categoryRefinements[event.StatusActive], keyword) {
			return keyword, "every issue has a started time, refined by the name", true
		}
		return event.StatusActive, "every issue has a started time", true
	}
	if keyword != "" {
		return keyword, "name", true
	}
	if v.started*2 > v.count {
		return event.StatusActive, "most issues have a started time", false
	}
	return "", "no category, timestamp or name evidence", false
}

var priorityNames = map[string]string{
	"urgent": "critical", "critical": "critical", "blocker": "critical", "highest": "critical", "p0": "critical", "showstopper": "critical",
	"high": "high", "major": "high", "p1": "high", "important": "high",
	"medium": "medium", "normal": "medium", "moderate": "medium", "p2": "medium",
	"low": "low", "minor": "low", "lowest": "low", "trivial": "low", "p3": "low", "p4": "low",
	"no priority": priorityNone, "none": priorityNone, "unset": priorityNone, "n/a": priorityNone, "unprioritized": priorityNone, "unprioritised": priorityNone,
}

var typeNames = map[string]string{
	"epic":  parse.KindPlan,
	"story": parse.KindTicket, "task": parse.KindTicket, "bug": parse.KindTicket, "sub task": parse.KindTicket, "subtask": parse.KindTicket,
	"improvement": parse.KindTicket, "feature": parse.KindTicket, "new feature": parse.KindTicket, "technical task": parse.KindTicket,
	"spike": parse.KindTicket, "chore": parse.KindTicket, "incident": parse.KindTicket, "support": parse.KindTicket,
}

func propose(v *facetValue) (target, reason string, certain bool) {
	switch v.facet {
	case facetStatus:
		return proposeStatus(v)
	case facetPriority:
		if t, ok := priorityNames[normalizeName(v.value)]; ok {
			return t, "name", true
		}
		return "", "unrecognised priority name", false
	default:
		if t, ok := typeNames[normalizeName(v.value)]; ok {
			return t, "name", true
		}
		return parse.KindTicket, "unrecognised issue type", false
	}
}

func importMappingPath(root, tracker string) string {
	return filepath.Join(root, ".tektite", "import", tracker+".yaml")
}

func readImportMapping(path string) (*ImportMapping, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read mapping: %w", err)
	}
	m := &ImportMapping{}
	if err := yaml.Unmarshal(data, m); err != nil {
		return nil, usageError("mapping %s is not valid: %v", filepath.ToSlash(path), err)
	}
	for _, name := range facetNames {
		for value, target := range m.facet(name) {
			if !slices.Contains(facetOptions[name], target) {
				return nil, usageError("mapping %s maps %s %q to %q, use one of %s", filepath.ToSlash(path), name, value, target, strings.Join(facetOptions[name], ", "))
			}
		}
	}
	return m, nil
}

func (s *Service) resolveImportMapping(in ImportInput, src importSource, data *importData, res *ImportResult) (*ImportMapping, error) {
	root := filepath.Dir(s.Repo.Dir())
	saved := importMappingPath(root, src.name())
	base := &ImportMapping{}
	switch {
	case in.MapFile != "":
		m, err := readImportMapping(in.MapFile)
		if errors.Is(err, os.ErrNotExist) {
			return nil, newError(KindNotFound, "pass --map with the path of a mapping file", "no mapping file at %s", filepath.ToSlash(in.MapFile))
		} else if err != nil {
			return nil, err
		}
		base = m
	default:
		if m, err := readImportMapping(saved); err == nil {
			base = m
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	had := base.size()
	pending := []*ImportQuestion{}
	for _, v := range scanFacets(data) {
		if _, ok := base.facet(v.facet)[v.value]; ok {
			continue
		}
		target, reason, certain := propose(v)
		if certain {
			base.set(v.facet, v.value, target)
			continue
		}
		pending = append(pending, &ImportQuestion{Facet: v.facet, Value: v.value, Count: v.count, Options: facetOptions[v.facet], Guess: target, Reason: reason})
	}
	if len(pending) > 0 {
		if err := s.settleQuestions(in, base, pending); err != nil {
			return nil, err
		}
	}
	res.MappingPath = ".tektite/import/" + src.name() + ".yaml"
	res.MappingNew = in.MapFile == "" && !in.DryRun && base.size() != had
	return base, nil
}

func (s *Service) settleQuestions(in ImportInput, m *ImportMapping, pending []*ImportQuestion) error {
	unmapped := []string{}
	for _, q := range pending {
		switch {
		case in.MapFile == "" && in.Ask != nil:
			answer, err := in.Ask(*q)
			if err != nil {
				return err
			}
			if !slices.Contains(q.Options, answer) {
				return usageError("%s %q cannot map to %q, use one of %s", q.Facet, q.Value, answer, strings.Join(q.Options, ", "))
			}
			m.set(q.Facet, q.Value, answer)
		case in.Yes && q.Guess != "":
			m.set(q.Facet, q.Value, q.Guess)
		default:
			unmapped = append(unmapped, fmt.Sprintf("%s %q", q.Facet, q.Value))
		}
	}
	if len(unmapped) > 0 {
		return newError(KindUsage, "run in a terminal to be asked, save a mapping in .tektite/import, or pass --map with a mapping file", "unmapped values: %s", strings.Join(unmapped, ", "))
	}
	return nil
}

func (s *Service) saveImportMapping(in ImportInput, res *ImportResult) error {
	if !res.MappingNew {
		return nil
	}
	data, err := yaml.Marshal(res.Mapping)
	if err != nil {
		return fmt.Errorf("encode mapping: %w", err)
	}
	path := filepath.Join(filepath.Dir(s.Repo.Dir()), filepath.FromSlash(res.MappingPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("save mapping: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("save mapping: %w", err)
	}
	return nil
}

func applyImportMapping(data *importData, m *ImportMapping) {
	for _, is := range data.issues {
		is.status = event.StatusOpen
		if is.rawStatus != "" {
			is.status = m.Status[is.rawStatus]
		} else {
			is.statusWarns = append(is.statusWarns, importWarn{parse.CodeImportUnmapped, "status", "issue has no status, imported as open"})
		}
		if p := m.Priority[is.rawPriority]; p != "" && p != priorityNone {
			is.priority = p
		}
		is.kind = parse.KindTicket
		if m.Type[is.rawTyp] == parse.KindPlan {
			is.kind = parse.KindPlan
		}
	}
}
