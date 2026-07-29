// Package rag provides the RAG (Retrieval-Augmented Generation) pipeline stage
// for SmartWordlist. It chunks reconnaissance data, indexes chunks with embedding
// vectors in veclite, and supports similarity search for context-aware generation.
package rag

import (
	"strings"

	"github.com/Ivogomez03/smartwordlist/pkg/types"
)

// Chunker splits a ReconResult into semantic text chunks suitable for
// embedding and vector search. Each non-empty section in the result becomes
// one chunk labelled with its source and carrying metadata about its origin.
type Chunker struct{}

// Chunk converts a ReconResult into a slice of semantic Chunks. Seven
// sections are evaluated — company, title, technologies, keywords, subdomains,
// paths, and emails. Sections with no data are skipped. Keywords are limited
// to the top 15 entries.
//
// Each chunk's Text field carries a human-readable label (e.g. "Company:
// Acme Corp") so the embedding model captures semantic context from the
// label alongside the data.
func (c *Chunker) Chunk(result *types.ReconResult) []types.Chunk {
	if result == nil {
		return nil
	}

	var chunks []types.Chunk

	if result.Company != "" {
		chunks = append(chunks, types.Chunk{
			Text:   "Company: " + result.Company,
			Source: "company",
			Metadata: map[string]string{
				"section": "company",
			},
		})
	}

	if result.Title != "" {
		chunks = append(chunks, types.Chunk{
			Text:   "Title: " + result.Title,
			Source: "title",
			Metadata: map[string]string{
				"section": "title",
			},
		})
	}

	if len(result.Technologies) > 0 {
		chunks = append(chunks, types.Chunk{
			Text:   "Technologies: " + strings.Join(result.Technologies, ", "),
			Source: "technologies",
			Metadata: map[string]string{
				"section": "technologies",
			},
		})
	}

	if len(result.Keywords) > 0 {
		kw := result.Keywords
		if len(kw) > 15 {
			kw = kw[:15]
		}
		chunks = append(chunks, types.Chunk{
			Text:   "Keywords: " + strings.Join(kw, ", "),
			Source: "keywords",
			Metadata: map[string]string{
				"section": "keywords",
			},
		})
	}

	if len(result.Subdomains) > 0 {
		chunks = append(chunks, types.Chunk{
			Text:   "Subdomains: " + strings.Join(result.Subdomains, ", "),
			Source: "subdomains",
			Metadata: map[string]string{
				"section": "subdomains",
			},
		})
	}

	if len(result.Paths) > 0 {
		chunks = append(chunks, types.Chunk{
			Text:   "Paths: " + strings.Join(result.Paths, ", "),
			Source: "paths",
			Metadata: map[string]string{
				"section": "paths",
			},
		})
	}

	if len(result.Emails) > 0 {
		chunks = append(chunks, types.Chunk{
			Text:   "Emails: " + strings.Join(result.Emails, ", "),
			Source: "emails",
			Metadata: map[string]string{
				"section": "emails",
			},
		})
	}

	return chunks
}
