// Package generation produces contextual password candidates through LLM
// prompting and rule-based mutation engines.
package generation

import (
	"fmt"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// MutationRules — loaded from YAML
// ---------------------------------------------------------------------------

// YearRange defines the inclusive start and end for year mutations.
type YearRange struct {
	Start int `yaml:"start" toml:"start"`
	End   int `yaml:"end" toml:"end"`
}

// MutationRules holds all configured mutation parameters read from a YAML
// rules file (e.g. defaults/rules.yaml).
type MutationRules struct {
	LeetMap        map[string][]string `yaml:"leet_map" toml:"leet_map"`
	Suffixes       []string            `yaml:"suffixes" toml:"suffixes"`
	Prefixes       []string            `yaml:"prefixes" toml:"prefixes"`
	YearRange      YearRange           `yaml:"year_range" toml:"year_range"`
	CaseVariations []string            `yaml:"case_variations" toml:"case_variations"`
}

// LoadRules reads and parses a MutationRules YAML file.  Returns the
// populated struct or an error describing the failure.
func LoadRules(path string) (*MutationRules, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load rules: %w", err)
	}

	var rules MutationRules
	if err := yaml.Unmarshal(data, &rules); err != nil {
		return nil, fmt.Errorf("load rules: parse YAML: %w", err)
	}

	return &rules, nil
}

// ---------------------------------------------------------------------------
// MutationEngine
// ---------------------------------------------------------------------------

// MutationEngine applies leet, case, suffix, prefix, and year mutations to a
// base word.  It implements the MutationEngine interface from the pipeline
// design.
//
// Mutate returns a deduplicated slice of mutated variants.  Leet variants
// are computed via a cartesian product capped at maxLeetVariants.
type MutationEngine struct {
	rules *MutationRules
}

// NewMutationEngine returns a ready-to-use engine configured with the given
// rules.  rules must not be nil.
func NewMutationEngine(rules *MutationRules) *MutationEngine {
	return &MutationEngine{rules: rules}
}

const maxLeetVariants = 200 // safety cap on combinatorial explosion

// Mutate applies every configured mutation to word and returns a
// deduplicated set of results.  The mutations applied are:
//
//  1. Original word
//  2. Leet-substitution variants (cartesian product, capped)
//  3. Case variants (lower, UPPER, Title) on the original word
//  4. Original + each suffix
//  5. Each prefix + original
//  6. Original + each year in [start, end]
//  7. Each case variant + each suffix
//  8. Each case variant + each year
func (me *MutationEngine) Mutate(word string) []string {
	seen := make(map[string]bool)
	var out []string

	add := func(w string) {
		w = strings.TrimSpace(w)
		if w == "" {
			return
		}
		if seen[w] {
			return
		}
		seen[w] = true
		out = append(out, w)
	}

	// 1. Original word
	add(word)

	// 2. Leet variants
	for _, v := range me.leetVariants(word) {
		add(v)
	}

	// 3. Case variants (original word only — not chained with leet)
	lower := strings.ToLower(word)
	upper := strings.ToUpper(word)
	title := toTitle(word)
	add(lower)
	add(upper)
	if title != lower && title != upper {
		add(title)
	}

	// Prepare base words to which we append suffix/year.
	bases := []string{word, lower, upper, title}
	seenBases := map[string]bool{word: true, lower: true, upper: true, title: true}
	for _, b := range bases {
		if b == "" || seenBases[b] {
			continue
		}
		seenBases[b] = true
	}

	// 4. Suffix on original
	for _, s := range me.rules.Suffixes {
		add(word + s)
	}

	// 5. Prefix on original
	for _, p := range me.rules.Prefixes {
		add(p + word)
	}

	// 6. Year suffix on original
	for y := me.rules.YearRange.Start; y <= me.rules.YearRange.End; y++ {
		add(fmt.Sprintf("%s%d", word, y))
	}

	// 7. Case variants + suffix
	for _, base := range bases {
		for _, s := range me.rules.Suffixes {
			add(base + s)
		}
	}

	// 8. Case variants + year
	for _, base := range bases {
		for y := me.rules.YearRange.Start; y <= me.rules.YearRange.End; y++ {
			add(fmt.Sprintf("%s%d", base, y))
		}
	}

	return out
}

// ---------------------------------------------------------------------------
// Leet substitution
// ---------------------------------------------------------------------------

// leetVariants builds all leet-substitution variants of word via the
// cartesian product of per-character alternatives defined in the leet map.
// The result is capped at maxLeetVariants to avoid combinatorial explosion
// on long words with many leet-capable characters.
func (me *MutationEngine) leetVariants(word string) []string {
	if len(me.rules.LeetMap) == 0 {
		return nil
	}

	// Build a per-position list of alternatives (original char + leet aliases).
	rowWord := []rune(word)
	positions := make([][]rune, len(rowWord))

	hasLeet := false
	for i, r := range rowWord {
		ch := string(unicode.ToLower(r))
		alts, ok := me.rules.LeetMap[ch]
		if !ok || len(alts) == 0 {
			positions[i] = []rune{r}
			continue
		}
		hasLeet = true
		// First entry is always the original character.
		opts := make([]rune, 1, 1+len(alts))
		opts[0] = r
		for _, a := range alts {
			ra, _ := utf8.DecodeRuneInString(a)
			if ra != utf8.RuneError && ra != 0 {
				opts = append(opts, ra)
			}
		}
		positions[i] = opts
	}

	if !hasLeet {
		return nil
	}

	// Calculate total product size.
	total := 1
	for _, p := range positions {
		total *= len(p)
		if total > maxLeetVariants {
			total = maxLeetVariants
			break
		}
	}

	// Backtracking cartesian product.
	variants := make([]string, 0, total)
	buf := make([]rune, len(word))
	var backtrack func(idx int)
	backtrack = func(idx int) {
		if idx == len(positions) {
			variants = append(variants, string(buf))
			return
		}
		for _, r := range positions[idx] {
			if len(variants) >= maxLeetVariants {
				return
			}
			buf[idx] = r
			backtrack(idx + 1)
		}
	}
	backtrack(0)

	return variants
}

// toTitle capitalises the first rune of s and lower-cases the rest.  It
// handles the case where s is empty or the first rune has no title mapping
// (non-letter characters pass through unchanged).
func toTitle(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	for i := 1; i < len(runes); i++ {
		runes[i] = unicode.ToLower(runes[i])
	}
	return string(runes)
}
