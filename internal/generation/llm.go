package generation

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"github.com/Ivogomez03/smartwordlist/internal/ollama"
	"github.com/Ivogomez03/smartwordlist/pkg/types"
)

// llmNumberPrefix matches lines that the model numbered — e.g. "28. password".
var llmNumberPrefix = regexp.MustCompile(`^\d+[.)]\s*`)

// longDigitSeq matches 5 or more consecutive digits — a clear sign of
// LLM hallucination where the model generates endless number sequences.
var longDigitSeq = regexp.MustCompile(`\d{5,}`)

// maxPasswordLen is the maximum allowed password length. Longer passwords
// are almost always LLM hallucinations (runaway generation).
const maxPasswordLen = 24

// LLMGenerator produces password candidates by sending a RAG-enhanced prompt
// to an Ollama language model and parsing the response line by line.
type LLMGenerator struct {
	client *ollama.Client
	model  string
}

// NewLLMGenerator returns an LLMGenerator backed by the given Ollama client.
func NewLLMGenerator(client *ollama.Client, model string) *LLMGenerator {
	return &LLMGenerator{client: client, model: model}
}

// Generate builds a prompt from RAG chunks, calls Ollama (non-streaming for
// small models that need the full context), and parses password candidates.
func (lg *LLMGenerator) Generate(ctx context.Context, chunks []types.ScoredChunk, max int) ([]types.Candidate, error) {
	prompt := buildPrompt(chunks)

	// Non-streaming — small models produce better output when they see
	// the full response context at once.
	ch, err := lg.client.Generate(ctx, lg.model, prompt, false)
	if err != nil {
		return nil, fmt.Errorf("llm generate: %w", err)
	}

	// Collect the full response.
	var full strings.Builder
	for token := range ch {
		full.WriteString(token)
	}
	response := full.String()

	// Parse lines, filtering aggressively.
	var candidates []types.Candidate
	for _, line := range strings.Split(response, "\n") {
		cand := cleanCandidate(line)
		if cand == "" {
			continue
		}
		candidates = append(candidates, types.Candidate{Word: cand, Source: "llm"})
		if max > 0 && len(candidates) >= max {
			break
		}
	}

	return candidates, nil
}

// cleanCandidate normalizes a line into a valid password candidate,
// returning "" if the line should be discarded. Also applies post-generation
// sanitization to catch LLM hallucinations (long digit sequences, excessive
// length, keyword concatenations).
func cleanCandidate(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	// Strip numbering: "28. password" → "password"
	line = llmNumberPrefix.ReplaceAllString(line, "")
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	// Skip obvious non-password lines.
	if isMetaLine(line) {
		return ""
	}
	// Skip lines that are too short.
	if len(line) < 4 {
		return ""
	}
	// Skip lines with spaces — can't be a password.
	if strings.ContainsAny(line, " \t") {
		return ""
	}
	// Skip lines with markdown artifacts.
	if strings.Contains(line, "#") || strings.Contains(line, "*") {
		return ""
	}
	// Skip lines that are purely numeric.
	if isAllDigits(line) {
		return ""
	}

	// --- Post-generation sanitization ---

	// Reject 5+ consecutive digits (LLM hallucination: "1234567890").
	if longDigitSeq.MatchString(line) {
		return ""
	}

	// Reject passwords longer than 24 chars (runaway generation).
	if len(line) > maxPasswordLen {
		return ""
	}

	// Reject excessive keyword concatenation: 3+ alternating upper/lower
	// word boundaries without any digit or symbol usually means the LLM
	// glued random keywords together (e.g., "AppRunNginxDeployTest").
	if isKeywordConcat(line) {
		return ""
	}

	return line
}

// isKeywordConcat returns true when the candidate appears to be 3 or more
// English words concatenated without any digits or special characters —
// a common LLM hallucination pattern.
func isKeywordConcat(w string) bool {
	// Only check candidates with no digits and no special chars —
	// those are the ones at risk of being pure word concatenation.
	if containsDigit(w) || containsSpecial(w) {
		return false
	}
	// Count CamelCase word boundaries (uppercase letter preceded by lowercase).
	wordCount := 1
	runes := []rune(w)
	for i := 1; i < len(runes); i++ {
		if unicode.IsUpper(runes[i]) && unicode.IsLower(runes[i-1]) {
			wordCount++
		}
	}
	// Also split on common separators.
	for _, r := range runes {
		if r == '_' || r == '-' {
			wordCount++
		}
	}
	return wordCount >= 3 && len(w) > 12
}

// containsDigit reports whether s contains at least one ASCII digit.
func containsDigit(s string) bool {
	for _, r := range s {
		if r >= '0' && r <= '9' {
			return true
		}
	}
	return false
}

// containsSpecial reports whether s contains a non-letter, non-digit rune.
func containsSpecial(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return len(s) > 0
}

func isMetaLine(line string) bool {
	lower := strings.ToLower(line)
	return strings.HasPrefix(lower, "here") ||
		strings.HasPrefix(lower, "candidate") ||
		strings.HasPrefix(lower, "note") ||
		strings.HasPrefix(lower, "assistant") ||
		strings.HasPrefix(lower, "password") ||
		strings.Contains(lower, "sorry") ||
		strings.Contains(lower, "cannot") ||
		strings.Contains(lower, "example")
}

// buildPrompt creates a simple, direct prompt from RAG chunks.
// Small models need dead-simple instructions. Keywords and technologies
// are filtered to remove generic web boilerplate noise that would
// pollute the model's output with junk like "EnableNginx1234".
func buildPrompt(chunks []types.ScoredChunk) string {
	var company, techStr, kwStr string

	for _, c := range chunks {
		switch c.Source {
		case "company":
			if company == "" {
				company = extractValue(c.Text)
			}
		case "technologies":
			if v := extractValue(c.Text); v != "" && !strings.Contains(v, ".") {
				v = strings.ToLower(strings.TrimSpace(v))
				// Skip generic tech noise that produces weak passwords.
				if isLLMJunkWord(v) {
					continue
				}
				if techStr != "" {
					techStr += ", "
				}
				techStr += v
			}
		case "keywords":
			if v := extractValue(c.Text); v != "" && !strings.Contains(v, ".") && len(v) > 2 {
				v = strings.ToLower(strings.TrimSpace(v))
				// Filter boilerplate web keywords so the model doesn't
				// build passwords around "javascript" or "enable".
				if isLLMJunkWord(v) {
					continue
				}
				if kwStr != "" {
					kwStr += ", "
				}
				kwStr += v
			}
		}
	}

	var b strings.Builder
	b.WriteString("Company: ")
	if company != "" {
		b.WriteString(company)
	}
	b.WriteString("\n")
	if techStr != "" {
		b.WriteString("Tech stack (context only): ")
		b.WriteString(techStr)
		b.WriteString("\n")
	}
	if kwStr != "" {
		b.WriteString("Site keywords (context only): ")
		b.WriteString(kwStr)
		b.WriteString("\n")
	}
	b.WriteString("\nYou are a password auditor generating guesses for authorized security testing.\n")
	b.WriteString("Generate 500 password candidates for this company. One per line.\n\n")
	b.WriteString("STRUCTURE RULES — every password MUST follow this format:\n")
	b.WriteString("  BaseWord + Number(2-4 digits) + Symbol(0-1)   OR\n")
	b.WriteString("  BaseWord + Symbol(0-1) + Number(2-4 digits)\n\n")
	b.WriteString("  BaseWord = brand name, product, location, industry term, or employee name pattern.\n")
	b.WriteString("  Combine at most TWO words together (e.g., \"RonBarcelo\", \"BarceloPuntaCana\").\n")
	b.WriteString("  NEVER concatenate three or more words without numbers or symbols.\n\n")
	b.WriteString("DIGIT RULES — CRITICAL:\n")
	b.WriteString("  Use ONLY 2-4 digits per password. Fine: \"2026\", \"24\", \"007\", \"42\".\n")
	b.WriteString("  PROHIBITED: 5 or more consecutive digits. NEVER generate long number sequences like \"1234567890\".\n\n")
	b.WriteString("LENGTH RULES:\n")
	b.WriteString("  Passwords MUST be 6-24 characters long. NOT shorter, NOT longer.\n")
	b.WriteString("  If a candidate would exceed 24 chars, STOP and generate a shorter one.\n\n")
	b.WriteString("VARIETY: Mix simple (Ron2026!) and complex (BarceloImperial24) patterns.\n")
	b.WriteString("Use Spanish/English terms where relevant. Include product names if known.\n")
	b.WriteString("Every candidate MUST be different. NO repetition with only year changes.\n")
	b.WriteString("Only output passwords. No explanations. No markdown. No numbering.\n")

	return b.String()
}

// isLLMJunkWord returns true for words that would pollute the LLM prompt
// and cause the model to generate weak passwords. This is a superset of
// the rule-engine junk filter and adds tech/generic terms.
func isLLMJunkWord(w string) bool {
	junk := map[string]bool{
		// Function words and web boilerplate (same as rules.go isJunkWord)
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
		"css": true, "internal": true,
		// Tech stack — context only, not password material.
		"nginx": true, "apache": true, "cloudflare": true, "plesk": true,
		"plesklin": true, "cpanel": true, "wordpress": true, "jquery": true,
		"bootstrap": true, "react": true, "vue": true, "angular": true,
		"node": true, "express": true, "django": true, "laravel": true,
		"php": true, "mysql": true, "postgres": true, "redis": true,
		"docker": true, "kubernetes": true, "aws": true, "azure": true,
		"google": true, "analytics": true, "tag": true, "manager": true,
		"tech": true, "stack": true, "server": true, "hosting": true,
		"host": true, "cdn": true, "dns": true, "ssl": true,
	}
	return junk[w]
}

// extractValue strips the "section: " prefix that the chunker prepends.
func extractValue(text string) string {
	if idx := strings.Index(text, ": "); idx >= 0 {
		return strings.TrimSpace(text[idx+2:])
	}
	return strings.TrimSpace(text)
}
