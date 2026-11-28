package main

import (
	"crypto/rand"
	"io"
	"os"
	"time"

	"github.com/tektite-tracker/tektite/internal/gitcli"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	git := gitcli.Git{Dir: cwd}
	os.Exit(run(os.Args[1:], env{
		in:          os.Stdin,
		out:         os.Stdout,
		err:         os.Stderr,
		cwd:         cwd,
		now:         time.Now,
		rand:        rand.Reader,
		getenv:      os.Getenv,
		interactive: interactive(os.Stdin, os.Stdout),
		gitUser:     git.User,
		gitHead:     git.Head,
		gitDirty:    git.Dirty,
		gitLog:      git.CommitsSince,
	}))
}

func interactive(in io.Reader, out io.Writer) bool {
	return isTerminal(in) && isTerminal(out)
}

func isTerminal(v any) bool {
	f, ok := v.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
