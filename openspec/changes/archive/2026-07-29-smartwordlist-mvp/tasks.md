# Tasks: SmartWordlist MVP

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Changed lines | 1800–2500 (24 new files) |
| 400-line budget risk | High |
| Chained PRs | Yes |
| Strategy | auto-forecast / pending |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: pending
400-line budget risk: High

PR slices (stacked-to-main or feature-branch-chain, tracker `feat/smartwordlist`): PR1 foundation → PR2 ollama+dicts → PR3 recon → PR4 rag → PR5 generation → PR6 scoring+export+plugin → PR7 wiring+tests+docs.

## Phase 1: Foundation (PR 1)

- [x] 1.1 Create `go.mod` — module `github.com/Ivogomez03/smartwordlist`, Go 1.23+, deps colly/cobra/viper/lipgloss/bubbletea/veclite/chroma/yaml.v3
- [x] 1.2 Create `pkg/types/config.go` — `Config`, `ReconResult`, `Candidate`, `ScoredCandidate`, `Chunk`, `ScoredChunk`, `Stats` types
- [x] 1.3 Create `internal/cli/styles.go` — Lip Gloss banner, sqlmap theme, progress
- [x] 1.4 Create `defaults/rules.yaml` — leet map, suffixes, prefixes, year range 2015–2026
- [x] 1.5 Create `cmd/smartwordlist/main.go` — Cobra root, `<domain>` arg, all cli-core flags

## Phase 2: Ollama + Embedded Dictionaries (PR 2)

- [x] 2.1 Create `internal/ollama/client.go` — health, embed, streaming generate
- [x] 2.2 Implement 404 model-not-found → rule-only fallback with warning
- [x] 2.3 Create `pkg/dict/embed.go` with `//go:embed dict/data/*.txt` loader
- [x] 2.4 Create `pkg/dict/data/common.txt` (~500 common passwords)
- [x] 2.5 Create `pkg/dict/data/seasons.txt` (seasons, months, years 2015–2026, walks)

## Phase 3: Reconnaissance (PR 3)

- [x] 3.1 Create `internal/recon/collector.go` — `ReconCollector` orchestrator, fan-in goroutines
- [x] 3.2 Create `internal/recon/scrape.go` — colly HTML: title, meta, keywords, tech, emails
- [x] 3.3 Create `internal/recon/dns.go` — DNS subdomain enum + crt.sh fallback
- [x] 3.4 Create `internal/recon/robots.go` — robots.txt + sitemap fetcher

## Phase 4: RAG (PR 4)

- [x] 4.1 Create `internal/rag/chunker.go` — `Chunker.Chunk(*ReconResult) []Chunk`
- [x] 4.2 Create `internal/rag/veclite.go` — 768d, HNSW, cache `~/.cache/smartwordlist/{sha256(domain)}.veclite`
- [x] 4.3 Create `internal/rag/embedder.go` — `OllamaEmbedder` (nomic-embed-text, 768d)

## Phase 5: Generation (PR 5)

- [x] 5.1 Create `internal/generation/llm.go` — prompt assembly + streaming Ollama generate
- [x] 5.2 Create `internal/generation/rules.go` — rule-only fallback
- [x] 5.3 Create `internal/generation/mutate.go` — leet, case, suffix/prefix (!, 123, @, year)
- [x] 5.4 Create `internal/generation/combo.go` — dict × context word combinations

## Phase 6: Scoring + Export + Plugin (PR 6)

- [x] 6.1 Create `internal/scoring/scorer.go` — source-weighted score, dedup, descending sort
- [x] 6.2 Create `internal/export/writer.go` — `ExportText` + `ExportJSON`
- [x] 6.3 Create `internal/plugin/loader.go` — YAML/TOML loader, errors, unknown-key warnings
- [x] 6.4 Create `internal/plugin/native.go` — Go .so loader + `recover()` isolation

## Phase 7: Wiring + Tests + Docs (PR 7)

- [x] 7.1 Wire `cmd/smartwordlist/main.go` — full pipeline + progress + `--max` truncation
- [x] 7.2 Add `internal/generation/mutate_test.go` — table-driven mutations
- [x] 7.3 Add `internal/scoring/scorer_test.go` — dedup + sort invariants
- [x] 7.4 Add `internal/plugin/loader_test.go` — valid/invalid YAML + panic isolation
- [x] 7.5 Add `internal/recon/collector_test.go` + `internal/rag/veclite_test.go` — httptest + in-memory
- [x] 7.6 Add `cmd/smartwordlist/main_test.go` — E2E golden for `--no-llm --max 100`
- [x] 7.7 Create `docs/architecture.md` + `docs/usage.md` (pipeline, quickstart, flags)
- [x] 7.8 Update `README.md` — description, install, usage, architecture link
