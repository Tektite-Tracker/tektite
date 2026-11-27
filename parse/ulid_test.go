package parse_test

import (
	"strings"
	"testing"

	"github.com/tektite-tracker/tektite/parse"
)

func TestIsULID(t *testing.T) {
	t.Run("A ULID is 26 Crockford base32 characters", func(t *testing.T) {
		valid := []string{
			"01HZ3K8Q5W2X9N4T7V6B1C0D3F",
			"01hz3k8q5w2x9n4t7v6b1c0d3f",
			"7ZZZZZZZZZZZZZZZZZZZZZZZZZ",
			"00000000000000000000000000",
		}
		for _, s := range valid {
			if !parse.IsULID(s) {
				t.Errorf("%q should be a ULID", s)
			}
		}
		invalid := map[string]string{
			"empty":        "",
			"too short":    "01HZ3K8Q5W2X9N4T7V6B1C0D3",
			"too long":     "01HZ3K8Q5W2X9N4T7V6B1C0D3FA",
			"overflow":     "81HZ3K8Q5W2X9N4T7V6B1C0D3F",
			"letter I":     "01HZ3K8Q5W2X9N4T7V6B1C0DIF",
			"letter L":     "01HZ3K8Q5W2X9N4T7V6B1C0DLF",
			"letter O":     "01HZ3K8Q5W2X9N4T7V6B1C0DOF",
			"letter U":     "01HZ3K8Q5W2X9N4T7V6B1C0DUF",
			"dash":         "01HZ3K8Q5W2X9N4T7V6B1C0D-F",
			"space":        "01HZ3K8Q5W2X9N4T7V6B1C0D F",
			"multibyte":    strings.Repeat("é", 13),
			"slug and ref": "add-retry-on-429-3fa",
		}
		for name, s := range invalid {
			if parse.IsULID(s) {
				t.Errorf("%s: %q should not be a ULID", name, s)
			}
		}
	})
}

func FuzzIsULID(f *testing.F) {
	f.Add("01HZ3K8Q5W2X9N4T7V6B1C0D3F")
	f.Fuzz(func(t *testing.T, s string) {
		if parse.IsULID(s) && (len(s) != 26 || strings.ContainsAny(strings.ToUpper(s), "ILOU")) {
			t.Fatalf("%q accepted", s)
		}
	})
}
