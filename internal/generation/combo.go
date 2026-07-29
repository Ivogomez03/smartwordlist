package generation

import (
	"strings"
)

// GenerateCombos produces password candidates by combining dictionary words
// with contextual words and applying the provided mutation function to each
// combination.
//
// Combinations built:
//   - dictWord + contextWord
//   - contextWord + dictWord
//
// The mutate function is then called on every combination, and all results
// are deduplicated before returning.
//
// dictWords and contextWords are expected to already be clean (lowercase,
// trimmed).  Both may be nil or empty — in that case only the non-empty
// side's mutations are used, which means nothing if both are empty.
func GenerateCombos(dictWords []string, contextWords []string, mutate func(string) []string) []string {
	if len(dictWords) == 0 && len(contextWords) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var out []string

	emit := func(w string) {
		w = strings.TrimSpace(w)
		if w == "" || seen[w] {
			return
		}
		seen[w] = true
		out = append(out, w)
	}

	// Build base combos: dictWord + contextWord and contextWord + dictWord.
	for _, dw := range dictWords {
		for _, cw := range contextWords {
			emit(dw + cw)
			emit(cw + dw)
		}
	}

	// If one side is empty, at least mutate the non-empty side so we still
	// produce candidates from the available words.
	if len(contextWords) == 0 {
		for _, dw := range dictWords {
			for _, m := range mutate(dw) {
				emit(m)
			}
		}
		return out
	}

	if len(dictWords) == 0 {
		for _, cw := range contextWords {
			for _, m := range mutate(cw) {
				emit(m)
			}
		}
		return out
	}

	// Build a set of base combinations plus the raw words, then mutate once
	// per unique base to avoid duplicate work.
	bases := make(map[string]bool, len(dictWords)*len(contextWords)*2)
	for _, dw := range dictWords {
		bases[dw] = true
		for _, cw := range contextWords {
			bases[dw+cw] = true
			bases[cw+dw] = true
		}
	}
	for _, cw := range contextWords {
		bases[cw] = true
	}

	for base := range bases {
		for _, m := range mutate(base) {
			emit(m)
		}
	}

	return out
}
