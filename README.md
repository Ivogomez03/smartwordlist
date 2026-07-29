# SmartWordlist

> Contextual password wordlist generator for authorized security assessments.

[![Go Version](https://img.shields.io/badge/Go-1.23+-00ADD8?style=flat&logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Version](https://img.shields.io/badge/version-0.1.0-blue)](https://github.com/Ivogomez03/smartwordlist)

SmartWordlist combines **reconnaissance**, **RAG (Retrieval-Augmented Generation)**, and **local LLM generation** to produce targeted password wordlists for penetration testing and red team engagements.

## Quick Install

```bash
go install github.com/Ivogomez03/smartwordlist/cmd/smartwordlist@latest
```

## Quick Example

```bash
# Rule-only mode — works out of the box
smartwordlist example.com --no-llm --max 1000 -o wordlist.txt

# LLM-enhanced mode — requires Ollama
smartwordlist example.com -o wordlist.txt --verbose
```

```
SmartWordlist v0.1.0 — contextual password wordlist generator

[i] Target domain: example.com
[+] Ollama connected — LLM-enhanced generation active
[i] Starting reconnaissance...
[+] Title: Example Corp | Industry Leader
[+] Company: Example Corp
[i] Technologies: 3 | Keywords: 12 | Subdomains: 2 | Emails: 1 | Paths: 4
[+] Done! Generated 847 unique candidates (from 1023 raw)
[+] Output written to: wordlist.txt
```

## Features

- **Reconnaissance** — HTML scraping, DNS enumeration, crt.sh lookup, robots.txt/sitemap discovery
- **RAG Pipeline** — Chunking, embedding (nomic-embed-text), vector search with HNSW
- **LLM Generation** — Context-aware prompts via Ollama, streaming response
- **Rule-Based Fallback** — Algorithmic generation when Ollama is unavailable
- **Mutation Engine** — Leet substitution, case variations, suffix/prefix, year patterns
- **Dictionary Combos** — Embedded dictionaries × context word combinations
- **Smart Scoring** — Source-weighted scoring with length and complexity bonuses
- **Multiple Export Formats** — Plain text wordlist + JSON metadata
- **Plugin System** — Custom YAML/TOML rule files for mutation tuning
- **Zero External Dependencies** — Embedded dictionaries, single binary

## Architecture

See [docs/architecture.md](docs/architecture.md) for the full pipeline diagram, data flow, and design decisions.

## Usage

See [docs/usage.md](docs/usage.md) for installation, flags, output formats, rules customization, and troubleshooting.

## Requirements

- **Go 1.23+** (to build from source)
- **Ollama** (optional, for LLM-enhanced generation)
  - `qwen3:0.6b` — text generation model
  - `nomic-embed-text` — embedding model

## ⚠️ Disclaimer

SmartWordlist is intended **exclusively for authorized security assessments**. Do not use this tool against systems you do not own or have explicit permission to test. Unauthorized use is illegal in most jurisdictions. The authors assume no liability for misuse.

## License

[MIT](LICENSE)
