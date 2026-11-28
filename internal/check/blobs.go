package check

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/tektite-tracker/tektite/internal/store"
	"github.com/tektite-tracker/tektite/parse"
)

var blobFile = regexp.MustCompile(`^([0-9a-f]{64})(\.[A-Za-z0-9]+)?$`)

func (c *run) evidenceCommit(nf *store.NodeFile) {
	if nf.Node.Kind != parse.KindEvidence {
		return
	}
	if store.EvidenceOfNode(nf).Commit == "" {
		c.add(parse.SeverityWarning, CodeEvidenceNoCommit, nf.Path, 0, fmt.Sprintf("%s is evidence that cannot be tied to a code version: provenance has no commit", nf.Ref))
	}
}

func (c *run) blobs() {
	if c.repo == nil {
		return
	}
	refs := c.snap.BlobRefs()
	names := make([]string, 0, len(refs))
	for name := range refs {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		path := c.repo.BlobPath(name)
		rel := c.repo.Rel(path)
		users := strings.Join(refs[name], ", ")
		data, err := os.ReadFile(path)
		if err != nil {
			for _, ref := range refs[name] {
				nf, _ := c.snap.Get(ref)
				c.add(parse.SeverityWarning, CodeMissingBlob, nf.Path, 0, fmt.Sprintf("%s references blob %s, which does not exist", ref, rel))
			}
			continue
		}
		if m := blobFile.FindStringSubmatch(name); m != nil {
			sum := sha256.Sum256(data)
			if got := hex.EncodeToString(sum[:]); got != m[1] {
				c.add(parse.SeverityError, CodeBlobTampered, rel, 0, fmt.Sprintf("blob %s no longer matches its hash name (content hashes to %s), referenced by %s", rel, got, users))
			}
		}
	}
	entries, err := os.ReadDir(c.repo.BlobsDir())
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.Type().IsRegular() || strings.HasPrefix(e.Name(), ".") || len(refs[e.Name()]) > 0 {
			continue
		}
		c.add(parse.SeverityNote, CodeUnreferencedBlob, c.repo.Rel(filepath.Join(c.repo.BlobsDir(), e.Name())), 0, fmt.Sprintf("blob %s is not referenced by any node", e.Name()))
	}
}
