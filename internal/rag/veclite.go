package rag

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"

	"github.com/abdul-hamid-achik/veclite"
	"github.com/Ivogomez03/smartwordlist/pkg/types"
)

// VecliteStore wraps the veclite embedded vector database for chunk storage
// and HNSW similarity search. It implements the ContextRetriever interface
// defined in the pipeline design.
//
// Each domain maps to a deterministic cache file under cacheDir:
//
//	~/.cache/smartwordlist/{sha256(domain)}.veclite
//
// veclite persists automatically on Close — no explicit save step needed.
type VecliteStore struct {
	cacheDir string
	dims     int
}

// NewVecliteStore initializes the store with a cache directory and vector
// dimension. The cache directory is created if it does not exist.
func NewVecliteStore(cacheDir string, dims int) (*VecliteStore, error) {
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("veclite store: create cache dir: %w", err)
	}
	return &VecliteStore{
		cacheDir: cacheDir,
		dims:     dims,
	}, nil
}

// cachePath returns the deterministic file path for a domain's veclite file.
func (vs *VecliteStore) cachePath(domain string) string {
	hash := sha256.Sum256([]byte(domain))
	return filepath.Join(vs.cacheDir, fmt.Sprintf("%x.veclite", hash))
}

// LoadCache checks whether a cached veclite file exists for the domain.
// When a cache exists the caller can skip re-indexing and go straight to Search.
func (vs *VecliteStore) LoadCache(domain string) (bool, error) {
	_, err := os.Stat(vs.cachePath(domain))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// SaveCache is a no-op for the ContextRetriever interface. veclite
// persists data to disk automatically on DB Close, so explicit saving
// is unnecessary.
func (vs *VecliteStore) SaveCache(domain string) error {
	return nil
}

// Index stores chunks with their pre-computed embedding vectors in a
// veclite database keyed by domain. It opens (or creates) the cache file,
// initialises a collection with HNSW indexing and cosine distance, inserts
// every chunk→vector pair, and closes the database to persist.
//
// chunks and vectors must have the same length; each vectors[i] is the
// embedding for chunks[i].Text.
func (vs *VecliteStore) Index(ctx context.Context, domain string, chunks []types.Chunk, vectors [][]float32) error {
	if len(chunks) != len(vectors) {
		return fmt.Errorf("veclite index: chunks (%d) and vectors (%d) length mismatch",
			len(chunks), len(vectors))
	}

	path := vs.cachePath(domain)

	// Start fresh: remove a stale cache so CreateCollection never hits
	// ErrCollectionExists from a previous interrupted run.
	_ = os.Remove(path)

	db, err := veclite.Open(path)
	if err != nil {
		return fmt.Errorf("veclite index: open db: %w", err)
	}
	defer db.Close()

	coll, err := db.CreateCollection("chunks",
		veclite.WithDimension(vs.dims),
		veclite.WithDistanceType(veclite.DistanceCosine),
		veclite.WithHNSW(16, 200),
	)
	if err != nil {
		return fmt.Errorf("veclite index: create collection: %w", err)
	}

	for i, chunk := range chunks {
		// Build payload: copy chunk fields plus domain for traceability.
		payload := make(map[string]any, len(chunk.Metadata)+3)
		payload["text"] = chunk.Text
		payload["source"] = chunk.Source
		payload["domain"] = domain
		for k, v := range chunk.Metadata {
			payload[k] = v
		}

		if _, err := coll.Insert(vectors[i], payload); err != nil {
			return fmt.Errorf("veclite index: insert chunk %d: %w", i, err)
		}
	}

	// Close triggers persistence to disk.
	return db.Close()
}

// Search performs HNSW similarity search and returns the top-K scored
// chunks for the given domain and query vector. The database is opened
// in read-only mode to avoid write-lock contention.
func (vs *VecliteStore) Search(ctx context.Context, domain string, queryVector []float32, k int) ([]types.ScoredChunk, error) {
	path := vs.cachePath(domain)

	db, err := veclite.Open(path, veclite.WithReadOnly(true))
	if err != nil {
		return nil, fmt.Errorf("veclite search: open db: %w", err)
	}
	defer db.Close()

	coll, err := db.GetCollection("chunks")
	if err != nil {
		return nil, fmt.Errorf("veclite search: get collection: %w", err)
	}

	results, err := coll.Search(queryVector, veclite.TopK(k))
	if err != nil {
		return nil, fmt.Errorf("veclite search: %w", err)
	}

	scored := make([]types.ScoredChunk, 0, len(results))
	for _, r := range results {
		payload := r.Record.Payload
		scored = append(scored, types.ScoredChunk{
			Chunk: types.Chunk{
				Text:     getStr(payload, "text"),
				Source:   getStr(payload, "source"),
				Metadata: toStringMap(payload),
			},
			Score: float64(r.Score),
		})
	}

	return scored, nil
}

// getStr extracts a string value from a map[string]any, returning "" on
// missing or non-string values.
func getStr(m map[string]any, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// toStringMap converts a map[string]any into map[string]string, silently
// dropping non-string values. Only string-typed payload values are kept.
func toStringMap(m map[string]any) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
}
