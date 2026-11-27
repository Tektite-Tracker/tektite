package store

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const DefaultBlobCapMB = 5

type Config struct {
	ContextBudget int `yaml:"context_budget"`
	BlobCapMB     int `yaml:"blob_cap_mb"`
}

func (c Config) BlobCapBytes() int64 {
	mb := c.BlobCapMB
	if mb < 1 {
		mb = DefaultBlobCapMB
	}
	return int64(mb) << 20
}

func (r *Repo) Config() (Config, error) {
	var c Config
	data, err := os.ReadFile(filepath.Join(r.Dir(), "config"))
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(data, &c); err != nil {
		return c, fmt.Errorf("parse config: %w", err)
	}
	return c, nil
}
