# ollama-provider Specification

## Purpose

Ollama HTTP client with model availability detection, streaming responses, multi-model support (Qwen3, Llama 3.1, Gemma), and graceful fallback to rule-only mode.

## Requirements

### Requirement: Auto-Detection at Startup

The system MUST health-check the Ollama API at startup to determine availability.

#### Scenario: Ollama running

- GIVEN Ollama is running on `localhost:11434`
- WHEN the tool starts
- THEN the LLM pipeline is activated and model list is queried

#### Scenario: Ollama unreachable

- GIVEN Ollama is not running
- WHEN the health check fails
- THEN the system falls back to rule-only mode with a visible warning

### Requirement: Model Configuration

The system MUST support configurable model IDs for generation, fallback, and embedding.

#### Scenario: Custom model selection

- GIVEN config specifies `qwen3:7b` as generation model
- WHEN generation starts
- THEN that specific model is used for the LLM call

### Requirement: Multi-Model Support

Qwen3, Llama 3.1, and Gemma model families MUST be supported.

#### Scenario: Llama model used

- GIVEN `llama3.1:8b` is pulled and available
- WHEN generation with that model is requested
- THEN the provider sends requests to the correct Ollama endpoint

### Requirement: Streaming Responses

The provider MUST handle streaming responses for progressive generation output.

#### Scenario: Streamed generation

- GIVEN a generation request is sent
- WHEN the response returns as NDJSON stream
- THEN candidates are collected as they arrive from the model

### Requirement: Graceful Degradation

If Ollama is unavailable or the model is not found, the system MUST fall back to rule-only mode.

#### Scenario: Model not pulled

- GIVEN Ollama is running but the requested model is not present
- WHEN the API returns 404
- THEN a warning is shown and rule-only generation proceeds

### Requirement: Dry-Run Mode

The provider SHALL enable connectivity testing without generation.

#### Scenario: Dry-run test

- GIVEN `smartwordlist example.com --dry-run-ollama`
- WHEN executed
- THEN API connectivity is tested and reported without generating candidates

### Requirement: Timeout and Retry

API calls SHALL use configurable timeout and retry settings.

#### Scenario: Slow Ollama response

- GIVEN a generation request exceeding 30s timeout
- WHEN the deadline is reached
- THEN the call is cancelled and the fallback is triggered
