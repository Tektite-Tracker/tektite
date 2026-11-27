package parse

import (
	"fmt"
	"strconv"
	"strings"
)

func StringOf(v any) string {
	return strings.TrimSpace(fmt.Sprint(v))
}

func IntOf(v any) (int, bool) {
	if v == nil {
		return 0, false
	}
	n, err := strconv.Atoi(StringOf(v))
	return n, err == nil
}
