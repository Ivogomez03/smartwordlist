// Package types defines shared data structures used across the SmartWordlist pipeline.
package types

import "time"

// Config holds all CLI flags and pipeline configuration injected into each stage.
type Config struct {
	// Domain is the target domain for reconnaissance (required positional arg).
	Domain string
	// Output is the output file path. Empty means stdout.
	Output string
	// Max is the maximum number of candidates to generate. 0 means unlimited.
	Max int
	// Verbose enables detailed progress output.
	Verbose bool
	// NoLLM disables LLM-enhanced generation, forcing rule-only mode.
	NoLLM bool
	// RulesPath is the path to the mutation rules YAML file.
	RulesPath string
	// Model is the Ollama LLM model name (e.g. "qwen3:0.6b").
	Model string
	// EmbedModel is the Ollama embedding model name (e.g. "nomic-embed-text").
	EmbedModel string
	// Path is an optional URL path (e.g. "/login") for the initial scrape target.
	// Empty means the root path "/".
	Path string
	// DryRunOllama skips the pipeline and only checks Ollama health + model availability.
	DryRunOllama bool
}

// ReconResult holds all intelligence gathered during the reconnaissance phase.
type ReconResult struct {
	// Title is the HTML <title> of the target page.
	Title string
	// Company is the detected organization name.
	Company string
	// Keywords are extracted meta keywords and page content terms.
	Keywords []string
	// Technologies are detected tech stack components (frameworks, servers, etc.).
	Technologies []string
	// Subdomains are DNS-enumerated subdomains.
	Subdomains []string
	// Emails are harvested email addresses.
	Emails []string
	// Paths are discovered URL paths (robots.txt, sitemap, etc.).
	Paths []string
}

// Candidate is a generated password candidate before scoring.
type Candidate struct {
	// Word is the candidate password string.
	Word string
	// Source identifies the generator origin (e.g., "llm", "rule-mutation", "dict").
	Source string
}

// ScoredCandidate is a scored and ranked password candidate.
type ScoredCandidate struct {
	// Word is the candidate password string.
	Word string
	// Score is the relevance/strength score (higher = better).
	Score float64
	// Source identifies the generator origin.
	Source string
}

// Chunk is a text chunk with source metadata used in RAG indexing.
type Chunk struct {
	// Text is the chunk content.
	Text string
	// Source identifies where this chunk came from (e.g., "title", "meta", "dns").
	Source string
	// Metadata holds additional context (e.g., URL, timestamp).
	Metadata map[string]string
}

// ScoredChunk is a chunk with a relevance score from similarity search.
type ScoredChunk struct {
	Chunk
	// Score is the similarity/relevance score.
	Score float64
}

// Stats holds generation statistics for JSON metadata export.
type Stats struct {
	// TotalCandidates is the total number of candidates generated.
	TotalCandidates int
	// GenerationTime is the wall-clock time for the generation phase.
	GenerationTime time.Duration
	// SourcesUsed lists all generator sources that contributed candidates.
	SourcesUsed []string
	// MutationCounts maps mutation types to their application counts.
	MutationCounts map[string]int
}
