# embeddings-rag Specification

## Purpose

veclite embedded vector DB with Ollama embedding client, semantic text chunking, similarity search, and context retrieval for generation.

## Requirements

### Requirement: Vector Store Initialization

The system MUST initialize veclite with a configurable dimension matching the Ollama embedding model.

#### Scenario: Default initialization

- GIVEN an embedding model with 768 dimensions
- WHEN the RAG pipeline starts
- THEN veclite is initialized and ready for insertions

### Requirement: Semantic Text Chunking

The system MUST chunk recon JSON into semantic blocks: company info, tech stack, paths, emails.

#### Scenario: Recon JSON with multiple sections

- GIVEN recon output with company name, 3 technologies, 5 paths, and 2 emails
- WHEN chunking runs
- THEN at least 4 chunks are produced (one per section), each with metadata

### Requirement: Embedding Generation and Storage

The system MUST generate embeddings via Ollama for each chunk and store them in veclite.

#### Scenario: Successful embedding

- GIVEN chunked text blocks and a running Ollama instance
- WHEN embedding generation executes
- THEN each chunk has a corresponding vector stored in veclite

### Requirement: Similarity Search

The system MUST support similarity search returning top-K relevant chunks for a query.

#### Scenario: Contextual query

- GIVEN a generation query "combine company name with years"
- WHEN similarity search runs with K=5
- THEN the 5 most relevant chunks (company info, year patterns) are returned

### Requirement: Embedding Cache

The system SHALL cache embeddings locally to avoid re-computation on repeated runs.

#### Scenario: Repeated run against same domain

- GIVEN a prior run cached embeddings for example.com
- WHEN a new run targets example.com
- THEN cached vectors are loaded without re-embedding
