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
		if len(part) > 2 && !isJunkWord(part) {
			add(part)
		}
	}
	// Technologies are NOT used as base words — they're context for the LLM,
	// not something people put in passwords.
	for _, k := range r.Keywords {
		if !isJunkWord(k) {
			add(k)
		}
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
// Accented characters (ó, ñ, ç, etc.) are first normalized to their ASCII
// base so Spanish/Portuguese brand names like "Barceló" become "Barcelo"
// instead of being truncated to "Barcel".
func cleanWord(s string) string {
	s = normalizeAccents(s)
	s = trimNonAlphaNumeric(s)
	if s == "" {
		return ""
	}
	return s
}

// isJunkWord returns true for words that are too generic to be useful
// as password base words. This includes common English function words,
// tech noise, and web boilerplate terms scraped from pages.
func isJunkWord(w string) bool {
	w = strings.ToLower(w)
	junk := map[string]bool{
		"the": true, "and": true, "for": true, "new": true, "all": true,
		"our": true, "its": true, "has": true, "are": true, "was": true,
		"can": true, "not": true, "you": true, "your": true, "from": true,
		"that": true, "this": true, "with": true, "have": true, "been": true,
		"will": true, "more": true, "page": true, "home": true, "site": true,
		"need": true, "run": true, "app": true, "api": true, "use": true,
		"get": true, "one": true, "two": true, "see": true, "now": true,
		"com": true, "org": true, "net": true, "www": true,
		"enable": true, "javascript": true, "cookie": true, "function": true,
		"brand": true, "center": true, "rights": true, "reserved": true,
		"privacy": true, "policy": true, "terms": true, "contact": true,
		"about": true, "search": true, "menu": true, "close": true,
		"open": true, "login": true, "register": true, "sign": true,
		"subscribe": true, "newsletter": true, "follow": true, "share": true,
		"like": true, "comment": true, "download": true, "upload": true,
		"click": true, "here": true, "link": true, "skip": true,
		"content": true, "main": true, "navigation": true, "footer": true,
		"header": true, "sidebar": true, "related": true, "previous": true,
		"next": true, "back": true, "top": true, "read": true,
		"view": true, "web": true, "website": true, "online": true,
		"internet": true, "https": true, "http": true, "html": true,
		"css": true,
	}
	return junk[w]
}

// accentMap maps common Spanish/Portuguese accented characters to ASCII.
var accentMap = map[rune]rune{
	'á': 'a', 'à': 'a', 'ã': 'a', 'â': 'a', 'ä': 'a',
	'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e',
	'í': 'i', 'ì': 'i', 'î': 'i', 'ï': 'i',
	'ó': 'o', 'ò': 'o', 'õ': 'o', 'ô': 'o', 'ö': 'o',
	'ú': 'u', 'ù': 'u', 'û': 'u', 'ü': 'u',
	'ñ': 'n', 'ç': 'c',
	'Á': 'A', 'À': 'A', 'Ã': 'A', 'Â': 'A', 'Ä': 'A',
	'É': 'E', 'È': 'E', 'Ê': 'E', 'Ë': 'E',
	'Í': 'I', 'Ì': 'I', 'Î': 'I', 'Ï': 'I',
	'Ó': 'O', 'Ò': 'O', 'Õ': 'O', 'Ô': 'O', 'Ö': 'O',
	'Ú': 'U', 'Ù': 'U', 'Û': 'U', 'Ü': 'U',
	'Ñ': 'N', 'Ç': 'C',
}

// normalizeAccents replaces accented characters with their ASCII base forms.
func normalizeAccents(s string) string {
	runes := []rune(s)
	for i, r := range runes {
		if repl, ok := accentMap[r]; ok {
			runes[i] = repl
		}
	}
	return string(runes)
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

// dedupeSlice returns a new slice with duplicate strings removed.
func dedupeSlice(ss []string) []string {
	seen := make(map[string]bool, len(ss))
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
