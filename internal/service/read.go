package service

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/tektite-tracker/tektite/internal/bundle"
	"github.com/tektite-tracker/tektite/internal/check"
	"github.com/tektite-tracker/tektite/internal/store"
	"github.com/tektite-tracker/tektite/internal/view"
)

func (s *Service) Show(ref string) (view.ShowDoc, error) {
	snap, err := s.load()
	if err != nil {
		return view.ShowDoc{}, err
	}
	nf, err := snap.Resolve(ref)
	if err != nil {
		return view.ShowDoc{}, fmt.Errorf("show: %w", err)
	}
	return view.BuildShow(snap, nf, s.Now()), nil
}

type ListInput struct {
	Kind   string
	Status string
	Under  string
	All    bool
}

func (s *Service) List(in ListInput) ([]view.ListItem, error) {
	snap, err := s.load()
	if err != nil {
		return nil, err
	}
	filter := view.ListFilter{Kind: in.Kind, Status: in.Status, All: in.All}
	if in.Under != "" {
		root, err := snap.Resolve(in.Under)
		if err != nil {
			return nil, fmt.Errorf("--under: %w", err)
		}
		filter.UnderRef = root.Ref
	}
	return view.List(snap, filter, s.Now()), nil
}

func (s *Service) Context(ref string, budget int) (*bundle.Bundle, error) {
	snap, err := s.load()
	if err != nil {
		return nil, err
	}
	nf, err := snap.Resolve(ref)
	if err != nil {
		return nil, fmt.Errorf("context: %w", err)
	}
	if budget < 1 {
		cfg, err := s.Repo.Config()
		if err != nil {
			return nil, fmt.Errorf("context: %w", err)
		}
		budget = cfg.ContextBudget
	}
	return bundle.Build(snap, nf, bundle.Options{Budget: budget, Head: s.git().Head(), BlobSize: s.blobSize}), nil
}

func (s *Service) blobSize(attachment string) (int64, bool) {
	name := store.BlobName(attachment)
	if name == "" {
		return 0, false
	}
	info, err := os.Stat(filepath.Join(s.Repo.BlobsDir(), name))
	if err != nil {
		return 0, false
	}
	return info.Size(), true
}

func (s *Service) Check(stale bool) (check.Report, error) {
	rep, err := check.RunWith(s.Repo, check.Options{Stale: stale, Log: s.git().CommitsSince})
	if err != nil {
		return check.Report{}, fmt.Errorf("check: %w", err)
	}
	return rep, nil
}
