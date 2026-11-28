package main

import (
	"fmt"
	"os/exec"
	"strings"
)

func gitHead(dir string) func() string {
	return func() string {
		c := exec.Command("git", "rev-parse", "HEAD")
		c.Dir = dir
		out, err := c.Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(out))
	}
}

func gitDirty(dir string) func() bool {
	return func() bool {
		c := exec.Command("git", "status", "--porcelain", "--", ".", ":(exclude).tektite")
		c.Dir = dir
		out, err := c.Output()
		return err == nil && strings.TrimSpace(string(out)) != ""
	}
}

func gitCommitsSince(dir string) func(string, []string) ([]string, error) {
	return func(since string, paths []string) ([]string, error) {
		args := []string{"log", "--format=%h", since + "..HEAD", "--"}
		if len(paths) == 0 {
			paths = []string{"."}
		}
		args = append(append(args, paths...), ":(exclude).tektite")
		c := exec.Command("git", args...)
		c.Dir = dir
		out, err := c.Output()
		if err != nil {
			return nil, fmt.Errorf("git log: %w", err)
		}
		return strings.Fields(string(out)), nil
	}
}

func gitUserName(dir string) func() string {
	return func() string {
		c := exec.Command("git", "config", "user.name")
		c.Dir = dir
		out, err := c.Output()
		if err != nil {
			return ""
		}
		return string(out)
	}
}
