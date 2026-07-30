package generation

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode"

	"github.com/Ivogomez03/smartwordlist/internal/ollama"
	"github.com/Ivogomez03/smartwordlist/pkg/types"
)

// llmNumberPrefix matches lines that the model numbered — e.g. "28. password".
var llmNumberPrefix = regexp.MustCompile(`^\d+[.)]\s*`)

// longDigitSeq matches 5 or more consecutive digits — LLM hallucination.
var longDigitSeq = regexp.MustCompile(`\d{5,}`)

// LLMGenerator produces password candidates via Ollama.
type LLMGenerator struct {
	client *ollama.Client
	model  string
	debug  bool
}

func NewLLMGenerator(client *ollama.Client, model string) *LLMGenerator {
	return &LLMGenerator{client: client, model: model}
}

func (lg *LLMGenerator) SetDebug(v bool) {
	lg.debug = v
}

// Generate sends a simple prompt to Ollama and extracts password-like
// strings from the response. The parser is intentionally lenient —
// downstream scoring and junk filters handle quality.
func (lg *LLMGenerator) Generate(ctx context.Context, chunks []types.ScoredChunk, max int) ([]types.Candidate, error) {
	prompt := buildPrompt(chunks)

	ch, err := lg.client.Generate(ctx, lg.model, prompt, false)
	if err != nil {
		return nil, fmt.Errorf("llm generate: %w", err)
	}

	var full strings.Builder
	for token := range ch {
		full.WriteString(token)
	}
	response := full.String()

	if lg.debug {
		fmt.Fprintf(os.Stderr, "\n--- RAW LLM (%d bytes) ---\n%s\n--- END RAW ---\n",
			len(response), response)
	}

	return extractCandidates(response, max), nil
}

// extractCandidates parses the LLM response with a lenient approach:
// split on newlines, strip common formatting, and accept anything that
// looks like a plausible password (4-30 chars, has letters, no spaces).
// Quality filtering happens downstream in the scorer and isJunkCandidate.
func extractCandidates(text string, max int) []types.Candidate {
	var candidates []types.Candidate
	seen := make(map[string]bool)

	for _, raw := range strings.Split(text, "\n") {
		word := cleanLine(raw)
		if word == "" {
			continue
		}
		if seen[word] {
			continue
		}
		seen[word] = true
		candidates = append(candidates, types.Candidate{Word: word, Source: "llm"})
		if max > 0 && len(candidates) >= max {
			break
		}
	}

	// Fallback: if splitting on newlines yielded nothing, try splitting
	// on whitespace/punctuation — some models output comma-separated lists.
	if len(candidates) == 0 && len(text) > 0 {
		words := splitOnWordBoundaries(text)
		for _, w := range words {
			word := cleanLine(w)
			if word == "" || seen[word] {
				continue
			}
			seen[word] = true
			candidates = append(candidates, types.Candidate{Word: word, Source: "llm"})
			if max > 0 && len(candidates) >= max {
				break
			}
		}
	}

	return candidates
}

// splitOnWordBoundaries splits text on commas, semicolons, and whitespace.
func splitOnWordBoundaries(text string) []string {
	// Replace common separators with spaces, then split.
	text = strings.ReplaceAll(text, ",", " ")
	text = strings.ReplaceAll(text, ";", " ")
	text = strings.ReplaceAll(text, "\t", " ")
	return strings.Fields(text)
}

// cleanLine normalizes a line into a password candidate or returns "".
// Rules are deliberately permissive — only reject obviously non-password text.
func cleanLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}

	// Strip markdown code fences.
	line = strings.TrimPrefix(line, "```")
	line = strings.TrimSuffix(line, "```")
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}

	// Strip numbering: "28. password" or "28) password".
	line = llmNumberPrefix.ReplaceAllString(line, "")
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}

	// Strip leading/trailing quotes.
	line = strings.Trim(line, "\"'`")
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}

	// Skip meta-commentary lines.
	if isMetaLine(line) {
		return ""
	}

	// Minimum length.
	if len(line) < 4 {
		return ""
	}

	// Maximum length (runaway generation).
	if len(line) > 30 {
		return ""
	}

	// Must have at least one letter.
	hasLetter := false
	for _, r := range line {
		if unicode.IsLetter(r) {
			hasLetter = true
			break
		}
	}
	if !hasLetter {
		return ""
	}

	// Reject lines with spaces (can't be a single password).
	if strings.ContainsAny(line, " \t") {
		return ""
	}

	// Reject lines with markdown formatting remnants.
	line = strings.ReplaceAll(line, "**", "")
	line = strings.ReplaceAll(line, "__", "")
	if strings.HasPrefix(line, "#") {
		return ""
	}

	// Reject 5+ consecutive digits (LLM hallucination).
	if longDigitSeq.MatchString(line) {
		return ""
	}

	return line
}

func isMetaLine(line string) bool {
	lower := strings.ToLower(line)
	// Only filter lines that are CLEARLY meta-commentary, not password candidates.
	return strings.HasPrefix(lower, "here are") ||
		strings.HasPrefix(lower, "sure") ||
		strings.HasPrefix(lower, "certainly") ||
		strings.HasPrefix(lower, "below") ||
		strings.HasPrefix(lower, "the following") ||
		strings.Contains(lower, "sorry") ||
		strings.Contains(lower, "cannot generate") ||
		strings.Contains(lower, "i cannot") ||
		strings.HasPrefix(lower, "note:") ||
		strings.HasPrefix(lower, "assistant:") ||
		strings.HasPrefix(lower, "user:")
}

// buildPrompt creates a short prompt that produces varied output.
// The key insight: models default to enumerating (word1, word2, word3...)
// unless explicitly told to vary. A few seed examples derived from the
// company name kickstart diversity without being hardcoded patterns.
func buildPrompt(chunks []types.ScoredChunk) string {
	var company string

	for _, c := range chunks {
		if c.Source == "company" && company == "" {
			company = extractValue(c.Text)
		}
	}

	// Derive seed words from company name for example variety.
	var seeds []string
	if company != "" {
		parts := strings.Fields(company)
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if len(p) > 2 {
				seeds = append(seeds, p)
			}
		}
	}

	var b strings.Builder
	b.WriteString("Company: ")
	if company != "" {
		b.WriteString(company)
	}
	b.WriteString("\n\n")
	b.WriteString("Generate 500 password guesses. Vary EVERY password:\n")
	b.WriteString("- Mix formats: Word+Number, Word+Symbol, WordWord, lowercase+year\n")
	b.WriteString("- Use different combinations of company name parts\n")
	b.WriteString("- Add symbols (!@#$%.) and 2-4 digit numbers (not sequential)\n")
	b.WriteString("- Mix upper/lowercase. Include short (6-8) and medium (10-16) lengths.\n")
	if len(seeds) > 0 {
		b.WriteString("Examples of VARIED passwords (do NOT copy these): ")
		for i, s := range seeds {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(s + "2024!")
			if i < len(seeds)-1 {
				seed2 := seeds[i+1]
				if seed2 != s {
					b.WriteString(", " + s + seed2 + "#24")
				}
			}
		}
		b.WriteString("\n")
	}
	b.WriteString("CRITICAL: Do NOT enumerate (word1, word2, word3...).\n")
	b.WriteString("Each line must look like a DIFFERENT person created it.\n")
	b.WriteString("Output only passwords. One per line. No other text.\n")

	return b.String()
}

// isLLMJunkWord is kept for reference but no longer used in the prompt builder.
// The minimal prompt doesn't include keywords or tech stack — just the company name.

// extractValue strips the "section: " prefix that the chunker prepends.
func extractValue(text string) string {
	if idx := strings.Index(text, ": "); idx >= 0 {
		return strings.TrimSpace(text[idx+2:])
	}
	return strings.TrimSpace(text)
}
