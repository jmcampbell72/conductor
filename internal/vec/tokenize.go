// Text tokenizer for TF-IDF embedding. Lowercases, splits on non-alphanumeric
// characters, and drops stopwords and tokens shorter than 3 characters.
//
// Author: Justin Campbell
package vec

import (
	"strings"
	"unicode"
)

var stopwords = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "or": true,
	"but": true, "in": true, "on": true, "at": true, "to": true,
	"for": true, "of": true, "with": true, "by": true, "from": true,
	"is": true, "it": true, "its": true, "be": true, "as": true,
	"was": true, "are": true, "were": true, "been": true, "has": true,
	"have": true, "had": true, "do": true, "does": true, "did": true,
	"will": true, "would": true, "can": true, "could": true, "may": true,
	"might": true, "shall": true, "should": true, "that": true, "this": true,
	"these": true, "those": true, "i": true, "we": true, "you": true,
	"he": true, "she": true, "they": true, "me": true, "us": true,
	"him": true, "her": true, "them": true, "my": true, "our": true,
	"your": true, "his": true, "their": true, "what": true, "which": true,
	"who": true, "when": true, "where": true, "how": true, "all": true,
	"not": true, "no": true, "so": true, "if": true, "then": true,
	"get": true, "use": true, "just": true, "like": true, "also": true,
	"into": true, "out": true, "up": true, "about": true, "more": true,
}

// tokenize lowercases text, splits on non-alphanumeric characters,
// and drops stopwords and tokens shorter than 3 characters.
func tokenize(text string) []string {
	text = strings.ToLower(text)
	raw := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	out := raw[:0]
	for _, t := range raw {
		if len(t) >= 3 && !stopwords[t] {
			out = append(out, t)
		}
	}
	return out
}
