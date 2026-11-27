package main

import (
	"crypto/rand"
	"io"
	"os"
	"time"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	os.Exit(run(os.Args[1:], env{
		in:          os.Stdin,
		out:         os.Stdout,
		err:         os.Stderr,
		cwd:         cwd,
		now:         time.Now,
		rand:        rand.Reader,
		getenv:      os.Getenv,
		interactive: interactive(os.Stdin, os.Stdout),
		gitUser:     gitUserName(cwd),
		gitHead:     gitHead(cwd),
		gitDirty:    gitDirty(cwd),
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
