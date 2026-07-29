package generation

import (
	"context"
	"fmt"
	"strings"

	"github.com/Ivogomez03/smartwordlist/pkg/types"
)

// RuleGenerator produces password candidates purely through rule-based
// mutation of reconnaissance data — no LLM involved.  It implements the
// CandidateGenerator interface and serves as the automatic fallback when
// Ollama is unavailable or --no-llm is set.
type RuleGenerator struct {
	recon  *types.ReconResult
	engine *MutationEngine
}

// NewRuleGenerator returns a RuleGenerator pre-configured with the
// reconnaissance data and mutation engine.
func NewRuleGenerator(recon *types.ReconResult, engine *MutationEngine) *RuleGenerator {
	return &RuleGenerator{recon: recon, engine: engine}
}

// Generate extracts context words from the stored ReconResult, mutates
// each one through the mutation engine, collects base years as words,
// and returns up to max deduplicated candidates.
//
// Context words are taken from: Company, Technologies, Keywords, and
// any path segments that look meaningful.  The year range from the
// rules is also used to generate raw-year candidates (e.g. "2026").
func (rg *RuleGenerator) Generate(ctx context.Context, chunks []types.ScoredChunk, max int) ([]types.Candidate, error) {
	// Collect candidate base words from recon data.
	words := rg.collectWords()

	seen := make(map[string]bool)
	var candidates []types.Candidate

	emit := func(word string, source string) bool {
		if word == "" || seen[word] {
			return false
		}
		seen[word] = true
		candidates = append(candidates, types.Candidate{Word: word, Source: source})
		return max > 0 && len(candidates) >= max
	}

	// Mutate every context word.
	for _, w := range words {
		for _, mutated := range rg.engine.Mutate(w) {
			if emit(mutated, "rule-mutation") {
				return candidates[:max], nil
			}
		}
	}

	// Also generate raw years and year variants as base words (e.g. "2026",
	// "2026!").
	for y := rg.engine.rules.YearRange.Start; y <= rg.engine.rules.YearRange.End; y++ {
		ys := fmt.Sprintf("%d", y)
		for _, mutated := range rg.engine.Mutate(ys) {
			if emit(mutated, "rule-year") {
				return candidates[:max], nil
			}
		}
	}

	if max > 0 && len(candidates) > max {
		candidates = candidates[:max]
	}

	return candidates, nil
}

// collectWords extracts meaningful base words from the reconnaissance
// result: company name, technologies, keywords, subdomains, and path
// segments.
func (rg *RuleGenerator) collectWords() []string {
	var words []string

	add := func(s string) {
		s = cleanWord(s)
		if s != "" {
			words = append(words, s)
		}
	}

	r := rg.recon
	if r == nil {
		return words
	}

	add(r.Company)
	// Also split multi-word company names into individual tokens.
	for _, part := range strings.Fields(r.Company) {
		add(part)
	}
	for _, t := range r.Technologies {
		add(t)
	}
	for _, k := range r.Keywords {
		add(k)
	}
	for _, sd := range r.Subdomains {
		// Only use the subdomain prefix, not the full FQDN.
		prefix, _, found := strings.Cut(sd, ".")
		if found && prefix != "" && prefix != "www" {
			add(prefix)
		}
	}
	// Extract meaningful segments from path fragments.
	for _, p := range r.Paths {
		segments := splitPath(p)
		for _, seg := range segments {
			add(seg)
		}
	}

	return dedupeSlice(words)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// cleanWord trims, lowercases, and removes non-alphanumeric characters
// from the edges of a word so it is suitable as a base for mutation.
func cleanWord(s string) string {
	s = trimNonAlphaNumeric(s)
	if s == "" {
		return ""
	}
	return s
}

// trimNonAlphaNumeric removes leading and trailing characters that are not
// letters or digits.
func trimNonAlphaNumeric(s string) string {
	if s == "" {
		return s
	}

	start := 0
	for start < len(s) {
		r, size := decodeFirst(s[start:])
		if isAlphaNumeric(r) {
			break
		}
		start += size
	}

	end := len(s)
	for end > start {
		r, size := decodeLast(s[:end])
		if isAlphaNumeric(r) {
			break
		}
		end -= size
	}

	return s[start:end]
}

func decodeFirst(s string) (rune, int) {
	for i, r := range s {
		return r, i + len(string(r))
	}
	return 0, 0
}

func decodeLast(s string) (rune, int) {
	runes := []rune(s)
	if len(runes) == 0 {
		return 0, 0
	}
	last := runes[len(runes)-1]
	return last, len(string(last))
}

func isAlphaNumeric(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9')
}

// splitPath breaks a URL path into word-like segments, skipping short or
// numeric-only fragments.
func splitPath(path string) []string {
	// Strip leading/trailing slashes and split.
	p := path
	for len(p) > 0 && p[0] == '/' {
		p = p[1:]
	}
	for len(p) > 0 && p[len(p)-1] == '/' {
		p = p[:len(p)-1]
	}
	if p == "" {
		return nil
	}

	parts := splitBy(p, '/')
	var out []string
	for _, part := range parts {
		// Split on common separators: -, _, .
		sub := splitByAny(part)
		for _, s := range sub {
			if len(s) >= 3 && !isOnlyDigits(s) {
				out = append(out, s)
			}
		}
	}
	return out
}

func splitBy(s string, sep byte) []string {
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			if i > start {
				parts = append(parts, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		parts = append(parts, s[start:])
	}
	return parts
}

func splitByAny(s string) []string {
	// Split on -, _, .
	var parts []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '-' || s[i] == '_' || s[i] == '.' {
			if i > start {
				parts = append(parts, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		parts = append(parts, s[start:])
	}
	return parts
}

func isOnlyDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
