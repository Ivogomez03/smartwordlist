// Package export writes scored password candidates to plain-text wordlists
// and structured JSON metadata files.  It implements the Exporter interface
// from the pipeline design.
package export

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/gentleman-programming/smartwordlist/pkg/types"
)

// Exporter writes scored candidates to text or JSON output.  Callers are
// responsible for truncating the candidate slice to respect --max before
// calling Export methods.
type Exporter struct{}

// NewExporter returns a ready-to-use Exporter.
func NewExporter() *Exporter {
	return &Exporter{}
}

// ExportText writes one password per line to w.  Each candidate's Word field
// is written followed by a newline.  Write errors are wrapped and returned.
func (e *Exporter) ExportText(candidates []types.ScoredCandidate, w io.Writer) error {
	for _, c := range candidates {
		if _, err := fmt.Fprintln(w, c.Word); err != nil {
			return fmt.Errorf("export text: %w", err)
		}
	}
	return nil
}

// jsonPayload is the JSON structure written by ExportJSON.  Field names match
// the scoring-export spec.
type jsonPayload struct {
	Total            int             `json:"total"`
	Generated        int             `json:"generated"`
	Deduplicated     int             `json:"deduplicated"`
	GenerationTimeMs int64           `json:"generation_time_ms"`
	SourcesUsed      []string        `json:"sources_used"`
	MutationCounts   map[string]int  `json:"mutation_counts"`
	Candidates       []jsonCandidate `json:"candidates"`
}

type jsonCandidate struct {
	Word   string  `json:"word"`
	Score  float64 `json:"score"`
	Source string  `json:"source"`
}

// ExportJSON writes a pretty-printed JSON document containing generation
// statistics and the full scored candidate list to w.
//
// The stats parameter provides pre-dedup totals.  The deduplicated count is
// computed as stats.TotalCandidates - len(candidates).  If the result is
// negative (e.g. when TotalCandidates was not set or the pipeline aggregated
// counts differently) it is clamped to zero.
func (e *Exporter) ExportJSON(
	candidates []types.ScoredCandidate,
	stats types.Stats,
	w io.Writer,
) error {
	dedup := stats.TotalCandidates - len(candidates)
	if dedup < 0 {
		dedup = 0
	}

	payload := jsonPayload{
		Total:            len(candidates),
		Generated:        stats.TotalCandidates,
		Deduplicated:     dedup,
		GenerationTimeMs: stats.GenerationTime.Milliseconds(),
		SourcesUsed:      stats.SourcesUsed,
		MutationCounts:   stats.MutationCounts,
		Candidates:       make([]jsonCandidate, 0, len(candidates)),
	}

	for _, c := range candidates {
		payload.Candidates = append(payload.Candidates, jsonCandidate{
			Word:   c.Word,
			Score:  c.Score,
			Source: c.Source,
		})
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(payload); err != nil {
		return fmt.Errorf("export JSON: %w", err)
	}
	return nil
}
