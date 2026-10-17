package domain

import "unicode"

// HasIndexableText reports whether text holds anything an index can match: a
// letter or a digit in any script.
//
// It is the one rule both retrieval stages share. The lexical stage drops a token
// that fails it, because quoting it would leave a phrase with nothing to look for.
// The semantic stage checks the whole query before embedding it, because a query
// of only punctuation has no meaning to embed — and answering it with a semantic
// neighbour would break the guarantee that such a query returns no results.
func HasIndexableText(text string) bool {
	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}
