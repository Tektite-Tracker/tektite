package parse

import "strings"

// RelMentions is the relation of an untyped wikilink.
const RelMentions = "mentions"

// LinkTypes lists the built-in link types in a stable order.
var LinkTypes = []string{"implements", "blocks", "evidences", "supersedes", "derived-from"}

var inverses = map[string]string{
	"implements":   "implemented-by",
	"blocks":       "blocked-by",
	"evidences":    "evidenced-by",
	"supersedes":   "superseded-by",
	"derived-from": "source-of",
	RelMentions:    "mentioned-by",
}

// Inverse returns the relation seen from the target of a link of type rel. Unknown types
// get a generic inverse so that they still show up as backlinks.
func Inverse(rel string) string {
	rel = strings.ToLower(strings.TrimSpace(rel))
	if inv, ok := inverses[rel]; ok {
		return inv
	}
	if orig, ok := strings.CutPrefix(rel, "inverse-of:"); ok {
		return orig
	}
	for k, v := range inverses {
		if v == rel {
			return k
		}
	}
	return "inverse-of:" + rel
}

// IsBuiltinLink reports whether rel is a built-in link type or the inverse of one.
func IsBuiltinLink(rel string) bool {
	rel = strings.ToLower(strings.TrimSpace(rel))
	if _, ok := inverses[rel]; ok {
		return true
	}
	for _, v := range inverses {
		if v == rel {
			return true
		}
	}
	return false
}

func frontmatterRel(key string) bool {
	key = strings.ToLower(key)
	if key == RelMentions || key == "mentioned-by" {
		return false
	}
	return IsBuiltinLink(key)
}
