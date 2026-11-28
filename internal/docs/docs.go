package docs

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	pages "github.com/tektite-tracker/tektite/docs"
)

const (
	indexTopic = "index"
	suffix     = ".md"
)

var ErrNotFound = errors.New("unknown topic")

type Topic struct {
	Topic string `json:"topic"`
	Title string `json:"title"`
}

func List() ([]Topic, error) {
	return list(pages.FS)
}

func Read(topic string) (string, error) {
	return read(pages.FS, topic)
}

func list(fsys fs.FS) ([]Topic, error) {
	out := []Topic{}
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, suffix) {
			return nil
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return fmt.Errorf("read %s: %w", p, err)
		}
		name := strings.TrimSuffix(p, suffix)
		out = append(out, Topic{Topic: name, Title: title(string(data), name)})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list docs: %w", err)
	}
	sort.Slice(out, func(i, j int) bool {
		if (out[i].Topic == indexTopic) != (out[j].Topic == indexTopic) {
			return out[i].Topic == indexTopic
		}
		return out[i].Topic < out[j].Topic
	})
	return out, nil
}

func read(fsys fs.FS, topic string) (string, error) {
	topic = strings.TrimSuffix(strings.Trim(topic, "/"), suffix)
	if topic == "" || !fs.ValidPath(topic) || path.Ext(topic) != "" {
		return "", fmt.Errorf("%w %q", ErrNotFound, topic)
	}
	data, err := fs.ReadFile(fsys, topic+suffix)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("%w %q", ErrNotFound, topic)
		}
		return "", fmt.Errorf("read topic %s: %w", topic, err)
	}
	return string(data), nil
}

func title(body, fallback string) string {
	for _, line := range strings.Split(body, "\n") {
		if t, ok := strings.CutPrefix(line, "# "); ok {
			return strings.TrimSpace(t)
		}
	}
	return fallback
}
