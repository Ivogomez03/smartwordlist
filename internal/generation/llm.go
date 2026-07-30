package generation

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/Ivogomez03/smartwordlist/internal/ollama"
	"github.com/Ivogomez03/smartwordlist/pkg/types"
)

// llmNumberPrefix matches lines that the model numbered despite being told
// not to — e.g. "28. RonBarceloBrandCenter2024" or "15) somepassword".
var llmNumberPrefix = regexp.MustCompile(`^\d+[.)]\s*`)

// LLMGenerator produces password candidates by sending a RAG-enhanced prompt
// to an Ollama language model and parsing the streaming response line by
// line.  It implements the CandidateGenerator interface from the pipeline
// design.
//
// When Ollama is unreachable or the model is missing the caller should catch
// the error and degrade to the RuleGenerator fallback.
type LLMGenerator struct {
	client *ollama.Client
	model  string
}

// NewLLMGenerator returns an LLMGenerator backed by the given Ollama client
// and model name.  The model is configurable; the default value (passed by
// the caller) is typically the LLM_MODEL env var or "qwen3".
func NewLLMGenerator(client *ollama.Client, model string) *LLMGenerator {
	return &LLMGenerator{client: client, model: model}
}

// Generate builds a structured prompt from the provided RAG chunks, streams
// the response from Ollama, and returns up to max password candidates (one
// per line).  If max is 0, all parsed candidates are returned.
func (lg *LLMGenerator) Generate(ctx context.Context, chunks []types.ScoredChunk, max int) ([]types.Candidate, error) {
	prompt := buildPrompt(chunks)

	ch, err := lg.client.Generate(ctx, lg.model, prompt, true)
	if err != nil {
		return nil, fmt.Errorf("llm generate: %w", err)
	}

	var (
		lineBuf    strings.Builder
		candidates []types.Candidate
	)

	for token := range ch {
		for _, r := range token {
			if r == '\n' {
				cand := strings.TrimSpace(lineBuf.String())
				lineBuf.Reset()
				if cand == "" {
					continue
				}
				// Strip leading numbering the model may add (e.g. "28. password").
				orig := cand
				cand = llmNumberPrefix.ReplaceAllString(cand, "")
				cand = strings.TrimSpace(cand)
				if cand == "" {
					continue
				}
				// Skip prompt-like artefacts the model may echo back.
				if skipLLMLine(cand) {
					continue
				}
				candidates = append(candidates, types.Candidate{
					Word:   cand,
					Source: "llm",
				})
				if max > 0 && len(candidates) >= max {
					return candidates[:max], nil
				}
				_ = orig
			} else {
				lineBuf.WriteRune(r)
			}
		}
	}

	// Flush any remaining content that didn't end with a newline.
	if lineBuf.Len() > 0 {
		cand := strings.TrimSpace(lineBuf.String())
		if cand != "" && !skipLLMLine(cand) {
			candidates = append(candidates, types.Candidate{
				Word:   cand,
				Source: "llm",
			})
		}
	}

	if max > 0 && len(candidates) > max {
		candidates = candidates[:max]
	}

	return candidates, nil
}

// buildPrompt assembles the LLM prompt from RAG-retrieved chunks.  It
// extracts structured context (company, technologies, keywords, paths) and
// asks the model to generate one password candidate per line using common
// corporate patterns.
func buildPrompt(chunks []types.ScoredChunk) string {
	var (
		company  string
		tech     []string
		keywords []string
		paths    []string
	)

	for _, c := range chunks {
		switch c.Source {
		case "company":
			if company == "" {
				company = extractValue(c.Text)
			}
		case "technologies":
			tech = append(tech, extractValue(c.Text))
		case "keywords":
			if v := extractValue(c.Text); v != "" && !strings.Contains(v, ".") {
				keywords = append(keywords, v)
			}
		case "paths":
			paths = append(paths, extractValue(c.Text))
		default:
			// Capture any additional context from other chunk sources.
			if v := extractValue(c.Text); v != "" {
				keywords = append(keywords, v)
			}
		}
	}

	var b strings.Builder
	b.WriteString("Generate realistic password candidates for this company.\n")
	b.WriteString("Think like an employee choosing a memorable but secure-enough password.\n\n")

	if company != "" {
		// Split company name into parts so the model can use them individually.
		parts := strings.Fields(company)
		b.WriteString(fmt.Sprintf("Company: %s", company))
		if len(parts) > 1 {
			b.WriteString(fmt.Sprintf(" (parts: %s)", strings.Join(parts, ", ")))
		}
		b.WriteString("\n")
	}
	if len(tech) > 0 {
		b.WriteString(fmt.Sprintf("Tech: %s\n", strings.Join(dedupeSlice(tech), ", ")))
	}
	if len(keywords) > 0 {
		b.WriteString(fmt.Sprintf("Context: %s\n", strings.Join(dedupeSlice(keywords), ", ")))
	}

	b.WriteString("\nExamples of good passwords for a ski resort named Cerro Bayo:\n")
	b.WriteString("CerroBayo2026, BayoAdmin2026!, Niev3Patagonia, CBAdmin2026\n\n")
	b.WriteString("Rules:\n")
	b.WriteString("- ONE password per line, no numbers/bullets/explanations.\n")
	b.WriteString("- NO spaces, NO dots, NO URLs — passwords are single words.\n")
	b.WriteString("- Use the company name parts, relevant keywords, and year 2026.\n")
	b.WriteString("- Each line must be the FINAL password — include numbers, symbols, and\n")
	b.WriteString("  capitalisation directly. Do NOT output base words to be mutated later.\n")
	b.WriteString("- Generate at least 500 candidates. Be creative and varied.\n")

	return b.String()
}

// extractValue strips the "section: " prefix that the chunker prepends,
// returning only the meaningful value.
func extractValue(text string) string {
	if idx := strings.Index(text, ": "); idx >= 0 {
		return strings.TrimSpace(text[idx+2:])
	}
	return strings.TrimSpace(text)
}

// skipLLMLine returns true for lines that look like meta-text the model
// echoed back rather than a real candidate password.
func skipLLMLine(line string) bool {
	lower := strings.ToLower(line)
	return strings.HasPrefix(lower, "here are") ||
		strings.HasPrefix(lower, "candidate") ||
		strings.Contains(lower, "sorry") ||
		strings.Contains(lower, "i cannot") ||
		strings.HasPrefix(lower, "note:") ||
		strings.HasPrefix(lower, "assistant") ||
		line == ""
}

// dedupeSlice returns a new slice with duplicate strings removed while
// preserving order.
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
