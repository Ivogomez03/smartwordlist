# candidate-generation Specification

## Purpose

LLM-powered candidate generation via RAG context, semantic rules engine, mutation engine (leet, case, suffixes, prefixes, years), and base dictionary combinations.

## Requirements

### Requirement: LLM Prompt Construction

The system MUST construct an LLM prompt using RAG-retrieved context and generation rules template.

#### Scenario: Prompt with full context

- GIVEN RAG returns company "Acme Corp", tech "React", location "Berlin"
- WHEN the prompt is assembled
- THEN it includes all three context items and instructions for pattern-based generation

### Requirement: Rule-Only Fallback

The system MUST support rule-only fallback when Ollama is unavailable, using predefined generation rules without LLM.

#### Scenario: Ollama not running

- GIVEN `--no-llm` flag or Ollama unreachable
- WHEN generation starts
- THEN candidates are produced using only mutation rules and dictionaries

### Requirement: Mutation Engine

Generated words MUST be mutated via: leet (a→4, e→3), case variations (upper, lower, title, camel), suffixes/prefixes (123, !, 2026, @), and year ranges.

#### Scenario: Leet and suffix mutation

- GIVEN a base word "acme" and current year 2026
- WHEN the mutation engine runs
- THEN output includes "4cm3", "Acme2026", "acme!"

### Requirement: Dictionary Combinations

The system MUST combine base dictionary words with contextual mutations.

#### Scenario: Season + company combo

- GIVEN company "Acme" and dictionary containing "Summer"
- WHEN dictionary combination executes
- THEN candidates include "SummerAcme", "AcmeSummer2026"

### Requirement: Generation Limits

The system SHALL support configurable generation limits via `--max`.

#### Scenario: Max limit enforced

- GIVEN `--max 500`
- WHEN generation completes
- THEN no more than 500 candidates enter the scoring phase
