package event

import (
	"fmt"
	"strings"
)

type diffOp struct {
	kind byte
	text string
}

const diffContext = 3

func UnifiedDiff(aName, bName, a, b string) string {
	as, bs := strings.Split(a, "\n"), strings.Split(b, "\n")
	ops := diffOps(as, bs)
	changed := false
	for _, op := range ops {
		if op.kind != ' ' {
			changed = true
			break
		}
	}
	if !changed {
		return ""
	}
	var out strings.Builder
	fmt.Fprintf(&out, "--- %s\n+++ %s\n", aName, bName)
	keep := make([]bool, len(ops))
	for i, op := range ops {
		if op.kind == ' ' {
			continue
		}
		for j := max(0, i-diffContext); j <= min(len(ops)-1, i+diffContext); j++ {
			keep[j] = true
		}
	}
	aLine, bLine := 0, 0
	for i := 0; i < len(ops); {
		if !keep[i] {
			if ops[i].kind != '+' {
				aLine++
			}
			if ops[i].kind != '-' {
				bLine++
			}
			i++
			continue
		}
		aStart, bStart := aLine, bLine
		aCount, bCount := 0, 0
		var body strings.Builder
		for ; i < len(ops) && keep[i]; i++ {
			body.WriteString(string(ops[i].kind) + ops[i].text + "\n")
			if ops[i].kind != '+' {
				aCount++
				aLine++
			}
			if ops[i].kind != '-' {
				bCount++
				bLine++
			}
		}
		if aCount > 0 {
			aStart++
		}
		if bCount > 0 {
			bStart++
		}
		fmt.Fprintf(&out, "@@ -%d,%d +%d,%d @@\n%s", aStart, aCount, bStart, bCount, body.String())
	}
	return out.String()
}

func diffOps(a, b []string) []diffOp {
	n, m := len(a), len(b)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var ops []diffOp
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, diffOp{' ', a[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			ops = append(ops, diffOp{'-', a[i]})
			i++
		default:
			ops = append(ops, diffOp{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, diffOp{'-', a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, diffOp{'+', b[j]})
	}
	return ops
}
