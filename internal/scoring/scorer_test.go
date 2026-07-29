package scoring

import (
	"testing"

	"github.com/gentleman-programming/smartwordlist/pkg/types"
)

func TestScorer_Score_SourceWeighting(t *testing.T) {
	scorer := NewScorer()

	candidates := []types.Candidate{
		{Word: "LLM", Source: "llm"},
		{Word: "Rule", Source: "rule-mutation"},
		{Word: "Dict", Source: "dict"},
		{Word: "Combo", Source: "combo"},
	}

	scored := scorer.Score(candidates)

	// LLM source should have highest base score
	if len(scored) != 4 {
		t.Fatalf("expected 4 scored candidates, got %d", len(scored))
	}

	// First should be LLM
	if scored[0].Source != "llm" {
		t.Errorf("expected llm first (highest score), got %s with %.2f", scored[0].Source, scored[0].Score)
	}
	// Last should be combo (lowest)
	if scored[3].Source != "combo" {
		t.Errorf("expected combo last (lowest score), got %s with %.2f", scored[3].Source, scored[3].Score)
	}
}

func TestScorer_Deduplicate_CaseInsensitive(t *testing.T) {
	scorer := NewScorer()

	candidates := []types.ScoredCandidate{
		{Word: "Password", Score: 8.0, Source: "llm"},
		{Word: "password", Score: 5.0, Source: "dict"},
		{Word: "PASSWORD", Score: 3.0, Source: "combo"},
		{Word: "unique", Score: 7.0, Source: "llm"},
	}

	deduped := scorer.Deduplicate(candidates)

	// "Password" and "password" and "PASSWORD" should collapse to one
	if len(deduped) != 2 {
		t.Fatalf("expected 2 candidates after dedup, got %d", len(deduped))
	}

	// The surviving "password" variant should be the highest-scored one
	for _, c := range deduped {
		if c.Word == "password" {
			t.Error("lowercase variant should not survive against higher-scored 'Password'")
		}
	}

	hasPassword := false
	hasUnique := false
	for _, c := range deduped {
		if c.Word == "Password" {
			hasPassword = true
			if c.Score != 8.0 {
				t.Errorf("Password should have score 8.0, got %.2f", c.Score)
			}
		}
		if c.Word == "unique" {
			hasUnique = true
		}
	}
	if !hasPassword {
		t.Error("Password (highest-scored) should survive dedup")
	}
	if !hasUnique {
		t.Error("unique should survive dedup")
	}
}

func TestScorer_Score_SortDescending(t *testing.T) {
	scorer := NewScorer()

	candidates := []types.Candidate{
		{Word: "low", Source: "combo"},
		{Word: "high", Source: "llm"},
		{Word: "mid", Source: "rule-mutation"},
	}

	scored := scorer.Score(candidates)

	for i := 1; i < len(scored); i++ {
		if scored[i-1].Score < scored[i].Score {
			t.Errorf("sort order violated at position %d: %.2f < %.2f",
				i, scored[i-1].Score, scored[i].Score)
		}
	}
}

func TestScorer_Score_EmptyInput(t *testing.T) {
	scorer := NewScorer()
	scored := scorer.Score(nil)
	if len(scored) != 0 {
		t.Errorf("expected 0 results for nil input, got %d", len(scored))
	}

	scored = scorer.Score([]types.Candidate{})
	if len(scored) != 0 {
		t.Errorf("expected 0 results for empty input, got %d", len(scored))
	}
}

func TestScorer_Deduplicate_EmptyInput(t *testing.T) {
	scorer := NewScorer()
	result := scorer.Deduplicate(nil)
	if len(result) != 0 {
		t.Errorf("expected 0 results for nil dedup, got %d", len(result))
	}

	result = scorer.Deduplicate([]types.ScoredCandidate{})
	if len(result) != 0 {
		t.Errorf("expected 0 results for empty dedup, got %d", len(result))
	}
}

func TestScorer_Score_AllSameScore(t *testing.T) {
	scorer := NewScorer()

	candidates := []types.Candidate{
		{Word: "short", Source: "dict"},
		{Word: "shorter", Source: "dict"},
		{Word: "short123", Source: "dict"},
	}

	scored := scorer.Score(candidates)

	// All have same source weight, so scores should differ only by length bonus
	// and pattern complexity. "short123" has digits → complexity bonus.
	// Order: higher score → longer word tiebreaker for same score.
	if len(scored) != 3 {
		t.Fatalf("expected 3 results, got %d", len(scored))
	}

	// "short123" should have highest score (digit bonus + longest)
	if scored[0].Word != "short123" {
		t.Errorf("expected short123 first (highest complexity), got %s", scored[0].Word)
	}
}

func TestScorer_Score_ClampToTen(t *testing.T) {
	scorer := NewScorer()

	candidates := []types.Candidate{
		{Word: "SuperLongPasswordWithDigits123!@#AndMixedCase", Source: "llm"},
	}

	scored := scorer.Score(candidates)
	if len(scored) != 1 {
		t.Fatal("expected 1 result")
	}
	if scored[0].Score > 10.0 {
		t.Errorf("score %.2f should not exceed 10.0", scored[0].Score)
	}
}
