package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tektite-tracker/tektite/internal/node"
	"github.com/tektite-tracker/tektite/parse"
)

const (
	DirName   = ".tektite"
	nodesDir  = "nodes"
	eventsDir = "events"
	extension = ".md"
)

var (
	ErrNoRepo = errors.New("no .tektite directory found")
	ErrExists = errors.New("node file already exists")
	ErrBadRef = errors.New("invalid node reference")
)

type Repo struct {
	Root string
}

type NodeFile struct {
	Ref       string
	Path      string
	Node      *parse.Node
	Diags     []parse.Diagnostic
	ParentRef string
	ParentErr error
}

type EventFile struct {
	Path  string
	Event *parse.Event
	Diags []parse.Diagnostic
}

type Orphan struct {
	Dir    string
	Events []*EventFile
}

type Snapshot struct {
	Nodes   []*NodeFile
	Events  map[string][]*EventFile
	Orphans []Orphan
	Tree    *Tree

	byRef map[string]*NodeFile
	refs  []string
}

func Discover(start string) (*Repo, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return nil, fmt.Errorf("resolve start directory: %w", err)
	}
	for {
		if isDir(filepath.Join(dir, DirName)) {
			return &Repo{Root: dir}, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, ErrNoRepo
		}
		dir = parent
	}
}

func Open(root string) (*Repo, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root: %w", err)
	}
	if !isDir(filepath.Join(abs, DirName)) {
		return nil, ErrNoRepo
	}
	return &Repo{Root: abs}, nil
}

func (r *Repo) Dir() string {
	return filepath.Join(r.Root, DirName)
}

func (r *Repo) NodesDir() string {
	return filepath.Join(r.Dir(), nodesDir)
}

func (r *Repo) EventsDir() string {
	return filepath.Join(r.Dir(), eventsDir)
}

func (r *Repo) Rel(path string) string {
	rel, err := filepath.Rel(r.Root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func (r *Repo) Load() (*Snapshot, error) {
	nodes, err := r.loadNodes()
	if err != nil {
		return nil, err
	}
	s := &Snapshot{Nodes: nodes, Events: map[string][]*EventFile{}, byRef: map[string]*NodeFile{}}
	for _, n := range nodes {
		s.byRef[n.Ref] = n
		s.refs = append(s.refs, n.Ref)
	}
	if err := r.loadEvents(s); err != nil {
		return nil, err
	}
	s.linkParents()
	return s, nil
}

func (r *Repo) loadNodes() ([]*NodeFile, error) {
	entries, err := readDir(r.NodesDir())
	if err != nil {
		return nil, err
	}
	out := []*NodeFile{}
	for _, e := range entries {
		if !isMarkdown(e) {
			continue
		}
		path := filepath.Join(r.NodesDir(), e.Name())
		src, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read node %s: %w", r.Rel(path), err)
		}
		n, diags := parse.ParseNode(e.Name(), src)
		out = append(out, &NodeFile{
			Ref:   strings.TrimSuffix(e.Name(), extension),
			Path:  r.Rel(path),
			Node:  n,
			Diags: diags,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ref < out[j].Ref })
	return out, nil
}

func (r *Repo) loadEvents(s *Snapshot) error {
	dirs, err := readDir(r.EventsDir())
	if err != nil {
		return err
	}
	for _, d := range dirs {
		if !d.IsDir() || strings.HasPrefix(d.Name(), ".") {
			continue
		}
		files, err := r.loadEventDir(filepath.Join(r.EventsDir(), d.Name()))
		if err != nil {
			return err
		}
		if ref, ok := s.eventOwner(d.Name()); ok {
			s.Events[ref] = append(s.Events[ref], files...)
			continue
		}
		s.Orphans = append(s.Orphans, Orphan{Dir: r.Rel(filepath.Join(r.EventsDir(), d.Name())), Events: files})
	}
	return nil
}

func (r *Repo) loadEventDir(dir string) ([]*EventFile, error) {
	entries, err := readDir(dir)
	if err != nil {
		return nil, err
	}
	out := []*EventFile{}
	for _, e := range entries {
		if !isMarkdown(e) {
			continue
		}
		path := filepath.Join(dir, e.Name())
		src, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read event %s: %w", r.Rel(path), err)
		}
		ev, diags := parse.ParseEvent(e.Name(), src)
		out = append(out, &EventFile{Path: r.Rel(path), Event: ev, Diags: diags})
	}
	return out, nil
}

func (s *Snapshot) eventOwner(dir string) (string, bool) {
	if _, ok := s.byRef[dir]; ok {
		return dir, true
	}
	ref, err := node.Resolve(s.refs, dir)
	return ref, err == nil
}

func (s *Snapshot) linkParents() {
	parents := map[string]string{}
	for _, n := range s.Nodes {
		parents[n.Ref] = ""
		if strings.TrimSpace(n.Node.Parent) == "" {
			continue
		}
		ref, err := node.Resolve(s.refs, n.Node.Parent)
		if err != nil {
			n.ParentErr = err
			continue
		}
		n.ParentRef = ref
		parents[n.Ref] = ref
	}
	s.Tree = buildTree(s.refs, parents)
}

func (s *Snapshot) Refs() []string {
	return append([]string{}, s.refs...)
}

func (s *Snapshot) Get(ref string) (*NodeFile, bool) {
	n, ok := s.byRef[ref]
	return n, ok
}

func (s *Snapshot) Resolve(query string) (*NodeFile, error) {
	ref, err := node.Resolve(s.refs, query)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", query, err)
	}
	return s.byRef[ref], nil
}

func (s *Snapshot) DuplicateIDs() map[string][]string {
	byID := map[string][]string{}
	for _, n := range s.Nodes {
		if n.Node.ID != "" {
			byID[n.Node.ID] = append(byID[n.Node.ID], n.Ref)
		}
	}
	for id, refs := range byID {
		if len(refs) < 2 {
			delete(byID, id)
		}
	}
	return byID
}

func (s *Snapshot) EventsOf(ref string) []*parse.Event {
	files := s.Events[ref]
	out := make([]*parse.Event, 0, len(files))
	for _, f := range files {
		out = append(out, f.Event)
	}
	return out
}

func (r *Repo) WriteNode(ref string, data []byte) (string, error) {
	if !validRef(ref) {
		return "", fmt.Errorf("write node %q: %w", ref, ErrBadRef)
	}
	return r.publishNew(r.NodesDir(), ref, data)
}

func (r *Repo) WriteEvent(ref, id string, data []byte) (string, error) {
	if !validRef(ref) || !validRef(id) {
		return "", fmt.Errorf("write event %q/%q: %w", ref, id, ErrBadRef)
	}
	return r.publishNew(filepath.Join(r.EventsDir(), ref), id, data)
}

func (r *Repo) publishNew(dir, name string, data []byte) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create %s: %w", r.Rel(dir), err)
	}
	final := filepath.Join(dir, name+extension)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return "", fmt.Errorf("create temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("write %q: %w", name, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("sync %q: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close %q: %w", name, err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return "", fmt.Errorf("set mode for %q: %w", name, err)
	}
	if err := os.Link(tmpName, final); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("write %q: %w", name, ErrExists)
		}
		return "", fmt.Errorf("publish %q: %w", name, err)
	}
	return r.Rel(final), nil
}

func validRef(ref string) bool {
	return ref != "" && !strings.HasPrefix(ref, ".") && !strings.ContainsAny(ref, `/\`) && !strings.ContainsRune(ref, 0)
}

func isMarkdown(e fs.DirEntry) bool {
	return e.Type().IsRegular() && !strings.HasPrefix(e.Name(), ".") && strings.HasSuffix(e.Name(), extension)
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func readDir(dir string) ([]fs.DirEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	return entries, nil
}

func Init(root string) (*Repo, bool, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, false, fmt.Errorf("resolve root: %w", err)
	}
	r := &Repo{Root: abs}
	created := false
	for _, dir := range []string{r.NodesDir(), r.EventsDir(), filepath.Join(r.Dir(), "blobs")} {
		if isDir(dir) {
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, false, fmt.Errorf("create %s: %w", r.Rel(dir), err)
		}
		created = true
	}
	config := filepath.Join(r.Dir(), "config")
	if _, err := os.Stat(config); errors.Is(err, fs.ErrNotExist) {
		if err := os.WriteFile(config, []byte("auto_commit: false\n"), 0o644); err != nil {
			return nil, false, fmt.Errorf("create %s: %w", r.Rel(config), err)
		}
		created = true
	}
	return r, created, nil
}

func (r *Repo) ReplaceFile(rel string, data []byte) error {
	path := filepath.Join(r.Root, filepath.FromSlash(rel))
	if !strings.HasPrefix(filepath.Clean(path), r.Dir()+string(filepath.Separator)) {
		return fmt.Errorf("replace %q: %w", rel, ErrBadRef)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write %s: %w", rel, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync %s: %w", rel, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", rel, err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return fmt.Errorf("set mode for %s: %w", rel, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace %s: %w", rel, err)
	}
	return nil
}

func (r *Repo) ReadFile(rel string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(r.Root, filepath.FromSlash(rel)))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", rel, err)
	}
	return data, nil
}
