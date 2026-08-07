package rag

import (
	"context"
	"fmt"

	"github.com/Ivogomez03/smartwordlist/internal/ollama"
)

// defaultDims is the dimension assumed before any embedding has been
// observed. It matches nomic-embed-text, the default model, but is only a
// placeholder — Dims() reflects the actual model's output once Embed has
// been called at least once.
const defaultDims = 768

// OllamaEmbedder generates embedding vectors using an Ollama model. It
// wraps the internal ollama HTTP client and implements the EmbeddingProvider
// interface from the pipeline design.
//
// dims starts at defaultDims (768, nomic-embed-text) and is corrected to the
// actual returned vector length on the first successful Embed call, so a
// user-configured --embedding-model with a different dimensionality (e.g.
// mxbai-embed-large at 1024, all-minilm at 384) does not silently corrupt
// the veclite collection schema.
type OllamaEmbedder struct {
	client *ollama.Client
	model  string
	dims   int
}

// NewOllamaEmbedder returns an embedder configured for the given Ollama
// client and model name. The dimension defaults to 768 (nomic-embed-text)
// until the first Embed call observes the model's actual vector length.
func NewOllamaEmbedder(client *ollama.Client, model string) *OllamaEmbedder {
	return &OllamaEmbedder{
		client: client,
		model:  model,
		dims:   defaultDims,
	}
}

// Embed generates embedding vectors for the given texts by calling the
// Ollama /api/embed endpoint. Each returned vector has Dims() elements.
// On success, Dims() is updated to match the actual vector length returned
// by the model, so callers must call Embed before relying on Dims() for a
// non-default embedding model.
func (oe *OllamaEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	vectors, err := oe.client.Embed(ctx, oe.model, texts)
	if err != nil {
		return nil, fmt.Errorf("ollama embedder: %w", err)
	}

	if len(vectors) > 0 && len(vectors[0]) > 0 {
		oe.dims = len(vectors[0])
	}

	return vectors, nil
}

// Dims returns the embedding vector dimension. It reflects the actual
// dimensionality observed on the last successful Embed call, or the
// defaultDims placeholder (768) if Embed has not been called yet.
func (oe *OllamaEmbedder) Dims() int {
	return oe.dims
}
