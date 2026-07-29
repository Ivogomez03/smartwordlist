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
			keywords = append(keywords, extractValue(c.Text))
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
	b.WriteString("You are a password generation assistant for authorized security assessments.\n")
	b.WriteString("Generate password candidates using common corporate patterns.\n\n")

	if company != "" {
		b.WriteString(fmt.Sprintf("Company: %s\n", company))
	}
	if len(tech) > 0 {
		b.WriteString(fmt.Sprintf("Technologies: %s\n", strings.Join(dedupeSlice(tech), ", ")))
	}
	if len(keywords) > 0 {
		b.WriteString(fmt.Sprintf("Keywords: %s\n", strings.Join(dedupeSlice(keywords), ", ")))
	}
	if len(paths) > 0 {
		// Truncate paths to avoid excessive prompt length.
		shown := paths
		if len(shown) > 10 {
			shown = shown[:10]
		}
		b.WriteString(fmt.Sprintf("Paths: %s\n", strings.Join(dedupeSlice(shown), ", ")))
	}

	b.WriteString("\nPassword patterns to use:\n")
	b.WriteString("- company name + year (e.g. Acme2026)\n")
	b.WriteString("- product or technology + number or symbol\n")
	b.WriteString("- keyword with leet substitutions\n")
	b.WriteString("- combinations of company, product, and season words\n")
	b.WriteString("- common corporate suffix/prefix patterns\n\n")

	b.WriteString("Rules:\n")
	b.WriteString("- Output ONE password per line.\n")
	b.WriteString("- Do NOT number or bullet the lines.\n")
	b.WriteString("- Do NOT include explanations — only the password.\n")
	b.WriteString("- Generate at least 50 candidates.\n")

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
		strings.HasPrefix(lower, "password") ||
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
