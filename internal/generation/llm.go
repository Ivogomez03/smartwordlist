package generation

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

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

// Generate sends a prompt to Ollama with JSON-Schema structured output
// constraints, then parses the response. The primary path expects a JSON
// object with a "passwords" array; a reinforced heuristic fallback handles
// models that ignore the schema.
func (lg *LLMGenerator) Generate(ctx context.Context, chunks []types.ScoredChunk, max int) ([]types.Candidate, error) {
	prompt := buildPrompt(chunks)

	// JSON-Schema object constraining the output shape.
	jsonSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"passwords": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "string",
				},
			},
		},
		"required": []string{"passwords"},
	}

	thinkFalse := false
	ch, err := lg.client.Generate(ctx, lg.model, prompt, false, jsonSchema, &thinkFalse)
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

// ---------------------------------------------------------------------------
// Candidate extraction (two-tier)
// ---------------------------------------------------------------------------

// passwordsResponse is the expected JSON shape from the LLM.
type passwordsResponse struct {
	Passwords []string `json:"passwords"`
}

// extractCandidates parses the LLM response using two tiers:
//
//  1. PRIMARY (JSON): locate the first '{' and matching '}' to extract a
//     {"passwords": [...]} object. Validate entries through isValidCandidate
//     and deterministic dedup (first-seen, lowercased key, preserves original
//     casing).
//
//  2. FALLBACK (line scan): only if JSON yields nothing. Split on newlines
//     and route every line through cleanLine + isValidCandidate. We do NOT
//     skip fenced code blocks: when a model ignores the schema it often
//     embeds the real password guesses inside a fenced "Output:" demo block,
//     and we want to RECOVER those while isValidCandidate rejects the code.
//     splitOnWordBoundaries is the last resort.
func extractCandidates(text string, max int) []types.Candidate {
	// ---- Primary: JSON ----
	if candidates := extractJSONCandidates(text, max); len(candidates) > 0 {
		return candidates
	}

	// ---- Fallback: line scan ----
	return extractLineCandidates(text, max)
}

// extractJSONCandidates finds the first JSON object containing a "passwords"
// array and returns validated + deduped candidates.
func extractJSONCandidates(text string, max int) []types.Candidate {
	start := strings.IndexByte(text, '{')
	if start < 0 {
		return nil
	}

	// Find matching close brace with basic nesting.
	depth := 0
	end := -1
	for i := start; i < len(text); i++ {
		switch text[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i + 1 // include the '}'
				goto found
			}
		}
	}
found:
	if end < 0 {
		return nil
	}

	var resp passwordsResponse
	if err := json.Unmarshal([]byte(text[start:end]), &resp); err != nil {
		return nil
	}
	if len(resp.Passwords) == 0 {
		return nil
	}

	// Validate + deterministic dedup (first-seen, keyed lowercase, preserves
	// original casing).
	seen := make(map[string]bool)
	var candidates []types.Candidate
	for _, p := range resp.Passwords {
		if !isValidCandidate(p) {
			continue
		}
		key := strings.ToLower(p)
		if seen[key] {
			continue
		}
		seen[key] = true
		candidates = append(candidates, types.Candidate{Word: p, Source: "llm"})
		if max > 0 && len(candidates) >= max {
			break
		}
	}

	return candidates
}

// extractLineCandidates is the reinforced fallback parser. It does NOT skip
// fenced code blocks: when a model ignores the JSON schema it embeds the
// real password guesses inside a fenced "Output:" demonstration block, and
// we want to RECOVER those. Code lines are rejected by isValidCandidate
// (parentheses, colons, blocklist tokens) rather than by fence tracking.
func extractLineCandidates(text string, max int) []types.Candidate {
	seen := make(map[string]bool)
	var candidates []types.Candidate

	for _, raw := range strings.Split(text, "\n") {
		// Fence delimiters themselves carry no password content.
		trimmed := strings.TrimSpace(raw)
		if strings.HasPrefix(trimmed, "```") {
			continue
		}

		word := cleanLine(raw)
		if word == "" {
			continue
		}
		key := strings.ToLower(word)
		if seen[key] {
			continue
		}
		if !isValidCandidate(word) {
			continue
		}
		seen[key] = true
		candidates = append(candidates, types.Candidate{Word: word, Source: "llm"})
		if max > 0 && len(candidates) >= max {
			return candidates
		}
	}

	// Last resort: split on word boundaries.
	if len(candidates) == 0 && len(text) > 0 {
		words := splitOnWordBoundaries(text)
		for _, w := range words {
			word := cleanLine(w)
			if word == "" {
				continue
			}
			key := strings.ToLower(word)
			if seen[key] {
				continue
			}
			if !isValidCandidate(word) {
				continue
			}
			seen[key] = true
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
	text = strings.ReplaceAll(text, ",", " ")
	text = strings.ReplaceAll(text, ";", " ")
	text = strings.ReplaceAll(text, "\t", " ")
	return strings.Fields(text)
}

// ---------------------------------------------------------------------------
// Candidate validation
// ---------------------------------------------------------------------------

// programmingNoise is a set of known programming/structural tokens that
// never belong in a password wordlist. Matched case-insensitively against
// the WHOLE token (not substrings).
var programmingNoise = map[string]bool{
	"python": true, "js": true, "javascript": true,
	"go": true, "rust": true, "code": true,
	"main": true, "def": true, "import": true,
	"return": true, "break": true, "continue": true,
	"for": true, "while": true, "if": true,
	"else": true, "elif": true, "print": true,
	"range": true, "random": true, "append": true,
	"string": true, "int": true, "void": true,
	"none": true, "null": true, "true": true, "false": true,
	"list": true, "array": true, "output": true,
	"password": true, "passwords": true,
	"example": true, "examples": true, "note": true,
	"approach": true, "explanation": true,
}

// codeSyntaxChars are code-structure characters that never appear in real
// passwords. Password symbols (!@#$%^&*+_-./ and digits/letters) are ALLOWED.
var codeSyntaxChars = []byte{'(', ')', '[', ']', '{', '}', '=', ':', ',', ';', '"', '\'', '`'}

// isValidCandidate returns true when w passes all structural and semantic
// filters and is a plausible password candidate.
func isValidCandidate(w string) bool {
	// Blocklist: whole-token programming noise (case-insensitive).
	lower := strings.ToLower(w)
	if programmingNoise[lower] {
		return false
	}

	// Reject code syntax characters: () [] {} = : , ; " ' backtick.
	if strings.ContainsAny(w, string(codeSyntaxChars)) {
		return false
	}

	return true
}

// cleanLine normalizes a line into a password candidate or returns "".
// Rules are deliberately permissive for real passwords but aggressively
// reject code, markdown, and meta-commentary.
func cleanLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}

	// Strip stray markdown code-fence markers (the caller skips bare fence
	// lines, but a fence may be concatenated to other text by splitOnWordBoundaries).
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

	// Length bounds.
	if len(line) < 4 {
		return ""
	}
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

	// Reject lines with spaces/tabs (can't be a single password).
	if strings.ContainsAny(line, " \t") {
		return ""
	}

	// Reject markdown formatting remnants.
	line = strings.ReplaceAll(line, "**", "")
	line = strings.ReplaceAll(line, "__", "")

	// NOTE: we intentionally do NOT reject a leading "#". Markdown headers
	// like "# Heading" are already rejected by the space check above, and the
	// model legitimately generates password guesses starting with "#" (e.g.
	// "#barcelo25"). Rejecting "#" would drop real candidates.

	// Reject 5+ consecutive digits (LLM hallucination).
	if longDigitSeq.MatchString(line) {
		return ""
	}

	return line
}

func isMetaLine(line string) bool {
	lower := strings.ToLower(line)
	// Filter lines that are CLEARLY meta-commentary, not password candidates.
	if strings.HasPrefix(lower, "here are") ||
		strings.HasPrefix(lower, "sure") ||
		strings.HasPrefix(lower, "certainly") ||
		strings.HasPrefix(lower, "below") ||
		strings.HasPrefix(lower, "the following") ||
		strings.HasPrefix(lower, "note:") ||
		strings.HasPrefix(lower, "assistant:") ||
		strings.HasPrefix(lower, "user:") {
		return true
	}

	// Catch prose lines that start with low-signal words only when the
	// line contains no digits AND no symbols that would suggest a password.
	// A real password like "to2024!" or "theboss#1" must NOT be caught.
	if !containsAny(lower, "0123456789!@#$%^&*+") {
		for _, prefix := range []string{
			"to solve", "solution", "approach", "generate",
			"this", "the ", "we ", "you ",
		} {
			if strings.HasPrefix(lower, prefix) {
				return true
			}
		}
	}

	return strings.Contains(lower, "sorry") ||
		strings.Contains(lower, "cannot generate") ||
		strings.Contains(lower, "i cannot")
}

// containsAny reports whether s contains any byte from chars.
func containsAny(s, chars string) bool {
	return strings.ContainsAny(s, chars)
}

// ---------------------------------------------------------------------------
// Prompt builder
// ---------------------------------------------------------------------------

// buildPrompt creates a prompt that instructs the model to output a
// structured JSON object. It derives seed words from the company name
// for context, then provides a concrete few-shot JSON example to guide
// the output shape. The prompt explicitly forbids code, explanations,
// markdown, and prose.
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

	// Role and context.
	b.WriteString("You are a password candidate generator for an AUTHORIZED security assessment.\n")
	b.WriteString("Output ONLY a single JSON object — nothing else.\n\n")

	// Context.
	b.WriteString("Company: ")
	if company != "" {
		b.WriteString(company)
	} else {
		b.WriteString("(unknown)")
	}
	b.WriteString("\n\n")

	// Diversity instructions — natural language, not a programming spec.
	b.WriteString("Generate 50 diverse password guesses derived from the company name and its parts.\n")
	b.WriteString("Include a mix of formats: word+number, word+symbol, word+word, lowercase+year.\n")
	b.WriteString("Use different combinations, symbols (!@#$%.), 2-4 digit numbers, and mixed case.\n")
	b.WriteString("Vary length: short (6-8 chars) and medium (10-16 chars).\n\n")

	// Concrete few-shot JSON example with company-derived seeds.
	if len(seeds) > 0 {
		b.WriteString("Example output shape (use your own generated passwords):\n")
		b.WriteString(`{"passwords": [`)
		for i, s := range seeds {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(`"` + s + `2024!", "` + s + titleFirst(s) + `#24"`)
			// Add cross-seed and varied examples.
			if i < len(seeds)-1 {
				s2 := seeds[i+1]
				b.WriteString(`, "` + s + s2 + `12", "` + strings.ToLower(s) + `_2025"`)
			}
		}
		b.WriteString("]}\n\n")
	}

	// Hard constraint — must be last, must be explicit.
	b.WriteString("Output ONLY the JSON object. No code. No explanation. No markdown. No prose.\n")

	return b.String()
}

// titleFirst upper-cases the first rune of s and leaves the rest unchanged.
// Unlike a byte slice (s[:1] + s[1:]), this is safe for multi-byte UTF-8
// leading characters (e.g. accented or non-Latin company names) — slicing by
// byte would otherwise cut a rune in half and corrupt the string.
func titleFirst(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[size:]
}

// extractValue strips the "section: " prefix that the chunker prepends.
func extractValue(text string) string {
	if idx := strings.Index(text, ": "); idx >= 0 {
		return strings.TrimSpace(text[idx+2:])
	}
	return strings.TrimSpace(text)
}
