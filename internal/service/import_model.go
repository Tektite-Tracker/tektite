package service

import (
	"time"

	"github.com/tektite-tracker/tektite/internal/node"
	"github.com/tektite-tracker/tektite/parse"
)

const (
	sourceField   = "source"
	sourceIDField = "source_id"
	legacyBeadsID = "beads_id"
	relBlocks     = parse.LinkBlocks
	relDerived    = parse.LinkDerivedFrom
	relSupersedes = parse.LinkSupersedes
	relRelates    = "relates-to"
)

type importWarn struct {
	code, field, msg string
}

type importLink struct {
	rel     string
	other   string
	inbound bool
	unmaps  string
}

type importComment struct {
	author string
	text   string
	at     time.Time
}

type importIssue struct {
	id, title, body string
	line            int

	kind, status, priority         string
	rawStatus, rawPriority, rawTyp string
	category                       string

	labels    []any
	assignee  string
	parent    string
	links     []importLink
	comments  []importComment
	closeNote string
	extra     []node.Field

	createdAt, startedAt, completedAt, canceledAt, closedAt time.Time

	statusWarns, createWarns, eventWarns []importWarn

	ref   string
	isNew bool
}

type importData struct {
	issues []*importIssue
}

type importSource interface {
	name() string
	defaultPath(root string) string
	fixed() bool
	read(path string, res *ImportResult) (*importData, error)
}

var importSources = map[string]importSource{}

func registerImportSource(s importSource) {
	importSources[s.name()] = s
}

func ImportSources() []string {
	return []string{"beads", "linear", "jira"}
}
