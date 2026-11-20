package fixture

import "strings"

// Title upper-cases the first letter of every word; no other file repeats it.
func Title(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}
