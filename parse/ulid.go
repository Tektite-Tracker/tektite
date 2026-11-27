package parse

import "strings"

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// IsULID reports whether s is a 26 character Crockford base32 ULID.
func IsULID(s string) bool {
	if len(s) != 26 {
		return false
	}
	s = strings.ToUpper(s)
	if s[0] > '7' {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !strings.ContainsRune(crockford, rune(s[i])) {
			return false
		}
	}
	return true
}
