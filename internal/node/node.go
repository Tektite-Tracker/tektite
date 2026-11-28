package node

import "strings"

const (
	PriorityCritical = "critical"
	PriorityHigh     = "high"
	PriorityMedium   = "medium"
	PriorityLow      = "low"
)

var Priorities = []string{PriorityCritical, PriorityHigh, PriorityMedium, PriorityLow}

func NormalizePriority(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func ValidPriority(v string) bool {
	n := NormalizePriority(v)
	for _, p := range Priorities {
		if p == n {
			return true
		}
	}
	return false
}
