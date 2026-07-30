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
// returning "" if the line should be discarded.
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
	// Skip lines that look like explanations or metadata.
	if strings.Count(line, " ") > 0 {
		return ""
	}
	return line
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
// Small models need dead-simple instructions with concrete examples.
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
				if techStr != "" {
					techStr += ", "
				}
				techStr += v
			}
		case "keywords":
			if v := extractValue(c.Text); v != "" && !strings.Contains(v, ".") && len(v) > 2 {
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
		b.WriteString("Tech: ")
		b.WriteString(techStr)
		b.WriteString("\n")
	}
	if kwStr != "" {
		b.WriteString("Keywords: ")
		b.WriteString(kwStr)
		b.WriteString("\n")
	}
	b.WriteString("\nGenerate 500 password guesses for this company. One per line.\n")
	b.WriteString("Example: RonBarcelo2026, BarceloAdmin!, BrandCenter2026, Ron2026Admin\n")
	b.WriteString("Only output passwords. No explanations. No markdown.\n")

	return b.String()
}

// extractValue strips the "section: " prefix that the chunker prepends.
func extractValue(text string) string {
	if idx := strings.Index(text, ": "); idx >= 0 {
		return strings.TrimSpace(text[idx+2:])
	}
	return strings.TrimSpace(text)
}
