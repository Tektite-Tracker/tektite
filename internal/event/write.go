package event

import (
	"fmt"
	"strings"
	"time"

	"github.com/tektite-tracker/tektite/internal/node"
)

type Draft struct {
	Type   string
	Actor  string
	At     time.Time
	Fields []node.Field
	Body   string
}

func KnownStatus(status string) bool {
	return knownStatuses[status]
}

func Render(d Draft) ([]byte, error) {
	if strings.TrimSpace(d.Type) == "" {
		return nil, fmt.Errorf("render event: type is empty")
	}
	fields := []node.Field{
		{Key: "type", Value: d.Type},
		{Key: "actor", Value: d.Actor},
		{Key: "at", Value: d.At.UTC().Format(time.RFC3339)},
	}
	fields = append(fields, d.Fields...)
	body := strings.TrimSpace(d.Body)
	if body != "" {
		body += "\n"
	}
	data, err := node.Render(fields, body)
	if err != nil {
		return nil, fmt.Errorf("render event: %w", err)
	}
	return data, nil
}
