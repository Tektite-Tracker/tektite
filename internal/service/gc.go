package service

import (
	"fmt"
	"os"
	"slices"
	"strings"
)

type GCResult struct {
	Removed []string
	DryRun  bool
}

func (s *Service) GCBlobs(dryRun bool) (*GCResult, error) {
	snap, err := s.load()
	if err != nil {
		return nil, err
	}
	referenced := snap.BlobRefs()
	entries, err := os.ReadDir(s.Repo.BlobsDir())
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("gc: read blobs: %w", err)
	}
	res := &GCResult{Removed: []string{}, DryRun: dryRun}
	for _, e := range entries {
		if !e.Type().IsRegular() || strings.HasPrefix(e.Name(), ".") || len(referenced[e.Name()]) > 0 {
			continue
		}
		res.Removed = append(res.Removed, e.Name())
	}
	slices.Sort(res.Removed)
	if dryRun {
		return res, nil
	}
	for _, name := range res.Removed {
		if err := s.Repo.RemoveBlob(name); err != nil {
			return nil, fmt.Errorf("gc: %w", err)
		}
	}
	return res, nil
}
