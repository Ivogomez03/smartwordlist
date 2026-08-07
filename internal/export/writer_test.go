package export

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Ivogomez03/smartwordlist/pkg/types"
)

func TestExportText_OnePerLine(t *testing.T) {
	candidates := []types.ScoredCandidate{
		{Word: "Password1!", Score: 8.5, Source: "llm"},
		{Word: "admin2024", Score: 4.0, Source: "combo"},
	}

	var buf bytes.Buffer
	if err := NewExporter().ExportText(candidates, &buf); err != nil {
		t.Fatalf("ExportText: %v", err)
	}

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), buf.String())
	}
	if lines[0] != "Password1!" || lines[1] != "admin2024" {
		t.Errorf("unexpected lines: %v", lines)
	}
}

func TestExportText_EmptyInput(t *testing.T) {
	var buf bytes.Buffer
	if err := NewExporter().ExportText(nil, &buf); err != nil {
		t.Fatalf("ExportText: %v", err)
	}
	if buf.Len() != 0 {
		t.Errorf("expected empty output for no candidates, got %q", buf.String())
	}
}

// TestExportJSON_DeduplicatedReflectsDedupNotTruncation is the regression
// guard for a bug where the JSON "deduplicated" field was computed as
// stats.TotalCandidates - len(exportedCandidates). Since the caller truncates
// to --max BEFORE calling ExportJSON, that formula silently folded candidates
// dropped by --max into the reported dedup count. ExportJSON must instead use
// stats.DeduplicatedCount, which the caller is expected to capture before
// truncation.
func TestExportJSON_DeduplicatedReflectsDedupNotTruncation(t *testing.T) {
	// Simulate: 100 raw candidates, 40 survived dedup, but only 10 are
	// exported because --max=10 truncated the deduped list.
	exported := make([]types.ScoredCandidate, 10)
	for i := range exported {
		exported[i] = types.ScoredCandidate{Word: "word", Score: 1.0, Source: "llm"}
	}

	stats := types.Stats{
		TotalCandidates:   100,
		DeduplicatedCount: 40,
		GenerationTime:    time.Second,
		SourcesUsed:       []string{"llm"},
		MutationCounts:    map[string]int{},
	}

	var buf bytes.Buffer
	if err := NewExporter().ExportJSON(exported, stats, &buf); err != nil {
		t.Fatalf("ExportJSON: %v", err)
	}

	var payload jsonPayload
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	if payload.Total != 10 {
		t.Errorf("Total = %d, want 10 (exported count)", payload.Total)
	}
	if payload.Generated != 100 {
		t.Errorf("Generated = %d, want 100", payload.Generated)
	}
	// The buggy formula would have computed 100 - 10 = 90 here.
	if payload.Deduplicated != 40 {
		t.Errorf("Deduplicated = %d, want 40 (the pre-truncation dedup count, not TotalCandidates - len(exported))", payload.Deduplicated)
	}
}

func TestExportJSON_CandidateFields(t *testing.T) {
	candidates := []types.ScoredCandidate{
		{Word: "Sup3rSecret!", Score: 9.25, Source: "llm"},
	}
	stats := types.Stats{TotalCandidates: 1, DeduplicatedCount: 1}

	var buf bytes.Buffer
	if err := NewExporter().ExportJSON(candidates, stats, &buf); err != nil {
		t.Fatalf("ExportJSON: %v", err)
	}

	var payload jsonPayload
	if err := json.Unmarshal(buf.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if len(payload.Candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(payload.Candidates))
	}
	got := payload.Candidates[0]
	if got.Word != "Sup3rSecret!" || got.Score != 9.25 || got.Source != "llm" {
		t.Errorf("candidate mismatch: %+v", got)
	}
}
