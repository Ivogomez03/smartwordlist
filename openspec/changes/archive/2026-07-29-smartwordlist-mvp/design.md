# Design: SmartWordlist MVP

## Technical Approach

Pipeline architecture: independent packages communicating through typed channels. Each stage (`recon → extract → embed → retrieve → generate → score → export`) is a composable function taking `context.Context` and typed I/O channels. A root `Pipeline` struct in `cmd/smartwordlist` chains stages at startup. Config flows via a `Config` struct injected into each stage constructor — no global state. Greenfield project; follows standard Go project layout (`cmd/`, `internal/`, `pkg/`).

## Architecture Decisions

| Decision | Choice | Alternatives | Rationale |
|----------|--------|-------------|-----------|
| Pipeline pattern | Channel-based staged functions: `func(ctx, in <-chan T, out chan<- U) error` | Callback chain (loses backpressure), interface chain (more boilerplate), single struct (untestable) | Channels give natural backpressure, context cancellation, stage-level test isolation |
| Error propagation | Partial failure tolerance per stage; fatal errors trigger graceful degradation; wrapped with `fmt.Errorf("stage %s: %w", name, err)` | Fail-fast (loses partial recon data), error channel (over-engineered for linear pipeline) | Recon must continue after one DNS failure; Ollama failure must degrade to rule-only |
| Concurrency | Recon: colly Async + goroutine per collector (fan-in). Generation: bounded worker pool. Embedding: sequential (Ollama bottleneck) | Single-threaded (too slow for HTTP+DNS), unbounded goroutines (resource risk) | colly handles HTTP concurrency natively; DNS lookups benefit from parallelism; Ollama is single-model, sequential is correct |
| veclite persistence | File-backed with domain-hash key. Schema: `{id, vector, metadata: {source, chunk_text, timestamp}}`. Cache loaded on subsequent runs | In-memory only (no reuse), external DB (overkill for CLI) | Spec requires caching; veclite's built-in file persistence is zero-config |
| Ollama abstraction | Thin `net/http` wrapper. Health: `GET /api/tags`. Embed: `POST /api/embed`. Generate: `POST /api/generate` (streaming). Config via Viper | SDK dependency (webdock-io/ollama-go-sdk adds external dep with little benefit for 3 endpoints) | Ollama API is simple REST; stdlib `net/http` avoids dependency lock-in and trims binary size |
| Embedded dictionaries | `//go:embed dict/data/*.txt` in `pkg/dict/embed.go`, loaded into `map[string][]string` at startup | External files (requires install path), hardcoded strings (unmaintainable) | `go:embed` produces self-contained binary with zero runtime I/O; dictionaries are small (~10KB) |
| Plugin panic safety | `recover()` wrapper at each plugin call boundary: `func safeCall(fn func() error) error { defer recover(); return fn() }` | Separate process (high overhead, complex IPC for MVP), no isolation (one bad plugin crashes tool) | `recover()` is zero-cost when no panic; separate process adds latency and serialization burden for a CLI tool |

## Data Flow

```
[CLI: Cobra + Config]
       │
       ▼
┌──────────────────────────────────────────────┐
│ ReconCollector                               │
│  ├ colly.HTMLCollector  → ReconResult        │
│  ├ DNSEnumerator        → []Subdomain        │
│  └ RobotsParser          → []Path            │
└──────────────┬───────────────────────────────┘
               │ ReconResult
               ▼
┌──────────────────────────────────────────────┐
│ Chunker → []Chunk{Text, Source, Meta}        │
└──────────────┬───────────────────────────────┘
               │ []Chunk
               ▼
┌──────────────────────────────────────────────┐
│ OllamaEmbedder → [][]float32                 │
│ veclite.Index → stored vectors               │
└──────────────┬───────────────────────────────┘
               │
               ▼
┌──────────────────────────────────────────────┐
│ ContextRetriever.Search("wordlist query", 5) │
│ → []ScoredChunk (top-K)                      │
└──────────────┬───────────────────────────────┘
               │ []ScoredChunk
               ▼
┌──────────────────────────────────────────────┐
│ Generator                                    │
│  ├ LLMGenerator (if Ollama healthy)          │
│  └ RuleGenerator (fallback)                  │
│       │                                       │
│       ▼                                       │
│  MutationEngine.Mutate(word) → []string      │
└──────────────┬───────────────────────────────┘
               │ []Candidate{Word, Source}
               ▼
┌──────────────────────────────────────────────┐
│ Scorer.{Score, Deduplicate, Sort}            │
│ → []ScoredCandidate{Word, Score, Source}     │
└──────────────┬───────────────────────────────┘
               │ []ScoredCandidate
               ▼
┌──────────────────────────────────────────────┐
│ Exporter                                     │
│  ├ ExportText    → wordlist.txt              │
│  └ ExportJSON    → metadata.json             │
└──────────────────────────────────────────────┘
```

## Key Interfaces

```go
// Recon — collects target intelligence
type ReconCollector interface {
    Collect(ctx context.Context, domain string) (*ReconResult, error)
}

// Embedding — generates vectors from text
type EmbeddingProvider interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
    Dims() int
}

// RAG — indexes and retrieves chunks
type ContextRetriever interface {
    Search(ctx context.Context, query string, k int) ([]ScoredChunk, error)
    Index(ctx context.Context, chunks []Chunk) error
    LoadCache(domain string) (bool, error)
    SaveCache(domain string) error
}

// Generation — produces candidate passwords
type CandidateGenerator interface {
    Generate(ctx context.Context, chunks []ScoredChunk, max int) ([]Candidate, error)
}

// Mutation — applies leet/case/suffix transforms
type MutationEngine interface {
    Mutate(word string) []string
}

// Scoring — deduplicates and ranks
type Scorer interface {
    Score([]Candidate) []ScoredCandidate
    Deduplicate([]ScoredCandidate) []ScoredCandidate
}

// Export — writes results
type Exporter interface {
    ExportText([]ScoredCandidate, io.Writer) error
    ExportJSON([]ScoredCandidate, Stats, io.Writer) error
}
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `cmd/smartwordlist/main.go` | Create | Cobra root, flag parsing, pipeline orchestration, Ollama health check |
| `internal/cli/styles.go` | Create | Lip Gloss theme (banner, colors, progress bar model) |
| `internal/recon/collector.go` | Create | ReconCollector orchestration + ReconResult struct |
| `internal/recon/scrape.go` | Create | colly HTML scraper: title, keywords, tech detection, emails |
| `internal/recon/dns.go` | Create | DNS subdomain enumeration + crt.sh fallback |
| `internal/recon/robots.go` | Create | robots.txt + sitemap.xml fetcher |
| `internal/rag/chunker.go` | Create | ReconResult → []Chunk: semantic sections |
| `internal/rag/veclite.go` | Create | veclite init, index, HNSW search, read/write cache |
| `internal/rag/embedder.go` | Create | OllamaEmbedder implementing EmbeddingProvider |
| `internal/generation/llm.go` | Create | LLMGenerator: prompt assembly + Ollama generate call |
| `internal/generation/rules.go` | Create | RuleGenerator: mutation-only fallback |
| `internal/generation/mutate.go` | Create | MutationEngine: leet, case, suffix, year combos |
| `internal/generation/combo.go` | Create | Dictionary word + context word combinations |
| `internal/scoring/scorer.go` | Create | Scorer: source-weighted scoring, case-insensitive dedup, sort |
| `internal/export/writer.go` | Create | Plain text (one per line) + JSON metadata (scores, sources, stats) |
| `internal/ollama/client.go` | Create | HTTP client: health, embed, generate (streaming), timeout/retry |
| `internal/plugin/loader.go` | Create | YAML/TOML rule loader + validation, `--rules` flag support |
| `internal/plugin/native.go` | Create | Go plugin (.so) loader with `recover()` isolation |
| `pkg/types/config.go` | Create | Config, ReconResult, Candidate, ScoredCandidate, Chunk, Stats |
| `pkg/dict/embed.go` | Create | `//go:embed` for base dictionaries |
| `pkg/dict/data/common.txt` | Create | Embedded common password dictionary |
| `pkg/dict/data/seasons.txt` | Create | Embedded season/year/pattern dictionary |
| `defaults/rules.yaml` | Create | Default mutation rules: leet map, suffixes, prefixes, year range |
| `go.mod` | Create | Module `github.com/Ivogomez03/smartwordlist` |

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | MutationEngine combinations | Table-driven: input word → expected []string; golden files |
| Unit | Scorer dedup + sort | Input/output slices; property: output length ≤ input |
| Unit | Rule parser validation | Valid + invalid YAML fixture files |
| Unit | Plugin panic recovery | Inject panicking mock plugin; assert error, not crash |
| Integration | Full pipeline `--no-llm` | `httptest.NewServer` as fake target domain; assert wordlist > 100 entries |
| Integration | veclite index + search | In-memory DB, known vectors, assert top-K correctness |
| Integration | Ollama real call | Guard with `OLLAMA_TEST=1` env; test embed + generate with `nomic-embed-text` |
| E2E | CLI golden output | `smartwordlist test.local --no-llm --max 100` → diff against golden file |

## Migration / Rollout

Greenfield — no migration required. The modular interface design allows replacing any single package (veclite → govector, Ollama HTTP → SDK) without touching others. Rule-only mode ensures the tool is always usable regardless of Ollama availability.

## Open Questions

- [ ] Cache directory: `~/.cache/smartwordlist/` (XDG) vs `./.smartwordlist/`? Recommend XDG.
- [ ] Default embedding model: `nomic-embed-text` (768d, better quality) vs `all-minilm` (384d, faster)? Recommend `nomic-embed-text`.
- [ ] Native Go plugin (.so) support for v0.1 or defer to v0.2? Recommend: YAML/TOML only for MVP; Go plugins add build-compatibility burden.
