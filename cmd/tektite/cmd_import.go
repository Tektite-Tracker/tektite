package main

import (
	"bufio"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/tektite-tracker/tektite/internal/service"
)

func (a *app) importCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import issues from another tracker's file export, once and one way",
	}
	cmd.AddCommand(
		a.importSourceCmd("beads", "Import a Beads issues.jsonl export into nodes", "path to the beads export (default .beads/issues.jsonl next to .tektite/)"),
	)
	return cmd
}

func (a *app) importSourceCmd(source, short, fromHelp string) *cobra.Command {
	var from, actor, mapFile string
	var dry, yes bool
	cmd := &cobra.Command{
		Use:   source,
		Short: short,
		Args:  exactArgs(0),
		RunE: func(_ *cobra.Command, _ []string) error {
			svc, err := a.svc()
			if err != nil {
				return err
			}
			in := service.ImportInput{Source: source, Path: from, DryRun: dry, Actor: actor, Yes: yes, MapFile: mapFile}
			if a.interactive && !yes && mapFile == "" {
				reader := bufio.NewReader(a.in)
				in.Ask = func(q service.ImportQuestion) (string, error) { return a.askMapping(reader, q) }
			}
			res, err := svc.Import(in)
			if err != nil {
				return err
			}
			return a.emit(res, func() {
				if dry && res.Mapping != nil {
					a.printMapping(res.Mapping)
				}
				for _, it := range res.Items {
					verb := it.Action
					if verb == "create" && res.DryRun {
						verb = "would create"
					}
					a.printf(a.out, "%s %s %s %s\n", verb, it.SourceID, it.Kind, it.Title)
				}
				a.printf(a.out, "%d created, %d skipped, %d links\n", res.Created, res.Skipped, res.Links)
				if res.MappingNew {
					a.printf(a.out, "saved the mapping to %s\n", res.MappingPath)
				}
				a.warn(warningTexts(res.Warnings))
			})
		},
	}
	cmd.Flags().StringVar(&from, "from", "", fromHelp)
	cmd.Flags().BoolVar(&dry, "dry-run", false, "print the proposed mapping and what would be imported without writing")
	cmd.Flags().StringVar(&actor, "actor", "", "actor recorded on imported events")
	cmd.Flags().BoolVar(&yes, "yes", false, "accept the proposed mapping for ambiguous values without asking")
	cmd.Flags().StringVar(&mapFile, "map", "", "mapping file to use instead of asking or the saved mapping")
	return cmd
}

func (a *app) printMapping(m *service.ImportMapping) {
	a.printf(a.out, "mapping:\n")
	for _, facet := range []struct {
		name string
		m    map[string]string
	}{{"status", m.Status}, {"priority", m.Priority}, {"type", m.Type}} {
		keys := make([]string, 0, len(facet.m))
		for k := range facet.m {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			a.printf(a.out, "  %s %q -> %s\n", facet.name, k, facet.m[k])
		}
	}
}

func (a *app) askMapping(r *bufio.Reader, q service.ImportQuestion) (string, error) {
	noun := "issues"
	if q.Count == 1 {
		noun = "issue"
	}
	for {
		hint := ""
		if q.Guess != "" {
			hint = fmt.Sprintf(", press enter for %s", q.Guess)
		}
		a.printf(a.err, "%s %q (%d %s, %s) maps to [%s]%s: ", q.Facet, q.Value, q.Count, noun, q.Reason, strings.Join(q.Options, "/"), hint)
		line, err := r.ReadString('\n')
		answer := strings.TrimSpace(line)
		if answer == "" && q.Guess != "" {
			return q.Guess, nil
		}
		if answer != "" {
			return answer, nil
		}
		if err != nil {
			return "", usage("no answer for %s %q", q.Facet, q.Value)
		}
	}
}
