package rag

import (
	"context"
	"fmt"

	"github.com/Ivogomez03/smartwordlist/internal/ollama"
)

// OllamaEmbedder generates embedding vectors using an Ollama model. It
// wraps the internal ollama HTTP client and implements the EmbeddingProvider
// interface from the pipeline design.
//
// The default model is nomic-embed-text with 768-dimensional vectors.
type OllamaEmbedder struct {
	client *ollama.Client
	model  string
	dims   int
}

// NewOllamaEmbedder returns an embedder configured for the given Ollama
// client and model name. The dimension defaults to 768 (nomic-embed-text).
func NewOllamaEmbedder(client *ollama.Client, model string) *OllamaEmbedder {
	return &OllamaEmbedder{
		client: client,
		model:  model,
		dims:   768, // nomic-embed-text
	}
}

// Embed generates embedding vectors for the given texts by calling the
// Ollama /api/embed endpoint. Each returned vector has Dims() elements.
func (oe *OllamaEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	vectors, err := oe.client.Embed(ctx, oe.model, texts)
	if err != nil {
		return nil, fmt.Errorf("ollama embedder: %w", err)
	}

	return vectors, nil
}

// Dims returns the embedding vector dimension — 768 for nomic-embed-text.
func (oe *OllamaEmbedder) Dims() int {
	return oe.dims
}
