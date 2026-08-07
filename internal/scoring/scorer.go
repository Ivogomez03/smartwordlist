// Package scoring scores, deduplicates, and sorts password candidates using
// source-weighted probability, length bonus, and pattern complexity analysis.
package scoring

import (
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/Ivogomez03/smartwordlist/pkg/types"
)

// sourceWeights maps candidate source labels (lowercased) to base scores on a
// 0-10 scale.  LLM-generated candidates receive the highest weight; rule-
// mutation, dictionary, and combo sources follow in descending order.
var sourceWeights = map[string]float64{
	"llm":           10,
	"rule-mutation": 5,
	"dict":          3,
	"combo":         2,
}

// defaultSourceWeight is used when a candidate's source does not match any
// known weight.  Unknown sources are treated conservatively.
const defaultSourceWeight = 2

// Scorer scores, deduplicates, and sorts password candidates.  It implements
// the Scorer interface from the pipeline design.
//
// Scoring is based on three factors:
//   - Source weight (LLM > rule > dict > combo)
//   - Length bonus (longer passwords are slightly preferred, capped at +1.0)
//   - Pattern complexity (digit, special char, mixed case; up to +1.5)
//
// The final score is clamped to [0, 10].
//
// Deduplication is case-insensitive: "Password" and "password" are treated
// as the same word and only the higher-scored variant is kept.
type Scorer struct{}

// NewScorer returns a ready-to-use Scorer.
func NewScorer() *Scorer {
	return &Scorer{}
}

// Score evaluates every candidate and returns a []ScoredCandidate sorted by
// score descending (highest probability first).  Candidates with equal scores
// are ordered by descending word length as a secondary tiebreaker.
func (s *Scorer) Score(candidates []types.Candidate) []types.ScoredCandidate {
	// Filter out candidates with whitespace — they can't be passwords.
	filtered := make([]types.Candidate, 0, len(candidates))
	for _, c := range candidates {
		if strings.ContainsAny(c.Word, " \t\n\r") {
			continue
		}
		filtered = append(filtered, c)
	}

	out := make([]types.ScoredCandidate, 0, len(filtered))
	for _, c := range filtered {
		sc := s.computeScore(c)
		out = append(out, types.ScoredCandidate{
			Word:   c.Word,
			Score:  clamp(sc),
			Source: c.Source,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return len(out[i].Word) > len(out[j].Word)
	})
	return out
}

// computeScore returns the raw (unclamped) score for a single candidate.
func (s *Scorer) computeScore(c types.Candidate) float64 {
	base, ok := sourceWeights[strings.ToLower(c.Source)]
	if !ok {
		base = defaultSourceWeight
	}

	// length bonus: +0.05 per character, capped at +1.0 (20 chars).
	lb := float64(len(c.Word)) * 0.05
	if lb > 1.0 {
		lb = 1.0
	}

	// pattern complexity bonuses
	var cb float64
	if containsDigit(c.Word) {
		cb += 0.5
	}
	if containsSpecial(c.Word) {
		cb += 0.5
	}
	if mixedCase(c.Word) {
		cb += 0.5
	}

	raw := base + lb + cb

	// Apply predictability penalty before clamping.
	raw -= penalizePredictable(c.Word)

	return raw
}

// clamp bounds v to [0, 10].
func clamp(v float64) float64 {
	return math.Max(0, math.Min(v, 10))
}

// Deduplicate removes case-insensitive duplicates, keeping the higher-scored
// entry when two words differ only in case.  Candidates with the same
// lowercased word from different sources retain the highest score.  The
// returned slice is NOT re-sorted — callers should sort after deduplication
// if needed.
func (s *Scorer) Deduplicate(candidates []types.ScoredCandidate) []types.ScoredCandidate {
	seen := make(map[string]int) // lowercase → index in out
	out := make([]types.ScoredCandidate, 0, len(candidates))

	for _, c := range candidates {
		lw := strings.ToLower(c.Word)
		if idx, exists := seen[lw]; exists {
			if c.Score > out[idx].Score {
				out[idx] = c
			}
			continue
		}
		seen[lw] = len(out)
		out = append(out, c)
	}
	return out
}

// --- helpers ----------------------------------------------------------------

func containsDigit(s string) bool {
	for _, r := range s {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func containsSpecial(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func mixedCase(s string) bool {
	var hasLower, hasUpper bool
	for _, r := range s {
		if unicode.IsLower(r) {
			hasLower = true
		} else if unicode.IsUpper(r) {
			hasUpper = true
		}
		if hasLower && hasUpper {
			return true
		}
	}
	return false
}

// penalizePredictable returns a penalty (0.0 to -6.0) for predictable
// password patterns: year suffixes, trailing symbols, long digit sequences,
// excessive length, repetitive characters, and common weak number patterns.
// These penalties are severe by design — candidates with these patterns
// should be pushed to the bottom of the ranking or eliminated entirely.
func penalizePredictable(word string) float64 {
	var penalty float64
	lower := strings.ToLower(word)

	// --- Long digit sequence: the #1 LLM hallucination signal ---
	// "AppRunNginx123456789012345678" → killed immediately.
	digitSeq := 0
	maxDigitSeq := 0
	for _, r := range lower {
		if r >= '0' && r <= '9' {
			digitSeq++
			if digitSeq > maxDigitSeq {
				maxDigitSeq = digitSeq
			}
		} else {
			digitSeq = 0
		}
	}
	if maxDigitSeq >= 8 {
		penalty += 5.0 // Catastrophic: endless number hallucination.
	} else if maxDigitSeq >= 5 {
		penalty += 3.0 // Severe: 5+ consecutive digits.
	} else if maxDigitSeq == 4 {
		penalty += 1.0 // Suspicious: 4 consecutive digits.
	}

	// --- Excessive length: >24 chars are almost always LLM garbage ---
	if len(word) > 24 {
		penalty += 3.0
	} else if len(word) > 20 {
		penalty += 1.0
	}

	// --- Year suffixes ---
	if len(lower) > 4 {
		suffix := lower[len(lower)-4:]
		if suffix >= "2015" && suffix <= "2030" {
			penalty += 2.0
		}
	}

	// --- Single-char symbol suffixes ---
	if len(lower) > 1 {
		last := lower[len(lower)-1]
		if last == '!' || last == '@' || last == '#' || last == '$' {
			penalty += 0.5
		}
	}

	// --- Repetitive characters (same char 3+ times in a row) ---
	repSeq := 0
	lastRune := rune(0)
	for _, r := range lower {
		if r == lastRune {
			repSeq++
		} else {
			if repSeq >= 3 {
				penalty += 1.5
			}
			repSeq = 1
			lastRune = r
		}
	}
	if repSeq >= 3 {
		penalty += 1.5
	}

	// --- Keyboard walks: common sequences ---
	walks := []string{
		"qwerty", "asdfgh", "zxcvbn", "qazwsx",
		"123456", "12345", "1234567", "12345678",
		"abcdef", "abc123",
	}
	for _, w := range walks {
		if strings.Contains(lower, w) {
			penalty += 2.0
			break
		}
	}

	// --- Common weak number patterns ---
	commonNums := []string{
		"123", "1234", "000", "111", "222", "333", "444",
		"555", "666", "777", "888", "999", "0000", "1111",
	}
	for _, num := range commonNums {
		if strings.Contains(lower, num) {
			penalty += 0.5
			break
		}
	}

	// --- All-lowercase with no digits or specials: zero complexity ---
	hasDigit := false
	hasSpecial := false
	hasUpper := false
	for _, r := range word {
		if r >= '0' && r <= '9' {
			hasDigit = true
		} else if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			hasSpecial = true
		} else if unicode.IsUpper(r) {
			hasUpper = true
		}
	}
	if !hasDigit && !hasSpecial && !hasUpper {
		penalty += 1.0 // Pure lowercase word — too simple.
	}

	return penalty
}
