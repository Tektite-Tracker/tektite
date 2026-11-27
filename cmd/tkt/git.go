package main

import (
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
