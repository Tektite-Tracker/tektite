package check

import (
	"fmt"
	"strings"

	"github.com/tektite-tracker/tektite/internal/store"
	"github.com/tektite-tracker/tektite/parse"
)

func (c *run) staleEvidence() {
	if !c.opts.Stale || c.opts.Log == nil {
		return
	}
	for _, nf := range c.snap.Nodes {
		if nf.Node.Kind != parse.KindEvidence {
			continue
		}
		commit := store.EvidenceOfNode(nf).Commit
		if commit == "" {
			continue
		}
		commits, err := c.opts.Log(commit, relatedFiles(nf))
		if err != nil || len(commits) == 0 {
			continue
		}
		c.add(parse.SeverityWarning, CodeEvidenceStale, nf.Path, 0, fmt.Sprintf("%s is possibly stale: recorded at %s, %d later commits touched related files (%s)", nf.Ref, commit, len(commits), strings.Join(commits, ", ")))
	}
}

func relatedFiles(nf *store.NodeFile) []string {
	files := []string{}
	if list, ok := nf.Node.Fields["files"].([]any); ok {
		for _, f := range list {
			if s := parse.StringOf(f); s != "" {
				files = append(files, s)
			}
		}
	}
	return files
}
