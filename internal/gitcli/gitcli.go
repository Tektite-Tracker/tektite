package gitcli

import (
	"fmt"
	"os/exec"
	"strings"
)

type Git struct {
	Dir string
}

func (g Git) run(args ...string) ([]byte, error) {
	c := exec.Command("git", args...)
	c.Dir = g.Dir
	out, err := c.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w", args[0], err)
	}
	return out, nil
}

func (g Git) Head() string {
	out, err := g.run("rev-parse", "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (g Git) Dirty() bool {
	out, err := g.run("status", "--porcelain", "--", ".", ":(exclude).tektite")
	return err == nil && strings.TrimSpace(string(out)) != ""
}

func (g Git) CommitsSince(since string, paths []string) ([]string, error) {
	args := []string{"log", "--format=%h", since + "..HEAD", "--"}
	if len(paths) == 0 {
		paths = []string{"."}
	}
	out, err := g.run(append(append(args, paths...), ":(exclude).tektite")...)
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(out)), nil
}

func (g Git) User() string {
	out, err := g.run("config", "user.name")
	if err != nil {
		return ""
	}
	return string(out)
}
