package view

import (
	"strconv"
	"strings"
)

type PrimeStep struct {
	N    int    `json:"n"`
	Text string `json:"text"`
}

type PrimeExit struct {
	Code    int    `json:"code"`
	Meaning string `json:"meaning"`
}

type Briefing struct {
	Steps     []PrimeStep `json:"steps"`
	ExitCodes []PrimeExit `json:"exit_codes"`
}

var primeSteps = []string{
	"`tektite next --claim --actor agent:<model>/<session>` returns the best unblocked ticket and takes a 30 minute lease on it.",
	"`tektite context <ref>` returns the node, its ancestor specs, open gaps, recent evidence and links, trimmed to a token budget (`--budget N`).",
	"Do the work. `tektite heartbeat <ref>` extends the lease, `tektite note <ref> \"text\"` records progress, and `tektite new gap` records something left open.",
	"`tektite attach-evidence <ref> --run \"<command>\"` runs a command and stores its output, exit code and commit as evidence. `tektite close-gap <ref>#<block> --evidence <ref>` closes a gap with it.",
	"`tektite set-status <ref> done`. If a policy gate is not met the command exits 5 and says what is missing. An agent cannot override a gate.",
}

var primeExits = []PrimeExit{
	{0, "ok"},
	{1, "error"},
	{2, "usage"},
	{3, "not found"},
	{4, "conflict (someone else holds the lease)"},
	{5, "policy violation"},
}

func NewBriefing() Briefing {
	b := Briefing{Steps: []PrimeStep{}, ExitCodes: append([]PrimeExit{}, primeExits...)}
	for i, t := range primeSteps {
		b.Steps = append(b.Steps, PrimeStep{N: i + 1, Text: t})
	}
	return b
}

func (b Briefing) Text() string {
	var sb strings.Builder
	sb.WriteString("Tektite agent briefing. Run these in order:\n\n")
	for _, s := range b.Steps {
		sb.WriteString(itoa(s.N) + ". " + s.Text + "\n")
	}
	sb.WriteString("\nExit codes: ")
	for i, e := range b.ExitCodes {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(itoa(e.Code) + " " + e.Meaning)
	}
	sb.WriteString(".\n")
	return sb.String()
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
