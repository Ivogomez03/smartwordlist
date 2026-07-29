# Proposal: SmartWordlist MVP

## Intent

Build the first working version of SmartWordlist — an open-source CLI tool that generates contextual password wordlists for authorized security assessments. The tool scrapes target context (recon), enriches it via RAG + local LLM (Ollama), and produces scored, deduplicated wordlists. No comparable tool combines recon → RAG → generation in a single binary today.

## Scope

### In Scope
- Go CLI with sqlmap-style colored output (Cobra + Charm stack)
- Reconnaissance pipeline: HTML scraping, title/keyword extraction, tech detection, subdomain enum, robots.txt/sitemap, email harvesting
- Embedded RAG: veclite vector store + Ollama embeddings for context retrieval
- Ollama integration: HTTP client with streaming, multi-model support (Qwen3, Llama 3.1, Gemma), auto-detection fallback
- Candidate generation: semantic rules, mutation engine (leet, case, year/season combos), base dictionaries embedded
- Scoring, deduplication, and export (plain wordlist + JSON metadata)
- YAML/TOML rule configuration + Go plugin system for extensibility
- Cross-platform static binaries (Linux, macOS, Windows)
- Documentation: architecture docs, usage guides, README

### Out of Scope
- Cloud-hosted LLM providers (OpenAI, Anthropic) — local-only for MVP
- GUI / web interface
- Distributed/clustered execution
- Password hash cracking (generation only, not testing)
- CI/CD pipeline and release automation (deferred to post-MVP)

## Capabilities

> Contract with sdd-spec. `openspec/specs/` is empty — all capabilities are new.

### New Capabilities
- `cli-core`: CLI framework — Cobra commands, argument parsing, Charm-stack colored output (Lip Gloss), progress bars (Bubble Tea), banner display
- `reconnaissance`: HTTP fetching (colly), HTML parsing, title/company/keyword extraction, tech detection, DNS/subdomain enumeration, robots.txt/sitemap.xml parsing, email harvesting, structured JSON output
- `embeddings-rag`: veclite embedded vector DB, Ollama embedding client, text chunking strategy, similarity search, context retrieval pipeline before generation
- `candidate-generation`: LLM-powered generation via RAG context, semantic rules engine, mutation engine (leet, case, suffixes, prefixes, years), base dictionary combos (common passwords, seasons, years, keyboard patterns)
- `scoring-export`: Deduplication, candidate scoring algorithm, probability-based sorting, plain-text wordlist output, JSON metadata output (scores, sources, statistics)
- `ollama-provider`: Ollama HTTP client, model availability detection, streaming responses, fallback to rule-only mode when unavailable, multi-model support
- `plugin-system`: YAML/TOML rule file loading, Go plugin interface for custom collectors and providers

### Modified Capabilities
None — greenfield project.

## Approach

Go, as determined by the [language exploration](../smartwordlist-lang-choice/exploration.md) (Go wins 9-2). Modular architecture with clean interfaces between pipeline stages: `recon → extract → embed → retrieve → generate → score → export`. Each stage is an independent package. Hybrid LLM mode: auto-detect Ollama at startup; if unavailable, degrade gracefully to rule-based generation with a visible warning. Base dictionaries embedded in the binary via `go:embed`.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `cmd/` | New | CLI entry points, Cobra command tree |
| `internal/recon/` | New | Reconnaissance pipeline modules |
| `internal/rag/` | New | veclite integration, embedding, retrieval |
| `internal/generation/` | New | Candidate generation + mutation engine |
| `internal/scoring/` | New | Scoring, dedup, sorting |
| `internal/export/` | New | Wordlist + JSON output writers |
| `internal/ollama/` | New | Ollama HTTP client + model management |
| `internal/plugin/` | New | YAML/TOML loader + Go plugin interface |
| `internal/cli/` | New | Charm-stack UI components (styles, progress, banners) |
| `pkg/` | New | Shared types, interfaces, base dictionaries |
| `docs/` | New | Architecture and usage documentation |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| veclite pre-1.0 API instability | Med | Pin version; govector as fallback |
| Ollama unavailable on target machine | High | Auto-detect + graceful rule-only fallback |
| LLM generation quality varies by model | Med | Test against Qwen3/Llama3.1/Gemma; model-specific prompts |
| Large recon output overwhelms RAG context | Low | Chunking strategy with configurable limits |

## Rollback Plan

Greenfield project — no existing functionality to break. If MVP proves unworkable: (1) the modular architecture allows replacing any single module without touching others, (2) rule-only mode ensures the tool is always usable even without Ollama, (3) veclite can be swapped for govector behind the same interface.

## Dependencies

- Go 1.23+ toolchain
- Ollama (optional, for LLM-enhanced generation)
- Network access to target domain (for reconnaissance)

## Success Criteria

- [ ] `smartwordlist example.com` produces a contextual wordlist of 1,000+ entries
- [ ] Recon pipeline extracts title, keywords, tech stack, emails, subdomains
- [ ] RAG retrieval improves wordlist relevance vs rule-only baseline
- [ ] Rule-only fallback works when Ollama is not running
- [ ] Single binary runs on Linux, macOS, and Windows
- [ ] JSON output includes scores, sources, and generation statistics
- [ ] YAML rule files allow customizing mutation rules without recompiling
