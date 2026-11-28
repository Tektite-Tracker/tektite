package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const archiveName = "archive"

func (r *Repo) ArchiveDir() string {
	return filepath.Join(r.Dir(), archiveName)
}

func (r *Repo) ArchiveNodesDir() string {
	return filepath.Join(r.ArchiveDir(), nodesDir)
}

func (r *Repo) ArchiveEventsDir() string {
	return filepath.Join(r.ArchiveDir(), eventsDir)
}

func (r *Repo) ArchiveBlobsDir() string {
	return filepath.Join(r.ArchiveDir(), "blobs")
}

func (r *Repo) BlobPath(name string) string {
	active := filepath.Join(r.BlobsDir(), name)
	if _, err := os.Stat(active); err == nil {
		return active
	}
	archived := filepath.Join(r.ArchiveBlobsDir(), name)
	if _, err := os.Stat(archived); err == nil {
		return archived
	}
	return active
}

type Move struct {
	Ref   string
	Blobs []string
}

func (r *Repo) Archive(moves []Move) error {
	return r.move(moves, r.NodesDir(), r.EventsDir(), r.BlobsDir(), r.ArchiveNodesDir(), r.ArchiveEventsDir(), r.ArchiveBlobsDir())
}

func (r *Repo) Unarchive(moves []Move) error {
	return r.move(moves, r.ArchiveNodesDir(), r.ArchiveEventsDir(), r.ArchiveBlobsDir(), r.NodesDir(), r.EventsDir(), r.BlobsDir())
}

func (r *Repo) move(moves []Move, fromNodes, fromEvents, fromBlobs, toNodes, toEvents, toBlobs string) error {
	for _, m := range moves {
		if !validRef(m.Ref) {
			return fmt.Errorf("move %q: %w", m.Ref, ErrBadRef)
		}
		for _, b := range m.Blobs {
			if !validRef(b) {
				return fmt.Errorf("move blob %q: %w", b, ErrBadRef)
			}
		}
		if _, err := os.Stat(filepath.Join(toNodes, m.Ref+extension)); err == nil {
			return fmt.Errorf("move %q: %w", m.Ref, ErrExists)
		}
		if _, err := os.Stat(filepath.Join(toEvents, m.Ref)); err == nil {
			return fmt.Errorf("move events of %q: %w", m.Ref, ErrExists)
		}
	}
	for _, m := range moves {
		if err := renameInto(filepath.Join(fromNodes, m.Ref+extension), toNodes, m.Ref+extension); err != nil {
			return err
		}
		if err := renameInto(filepath.Join(fromEvents, m.Ref), toEvents, m.Ref); err != nil {
			return err
		}
		for _, b := range m.Blobs {
			src := filepath.Join(fromBlobs, b)
			if _, err := os.Stat(filepath.Join(toBlobs, b)); err == nil {
				if err := os.Remove(src); err != nil && !errors.Is(err, fs.ErrNotExist) {
					return fmt.Errorf("remove %s: %w", src, err)
				}
				continue
			}
			if err := renameInto(src, toBlobs, b); err != nil {
				return err
			}
		}
	}
	return nil
}

func renameInto(src, dir, name string) error {
	if _, err := os.Stat(src); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	if err := os.Rename(src, filepath.Join(dir, name)); err != nil {
		return fmt.Errorf("move %s: %w", src, err)
	}
	return nil
}
