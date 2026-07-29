# scoring-export Specification

## Purpose

Candidate deduplication, scoring, probability-based sorting, plain-text wordlist output, and JSON metadata export.

## Requirements

### Requirement: Deduplication

The system MUST deduplicate candidates case-insensitively by default.

#### Scenario: Duplicate candidates

- GIVEN candidates ["Password", "password", "PASSWORD"]
- WHEN deduplication runs
- THEN only one entry (first encountered case) remains

### Requirement: Candidate Scoring

Each candidate MUST be scored by: source (LLM > rule > dictionary), mutation count (fewer = higher), RAG relevance, and pattern complexity.

#### Scenario: Scoring comparison

- GIVEN an LLM-generated candidate "AcmeCorp2026!" and a dict candidate "password"
- WHEN scoring executes
- THEN the LLM candidate receives a higher score

### Requirement: Sorted Output

The system MUST sort output by descending score.

#### Scenario: Mixed-quality candidates

- GIVEN scored candidates with values [0.85, 0.42, 0.91, 0.30]
- WHEN sorting runs
- THEN the output order is [0.91, 0.85, 0.42, 0.30]

### Requirement: Plain-Text Wordlist

Output MUST include a plain-text wordlist file with one candidate per line.

#### Scenario: Export wordlist

- GIVEN 1000 scored and sorted candidates
- WHEN export writes the wordlist
- THEN each line contains exactly one password

### Requirement: JSON Metadata

Metadata MUST be exported as JSON: total candidates, per-candidate scores, sources, and statistics.

#### Scenario: Metadata correctness

- GIVEN generation completed with 500 LLM + 500 rule candidates
- WHEN JSON metadata is written
- THEN the file includes generation_time, source_counts, and per-candidate score entries

### Requirement: Output Size Limit

The system SHALL respect `--max` to limit output entries.

#### Scenario: Max limit on export

- GIVEN `--max 200` and 500 generated candidates
- WHEN export runs
- THEN exactly 200 entries (top-scoring) are written
