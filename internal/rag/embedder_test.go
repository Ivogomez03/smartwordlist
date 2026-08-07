package rag

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ivogomez03/smartwordlist/internal/ollama"
)

// TestOllamaEmbedder_DimsMatchesModel is the regression guard for a bug
// where Dims() always returned the hardcoded nomic-embed-text value (768)
// regardless of the configured --embedding-model, silently corrupting the
// veclite collection schema for any model with a different vector length
// (e.g. mxbai-embed-large at 1024, all-minilm at 384).
func TestOllamaEmbedder_DimsMatchesModel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a model that returns 384-dimensional vectors, not 768.
		vec := make([]float32, 384)
		for i := range vec {
			vec[i] = 0.1
		}
		resp := map[string]any{
			"model":      "all-minilm",
			"embeddings": [][]float32{vec},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client := ollama.NewClient(srv.URL, 0)
	embedder := NewOllamaEmbedder(client, "all-minilm")

	if embedder.Dims() != defaultDims {
		t.Fatalf("expected placeholder Dims() of %d before any Embed call, got %d", defaultDims, embedder.Dims())
	}

	vectors, err := embedder.Embed(t.Context(), []string{"some text"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vectors) != 1 || len(vectors[0]) != 384 {
		t.Fatalf("expected 1 vector of length 384, got %d vectors of length %d", len(vectors), len(vectors[0]))
	}

	if embedder.Dims() != 384 {
		t.Errorf("Dims() should reflect the actual model output (384), got %d", embedder.Dims())
	}
}
