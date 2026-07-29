# Architecture: SmartWordlist

Pipeline architecture for generating contextual password wordlists.

## Pipeline Diagram

```
                        ┌─────────────────────────┐
                        │   CLI: Cobra + Config    │
                        │   cmd/smartwordlist/     │
                        └────────────┬────────────┘
                                     │ domain, flags
                                     ▼
                 ┌───────────────────────────────────┐
                 │  Reconnaissance                   │
                 │  internal/recon/                  │
                 │  ├ HTML scraper (colly)           │
                 │  ├ DNS enumeration + crt.sh       │
                 │  └ robots.txt / sitemap           │
                 └───────────────┬───────────────────┘
                                 │ ReconResult
                                 ▼
                 ┌───────────────────────────────────┐
                 │  Ollama Health Check              │
                 │  internal/ollama/                 │
                 │  GET /api/tags → healthy?         │
                 └──────┬──────────────┬─────────────┘
                        │ healthy      │ unhealthy / --no-llm
                        ▼              ▼
          ┌──────────────────┐  ┌──────────────────────┐
          │  RAG Pipeline    │  │  Rule-Only Pipeline   │
          │  internal/rag/   │  │  internal/generation/ │
          │                  │  │                        │
          │  1. Chunk Recon  │  │  1. Extract context   │
          │  2. Embed (Oll)  │  │     words from recon  │
          │  3. Index veclite│  │  2. Apply mutations   │
          │  4. Search top-K │  │  3. Generate combos   │
          │  5. LLM Generate │  │                        │
          │     (Ollama)     │  │                        │
          └────────┬─────────┘  └───────────┬────────────┘
                   │                        │
                   └────────┬───────────────┘
                            │ []Candidate
                            ▼
              ┌──────────────────────────┐
              │  Mutation Engine         │
              │  internal/generation/    │
              │  ├ Leet substitution     │
              │  ├ Case variations       │
              │  ├ Suffix / Prefix       │
              │  └ Year combinations     │
              └───────────┬──────────────┘
                          │ []Candidate
                          ▼
              ┌──────────────────────────┐
              │  Dictionary Combos       │
              │  internal/generation/    │
              │  dict × context words    │
              └───────────┬──────────────┘
                          │ []Candidate
                          ▼
              ┌──────────────────────────┐
              │  Scoring                 │
              │  internal/scoring/       │
              │  ├ Source-weighted       │
              │  │ LLM=10 > Rule=5 >     │
              │  │ Dict=3 > Combo=2      │
              │  ├ Length bonus          │
              │  ├ Complexity bonus      │
              │  └ Dedup (case-insens)   │
              └───────────┬──────────────┘
                          │ []ScoredCandidate
                          ▼
              ┌──────────────────────────┐
              │  Export                  │
              │  internal/export/        │
              │  ├ ExportText (plain)    │
              │  └ ExportJSON (metadata) │
              └──────────────────────────┘
```

## Package Structure

| Package | Path | Responsibility |
|---------|------|----------------|
| `main` | `cmd/smartwordlist/` | CLI entry, flag parsing, pipeline orchestration |
| `cli` | `internal/cli/` | Lip Gloss styles, banner, progress model |
| `recon` | `internal/recon/` | Reconnaissance: HTML scrape, DNS, robots/sitemap |
| `rag` | `internal/rag/` | RAG: chunker, veclite store, Ollama embedder |
| `ollama` | `internal/ollama/` | HTTP client for Ollama API (health, embed, generate) |
| `generation` | `internal/generation/` | LLM generation, rule-based generation, mutation engine, combos |
| `scoring` | `internal/scoring/` | Source-weighted scoring, deduplication, sorting |
| `export` | `internal/export/` | Plain text and JSON export writers |
| `plugin` | `internal/plugin/` | YAML/TOML rule loading, native plugin isolation |
| `types` | `pkg/types/` | Shared data structures (Config, ReconResult, Candidate, etc.) |
| `dict` | `pkg/dict/` | Embedded password dictionaries via `//go:embed` |

## Key Interfaces

```go
type ReconCollector interface {
    Collect(ctx context.Context, domain string) (*ReconResult, error)
}

type EmbeddingProvider interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    Dims() int
}

type ContextRetriever interface {
    Search(ctx context.Context, domain string, queryVector []float32, k int) ([]ScoredChunk, error)
    Index(ctx context.Context, domain string, chunks []Chunk, vectors [][]float32) error
}

type CandidateGenerator interface {
    Generate(ctx context.Context, chunks []ScoredChunk, max int) ([]Candidate, error)
}
```

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Pipeline pattern | Channel-based stages with context propagation | Backpressure, cancellation, test isolation |
| Error handling | Partial failure tolerance; degrade gracefully | Recon one failure ≠ total failure; Ollama down → rule-only |
| Ollama integration | Stdlib `net/http` wrapper | 3 endpoints; SDK adds dependency bloat |
| Dictionaries | `//go:embed` | Self-contained binary, zero runtime I/O |
| Vector DB | veclite (HNSW, cosine) | Zero-config, file-backed persistence |
| Plugin system | YAML/TOML for v0.1; Go .so deferred to v0.2 | YAML/TOML covers 95% of use cases |
| Panic safety | `recover()` at plugin boundary | Zero-cost when no panic; separate process overkill for CLI |

## Data Flow

1. **Recon** → fan-in goroutines (HTML, DNS, robots) → merged `ReconResult`
2. **RAG path** (LLM mode): `ReconResult` → `Chunker` → `OllamaEmbedder` → `VecliteStore` → search → `LLMGenerator`
3. **Rule path** (fallback): `ReconResult` → `RuleGenerator` with mutation engine
4. **Post-generation**: mutation engine on all candidates → dictionary combos → scoring → dedup → export

## Testing Strategy

- **Unit**: Table-driven tests for mutation engine, scorer, plugin loader
- **Integration**: veclite index+search with synthetic vectors, httptest for reconnaissance
- **E2E**: `--no-llm` pipeline with test HTTP server; skips in `-short` mode
