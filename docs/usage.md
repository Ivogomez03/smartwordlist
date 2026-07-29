# Usage: SmartWordlist

Generate contextual password wordlists for authorized security assessments.

## Installation

### From source

```bash
git clone https://github.com/Ivogomez03/smartwordlist.git
cd smartwordlist
go install ./cmd/smartwordlist
```

### go install

```bash
go install github.com/Ivogomez03/smartwordlist/cmd/smartwordlist@latest
```

Requires **Go 1.23+**.

## Quickstart

```bash
# Rule-only mode (works without Ollama)
smartwordlist example.com --no-llm --max 1000 -o wordlist.txt

# LLM-enhanced mode (requires Ollama running locally)
smartwordlist example.com -o wordlist.txt --verbose
```

**Terminal output example:**

```
SmartWordlist v0.1.0 — contextual password wordlist generator

[i] Target domain: example.com
[i] Rules file: defaults/rules.yaml
[+] Ollama connected — LLM-enhanced generation active
[i] Starting reconnaissance...
[+] Title: Example Corp | Industry Leader
[+] Company: Example Corp
[i] Technologies: 3
[i] Keywords: 12
[i] Subdomains: 2
[i] Emails: 1
[i] Paths: 4
[i] Generating candidates...
[i] Mutations: +342 new variants
[i] Combinations: +156 new variants
[i] Scoring + dedup: 1023 → 847 candidates

[+] Done! Generated 847 unique candidates (from 1023 raw)
[i] Generation time: 2.1s
[i] Total time: 8.3s
[i] Sources: [llm rule-mutation combo]
[+] Output written to: wordlist.txt
```

## Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `<domain>` | (positional) | *required* | Target domain for reconnaissance |
| `--output` | `-o` | stdout | Output file for wordlist |
| `--max` | `-m` | 0 (unlimited) | Maximum candidates to generate |
| `--verbose` | `-v` | false | Show detailed phase timing |
| `--no-llm` | — | false | Force rule-only mode |
| `--rules` | `-r` | `defaults/rules.yaml` | Custom mutation rules file |
| `--json` | — | `<output>.json` | JSON metadata output path |

## Output Formats

### Plain Text (`--output`)

One candidate per line:

```
Acme2026
acme123
4dm1n!
AdminWidget
CloudAcme
...
```

### JSON (`--json`)

Structured metadata including scores, sources, and generation stats:

```json
{
  "total": 847,
  "generated": 1023,
  "deduplicated": 176,
  "generation_time_ms": 2147,
  "sources_used": ["llm", "rule-mutation", "combo"],
  "mutation_counts": {"mutated": 342, "combos": 156},
  "candidates": [
    {"word": "Acme2026", "score": 10.5, "source": "llm"},
    ...
  ]
}
```

## Rules Customization

Create a custom YAML file to tune mutation behaviour:

```yaml
leet_map:
  a: ["4", "@"]
  e: ["3"]
  i: ["1", "!"]
  o: ["0"]
  s: ["5", "$"]

suffixes:
  - "123"
  - "!"
  - "@"
  - "2026"

prefixes:
  - "admin_"
  - "dev_"

year_range:
  start: 2020
  end: 2026

case_variations:
  - lower
  - upper
  - title
```

Use with: `smartwordlist example.com --rules my-rules.yaml`

TOML format is also supported: `smartwordlist example.com --rules my-rules.toml`

## Mode Selection

| Condition | Mode | Generation |
|-----------|------|------------|
| Ollama running + `--no-llm` not set | LLM-enhanced | RAG → LLM prompt → candidates |
| Ollama down or `--no-llm` flag set | Rule-only | Recon data → mutation engine → candidates |

The tool automatically degrades to rule-only mode when Ollama is unreachable.

## Troubleshooting

| Problem | Solution |
|---------|----------|
| `ollama: connection refused` | Start Ollama: `ollama serve` |
| `model not found` | Pull the model: `ollama pull qwen3:0.6b && ollama pull nomic-embed-text` |
| `all collectors failed` | Check network connectivity; try with `--verbose` to see per-collector errors |
| Slow reconnaissance | DNS enumeration probes many subdomains; use `--verbose` to track progress |
| Large memory usage | veclite stores vectors in memory; limit with `--max` to cap candidate count |
