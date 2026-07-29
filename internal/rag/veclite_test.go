package rag

import (
	"context"
	"testing"

	"github.com/gentleman-programming/smartwordlist/pkg/types"
)

func TestVecliteStore_IndexAndSearch(t *testing.T) {
	dir := t.TempDir()
	dims := 4

	vs, err := NewVecliteStore(dir, dims)
	if err != nil {
		t.Fatalf("NewVecliteStore: %v", err)
	}

	domain := "test.local"
	chunks := []types.Chunk{
		{Text: "Company: Acme Corp", Source: "company", Metadata: map[string]string{"section": "company"}},
		{Text: "Technologies: React, Go", Source: "technologies", Metadata: map[string]string{"section": "technologies"}},
		{Text: "Keywords: widget, security", Source: "keywords", Metadata: map[string]string{"section": "keywords"}},
	}

	// Synthetic vectors: each is a simple 4d vector where first entry encodes priority.
	vectors := [][]float32{
		{1.0, 0, 0, 0},
		{0, 1.0, 0, 0},
		{0, 0, 1.0, 0},
	}

	ctx := context.Background()
	if err := vs.Index(ctx, domain, chunks, vectors); err != nil {
		t.Fatalf("Index: %v", err)
	}

	// Search with a query vector closest to the first chunk.
	queryVector := []float32{0.9, 0.1, 0, 0.1}
	results, err := vs.Search(ctx, domain, queryVector, 2)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected search results, got none")
	}

	if len(results) > 2 {
		t.Fatalf("expected at most 2 results, got %d", len(results))
	}

	// The first result should be the "company" chunk (closest cosine distance).
	if results[0].Source != "company" {
		t.Errorf("expected 'company' as top result, got '%s'", results[0].Source)
	}
}

func TestVecliteStore_EmptyInput(t *testing.T) {
	dir := t.TempDir()
	vs, err := NewVecliteStore(dir, 4)
	if err != nil {
		t.Fatalf("NewVecliteStore: %v", err)
	}

	// Index with empty slices should work.
	err = vs.Index(context.Background(), "empty.local", nil, nil)
	if err != nil {
		t.Logf("Index with empty input: %v (may be OK)", err)
	}
}

func TestVecliteStore_MismatchedLengths(t *testing.T) {
	dir := t.TempDir()
	vs, err := NewVecliteStore(dir, 4)
	if err != nil {
		t.Fatalf("NewVecliteStore: %v", err)
	}

	chunks := []types.Chunk{
		{Text: "chunk1", Source: "test"},
	}
	vectors := [][]float32{
		{1.0, 0, 0, 0},
		{0, 1.0, 0, 0},
	}

	err = vs.Index(context.Background(), "mismatch.local", chunks, vectors)
	if err == nil {
		t.Fatal("expected error for mismatched chunks/vectors length")
	}
}

func TestVecliteStore_CacheLoadSave(t *testing.T) {
	dir := t.TempDir()
	vs, err := NewVecliteStore(dir, 4)
	if err != nil {
		t.Fatalf("NewVecliteStore: %v", err)
	}

	domain := "cache.local"

	// No cache exists yet.
	exists, err := vs.LoadCache(domain)
	if err != nil {
		t.Fatalf("LoadCache: %v", err)
	}
	if exists {
		t.Error("expected no cache for fresh domain")
	}

	// SaveCache is a no-op (veclite auto-persists on Close).
	if err := vs.SaveCache(domain); err != nil {
		t.Fatalf("SaveCache: %v", err)
	}

	// Index some data.
	chunks := []types.Chunk{{Text: "test", Source: "test"}}
	vectors := [][]float32{{1.0, 0, 0, 0}}
	if err := vs.Index(context.Background(), domain, chunks, vectors); err != nil {
		t.Fatalf("Index: %v", err)
	}

	// After indexing, the cache file should exist.
	exists, err = vs.LoadCache(domain)
	if err != nil {
		t.Fatalf("LoadCache after index: %v", err)
	}
	if !exists {
		t.Error("expected cache to exist after index")
	}
}

func TestChunker_Chunk(t *testing.T) {
	chunker := &Chunker{}

	result := &types.ReconResult{
		Company:      "Acme Corp",
		Title:        "Acme — Enterprise Widgets",
		Technologies: []string{"React", "Go", "PostgreSQL"},
		Keywords:     []string{"widget", "security", "cloud", "automation"},
		Subdomains:   []string{"www.acmecorp.com", "api.acmecorp.com"},
		Paths:        []string{"/admin", "/login"},
		Emails:       []string{"info@acmecorp.com"},
	}

	chunks := chunker.Chunk(result)

	if len(chunks) == 0 {
		t.Fatal("expected non-empty chunks")
	}

	sections := make(map[string]bool)
	for _, c := range chunks {
		sections[c.Source] = true
		if c.Text == "" {
			t.Errorf("chunk for source %q has empty text", c.Source)
		}
	}

	expected := []string{"company", "title", "technologies", "keywords", "subdomains", "paths", "emails"}
	for _, s := range expected {
		if !sections[s] {
			t.Errorf("expected chunk for section %q", s)
		}
	}
}

func TestChunker_NilInput(t *testing.T) {
	chunker := &Chunker{}
	chunks := chunker.Chunk(nil)
	if chunks != nil {
		t.Errorf("expected nil for nil input, got %d chunks", len(chunks))
	}
}

func TestChunker_EmptyReconResult(t *testing.T) {
	chunker := &Chunker{}
	chunks := chunker.Chunk(&types.ReconResult{})
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty recon result, got %d", len(chunks))
	}
}
