package event

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/tektite-tracker/tektite/internal/node"
	"github.com/tektite-tracker/tektite/parse"
)

type Draft struct {
	Type   string
	Actor  string
	At     time.Time
	Fields []node.Field
	Body   string
}

func KnownStatus(status string) bool {
	return slices.Contains(Lifecycle, status)
}

func Render(d Draft) ([]byte, error) {
	if strings.TrimSpace(d.Type) == "" {
		return nil, fmt.Errorf("render event: type is empty")
	}
	fields := []node.Field{
		{Key: "type", Value: d.Type},
		{Key: "actor", Value: d.Actor},
		{Key: "at", Value: parse.FormatTime(d.At)},
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
