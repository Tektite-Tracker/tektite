package node

import (
	"cmp"
	"slices"
)

func Ancestors(parents map[string]string, id string) ([]string, bool) {
	seen := map[string]bool{id: true}
	out := []string{}
	for cur := parents[id]; cur != ""; cur = parents[cur] {
		if seen[cur] {
			return out, true
		}
		seen[cur] = true
		out = append(out, cur)
	}
	return out, false
}

func Cycles(parents map[string]string) [][]string {
	ids := make([]string, 0, len(parents))
	for id := range parents {
		ids = append(ids, id)
	}
	slices.Sort(ids)

	done := map[string]bool{}
	cycles := [][]string{}
	for _, start := range ids {
		path := []string{}
		onPath := map[string]int{}
		for cur := start; cur != "" && !done[cur]; cur = parents[cur] {
			if i, ok := onPath[cur]; ok {
				cycles = append(cycles, rotate(path[i:]))
				break
			}
			onPath[cur] = len(path)
			path = append(path, cur)
		}
		for _, p := range path {
			done[p] = true
		}
	}
	slices.SortFunc(cycles, func(a, b []string) int { return cmp.Compare(a[0], b[0]) })
	return cycles
}

func rotate(cycle []string) []string {
	lo := 0
	for i, id := range cycle {
		if id < cycle[lo] {
			lo = i
		}
	}
	return append(append([]string{}, cycle[lo:]...), cycle[:lo]...)
}
