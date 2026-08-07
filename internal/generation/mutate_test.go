package generation

import (
	"testing"
)

func TestMutationEngine_Mutate_Leet(t *testing.T) {
	rules := &MutationRules{
		LeetMap: map[string][]string{
			"a": {"4", "@"},
			"e": {"3"},
			"i": {"1", "!"},
			"o": {"0"},
		},
		Suffixes:       []string{"123"},
		Prefixes:       []string{"admin_"},
		YearRange:      YearRange{Start: 2025, End: 2026},
		CaseVariations: []string{"lower", "upper", "title"},
	}
	engine := NewMutationEngine(rules)

	tests := []struct {
		name      string
		input     string
		wantAnyOf []string // at least one of these must be present
		wantNone  []string // none of these should be present
	}{
		{
			name:      "admin produces leet variants",
			input:     "admin",
			wantAnyOf: []string{"4dm1n", "adm1n", "@dm1n"},
		},
		{
			name:      "leet includes original word",
			input:     "admin",
			wantAnyOf: []string{"admin"},
		},
		{
			name:      "case variations include Title Case",
			input:     "acme",
			wantAnyOf: []string{"Acme", "ACME", "acme"},
		},
		{
			name:      "suffix appended to original",
			input:     "acme",
			wantAnyOf: []string{"acme123"},
		},
		{
			name:      "year appended to original",
			input:     "acme",
			wantAnyOf: []string{"acme2026", "acme2025"},
		},
		{
			name:      "prefix prepended to original",
			input:     "acme",
			wantAnyOf: []string{"admin_acme"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := engine.Mutate(tt.input)

			if len(tt.wantAnyOf) > 0 {
				found := false
				for _, want := range tt.wantAnyOf {
					for _, r := range results {
						if r == want {
							found = true
							break
						}
					}
					if found {
						break
					}
				}
				if !found {
					t.Errorf("Mutate(%q): none of %v found in results (got %d variants)",
						tt.input, tt.wantAnyOf, len(results))
				}
			}

			if len(tt.wantNone) > 0 {
				for _, avoid := range tt.wantNone {
					for _, r := range results {
						if r == avoid {
							t.Errorf("Mutate(%q): unwanted value %q found in results", tt.input, avoid)
						}
					}
				}
			}
		})
	}
}

func TestMutationEngine_Mutate_NoDuplicates(t *testing.T) {
	rules := &MutationRules{
		LeetMap: map[string][]string{
			"a": {"4", "@"},
			"e": {"3"},
			"i": {"1"},
			"o": {"0"},
			"s": {"5", "$"},
		},
		Suffixes:       []string{"!", "123", "@", "2026", "2025"},
		Prefixes:       []string{"admin_", "dev_"},
		YearRange:      YearRange{Start: 2024, End: 2026},
		CaseVariations: []string{"lower", "upper", "title"},
	}
	engine := NewMutationEngine(rules)

	results := engine.Mutate("password")

	seen := make(map[string]bool)
	for _, r := range results {
		if seen[r] {
			t.Errorf("Mutate(\"password\"): duplicate value %q found", r)
		}
		seen[r] = true
	}

	if len(results) == 0 {
		t.Error("Mutate(\"password\"): expected non-empty results")
	}
}

func TestMutationEngine_Mutate_EmptyInput(t *testing.T) {
	rules := &MutationRules{
		LeetMap:        map[string][]string{"a": {"4"}},
		Suffixes:       []string{"123"},
		Prefixes:       []string{"!"},
		YearRange:      YearRange{Start: 2025, End: 2026},
		CaseVariations: []string{"lower"},
	}
	engine := NewMutationEngine(rules)

	results := engine.Mutate("")
	// Empty input: original gets filtered, but suffixes/prefixes/years on
	// empty string produce variants (e.g. ""+"123" = "123", "!"+"" = "!").
	// This is valid behaviour — empty-string mutations still produce candidates.
	if len(results) == 0 {
		t.Error("Mutate(\"\"): expected some results from suffix/prefix/year variants")
	}
	// Verify the empty string itself doesn't appear.
	for _, r := range results {
		if r == "" {
			t.Error("Mutate(\"\"): empty string appeared in results")
		}
	}
}

func TestMutationEngine_Mutate_YearRange(t *testing.T) {
	rules := &MutationRules{
		LeetMap:        map[string][]string{},
		Suffixes:       []string{},
		Prefixes:       []string{},
		YearRange:      YearRange{Start: 2025, End: 2026},
		CaseVariations: []string{},
	}
	engine := NewMutationEngine(rules)

	results := engine.Mutate("corp")
	has2025 := false
	has2026 := false
	for _, r := range results {
		if r == "corp2025" {
			has2025 = true
		}
		if r == "corp2026" {
			has2026 = true
		}
	}
	if !has2025 || !has2026 {
		t.Errorf("Mutate(\"corp\"): expected corp2025 and corp2026, got %v", results)
	}
}

func TestMutationEngine_Mutate_CaseVariations(t *testing.T) {
	rules := &MutationRules{
		LeetMap:        map[string][]string{},
		Suffixes:       []string{},
		Prefixes:       []string{},
		YearRange:      YearRange{Start: 2025, End: 2025},
		CaseVariations: []string{"lower", "upper", "title"},
	}
	engine := NewMutationEngine(rules)

	results := engine.Mutate("hello")
	hasLower := false
	hasUpper := false
	hasTitle := false
	for _, r := range results {
		switch r {
		case "hello":
			hasLower = true
		case "HELLO":
			hasUpper = true
		case "Hello":
			hasTitle = true
		}
	}
	if !hasLower {
		t.Error("expected lowercase variant 'hello'")
	}
	if !hasUpper {
		t.Error("expected uppercase variant 'HELLO'")
	}
	if !hasTitle {
		t.Error("expected title variant 'Hello'")
	}
}
