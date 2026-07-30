package generation

import (
	"strings"
	"testing"

	"github.com/Ivogomez03/smartwordlist/pkg/types"
)

// ---------------------------------------------------------------------------
// extractCandidates — pathological Python-program response (real bug)
// ---------------------------------------------------------------------------

func TestExtractCandidates_PathologicalPythonResponse(t *testing.T) {
	// Simulates the real output from qwen3:4b that produced a Python program
	// instead of a wordlist.
	response := `To solve this problem, we need to generate 500 unique passwords.
Here is a Python program that does it:

` + "```python" + `
import random
def main():
    numbers = []
    for i in range(500):
        n = random.randint(1000, 9999)
        numbers.append(n)
    print(numbers)

if __name__ == "__main__":
    main()
    continue
    break
` + "```" + `

python
continue
break
numbers.append(n)
main()
`

	// The first JSON-path attempt will fail (no JSON in this response).
	// extractCandidates falls back to line scan.
	candidates := extractCandidates(response, 0)

	// Build a set of candidate words for fast check.
	words := make(map[string]bool)
	for _, c := range candidates {
		words[c.Word] = true
	}

	// Garbage tokens that MUST be rejected.
	garbage := []string{
		"python", "main", "continue", "break",
		"numbers.append(n)", "main()",
	}

	for _, g := range garbage {
		if words[g] {
			t.Errorf("GARBAGE SURVIVED: %q should have been rejected but was in candidates", g)
		}
	}

	// Verify each garbage token is rejected by a specific rule.
	t.Run("python-blocklist", func(t *testing.T) {
		if words["python"] {
			t.Error(`"python" not rejected by programmingNoise blocklist`)
		}
	})
	t.Run("main-blocklist", func(t *testing.T) {
		if words["main"] {
			t.Error(`"main" not rejected by programmingNoise blocklist`)
		}
	})
	t.Run("continue-blocklist", func(t *testing.T) {
		if words["continue"] {
			t.Error(`"continue" not rejected by programmingNoise blocklist`)
		}
	})
	t.Run("break-blocklist", func(t *testing.T) {
		if words["break"] {
			t.Error(`"break" not rejected by programmingNoise blocklist`)
		}
	})
	t.Run("numbers.append(n)-parens", func(t *testing.T) {
		if words["numbers.append(n)"] {
			t.Error(`"numbers.append(n)" not rejected — parentheses should trigger codeSyntaxChars rejection`)
		}
	})
	t.Run("main()-parens", func(t *testing.T) {
		if words["main()"] {
			t.Error(`"main()" not rejected — parentheses should trigger codeSyntaxChars rejection`)
		}
	})
}

// ---------------------------------------------------------------------------
// extractCandidates — JSON primary path
// ---------------------------------------------------------------------------

func TestExtractCandidates_CleanJSON(t *testing.T) {
	response := `{"passwords": ["Ron2024!", "Barcelo#23", "ron_barcelo12"]}`

	candidates := extractCandidates(response, 0)

	if len(candidates) != 3 {
		t.Fatalf("expected 3 candidates, got %d: %v", len(candidates), candidates)
	}
	if candidates[0].Word != "Ron2024!" {
		t.Errorf("candidates[0] = %q, want %q", candidates[0].Word, "Ron2024!")
	}
	if candidates[1].Word != "Barcelo#23" {
		t.Errorf("candidates[1] = %q, want %q", candidates[1].Word, "Barcelo#23")
	}
	if candidates[2].Word != "ron_barcelo12" {
		t.Errorf("candidates[2] = %q, want %q", candidates[2].Word, "ron_barcelo12")
	}
	for _, c := range candidates {
		if c.Source != "llm" {
			t.Errorf("candidate %q has source %q, want %q", c.Word, c.Source, "llm")
		}
	}
}

func TestExtractCandidates_JSONWrappedInProse(t *testing.T) {
	response := `Here you go:
{"passwords": ["RonBarcelo!","brand2024"]}
Done.`

	candidates := extractCandidates(response, 0)

	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d: %v", len(candidates), candidates)
	}
	if candidates[0].Word != "RonBarcelo!" {
		t.Errorf("candidates[0] = %q, want %q", candidates[0].Word, "RonBarcelo!")
	}
	if candidates[1].Word != "brand2024" {
		t.Errorf("candidates[1] = %q, want %q", candidates[1].Word, "brand2024")
	}
}

// ---------------------------------------------------------------------------
// extractCandidates — fallback line scan (no JSON)
// ---------------------------------------------------------------------------

func TestExtractCandidates_FallbackRejectsCodeTokens(t *testing.T) {
	// No JSON in response — falls back to line scan.
	response := `Ron2024!, Barcelo#23, brand2024, python, main()`

	candidates := extractCandidates(response, 0)

	// Collect words.
	words := make(map[string]bool)
	for _, c := range candidates {
		words[c.Word] = true
	}

	// Good passwords must survive.
	if !words["Ron2024!"] {
		t.Error(`"Ron2024!" missing from candidates`)
	}
	if !words["Barcelo#23"] {
		t.Error(`"Barcelo#23" missing from candidates`)
	}
	if !words["brand2024"] {
		t.Error(`"brand2024" missing from candidates`)
	}

	// Code tokens must be rejected.
	if words["python"] {
		t.Error(`"python" not rejected by programmingNoise blocklist in fallback`)
	}
	if words["main()"] {
		t.Error(`"main()" not rejected by codeSyntaxChars in fallback`)
	}
}

// ---------------------------------------------------------------------------
// extractCandidates — deduplication
// ---------------------------------------------------------------------------

func TestExtractCandidates_DedupPreservesFirstCasing(t *testing.T) {
	response := `{"passwords": ["Ron2024!", "ron2024!", "RON2024!", "Barcelo#23"]}`

	candidates := extractCandidates(response, 0)

	if len(candidates) != 2 {
		t.Fatalf("expected 2 unique candidates (dedup on lowercase), got %d: %v", len(candidates), candidates)
	}
	// First-seen casing preserved.
	if candidates[0].Word != "Ron2024!" {
		t.Errorf("candidates[0] = %q, want %q (first-seen casing)", candidates[0].Word, "Ron2024!")
	}
	if candidates[1].Word != "Barcelo#23" {
		t.Errorf("candidates[1] = %q, want %q", candidates[1].Word, "Barcelo#23")
	}
}

// ---------------------------------------------------------------------------
// buildPrompt assertions
// ---------------------------------------------------------------------------

func TestBuildPrompt_NoCodeInstructions(t *testing.T) {
	chunks := []types.ScoredChunk{
		{Chunk: types.Chunk{Text: "company: Ron Barcelo", Source: "company"}, Score: 1.0},
	}

	prompt := buildPrompt(chunks)

	// Must mention JSON / passwords array.
	if !strings.Contains(prompt, "passwords") {
		t.Error("prompt does not mention 'passwords' key")
	}
	if !strings.Contains(prompt, "JSON") {
		t.Error("prompt does not mention JSON output")
	}

	// Must forbid code/explanation.
	if !strings.Contains(prompt, "No code") {
		t.Error("prompt does not forbid code")
	}
	if !strings.Contains(prompt, "No explanation") {
		t.Error("prompt does not forbid explanations")
	}

	// Must NOT contain algorithmic-sounding constraint.
	if strings.Contains(prompt, "not sequential") {
		t.Error("prompt still contains 'not sequential' — reads like a programming spec")
	}

	// Must NOT contain the literal word "python" (triggers code-mode in small models).
	if strings.Contains(prompt, "python") || strings.Contains(prompt, "Python") {
		t.Error("prompt contains 'python' literal — risks code-generation mode")
	}
}

func TestBuildPrompt_EmptyCompany(t *testing.T) {
	chunks := []types.ScoredChunk{
		{Chunk: types.Chunk{Text: "company: ", Source: "company"}, Score: 1.0},
	}

	prompt := buildPrompt(chunks)

	if !strings.Contains(prompt, "JSON") {
		t.Error("prompt for empty company does not request JSON")
	}
}

// ---------------------------------------------------------------------------
// isValidCandidate — guard against false positives
// ---------------------------------------------------------------------------

func TestIsValidCandidate_RejectsCodeSyntax(t *testing.T) {
	tests := []struct {
		input string
		valid bool
	}{
		{"numbers.append(n)", false},
		{"main()", false},
		{"foo(bar)", false},
		{"x=[1,2,3]", false},
		{"{key:val}", false},
		{`"quoted"`, false},
		{"code;", false},
		{"x=1", false},
		{"a:b", false},
		{"python", false},
		{"main", false},
		{"continue", false},
		{"break", false},
		// Real passwords that MUST survive.
		{"Ron2024!", true},
		{"Barcelo#23", true},
		{"ron_barcelo12", true},
		{"admin@2024", true},
		{"P@ssw0rd!", true},
		{"test.123", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := isValidCandidate(tt.input); got != tt.valid {
				t.Errorf("isValidCandidate(%q) = %v, want %v", tt.input, got, tt.valid)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// extractCandidates — recovers passwords embedded in a fenced "Output:" demo
// (the real qwen3:4b scenario where the model ignores the JSON schema and
// writes a Python program with the results shown in a fenced block)
// ---------------------------------------------------------------------------

func TestExtractCandidates_RecoversEmbeddedOutputPasswords(t *testing.T) {
	// Simulates the real ronbarcelobc.com run: qwen3:4b ignored the JSON
	// schema, wrote a Python program, then embedded the generated passwords
	// inside a fenced "Output:" demonstration as a numbered list.
	response := "To generate 500 unique passwords...:\n\n" +
		"```python\n" +
		"import random\n" +
		"valid_numbers = []\n" +
		"for num in range(10, 10000):\n" +
		"    valid_numbers.append(s)\n" +
		"company_parts = ['ron', 'barcelo']\n" +
		"```\n\n" +
		"**Output:**\n" +
		"```\n" +
		"First 5 passwords:\n" +
		"1. .brandcenter23\n" +
		"2. #barcelo25\n" +
		"3. !roncenter46\n" +
		"4. @brand28\n" +
		"5. .barcelo37\n" +
		"```\n\n" +
		"**Revised Output:**\n" +
		"```\n" +
		"1. .brandcenter24\n" +
		"```\n\n" +
		"else:\n" +
		"    continue\n"

	candidates := extractCandidates(response, 0)
	words := make(map[string]bool)
	for _, c := range candidates {
		words[c.Word] = true
	}

	// Embedded demo passwords MUST be recovered — fence tracking must not
	// skip them. Includes a "#"-prefixed password to guard the leading-#
	// relaxation.
	must := []string{".brandcenter23", "#barcelo25", "!roncenter46", "@brand28", ".barcelo37", ".brandcenter24"}
	for _, w := range must {
		if !words[w] {
			t.Errorf("embedded password %q NOT recovered — must not be skipped as fenced content", w)
		}
	}

	// Code garbage MUST still be rejected (by isValidCandidate, not by fences).
	garbage := []string{"python", "else:", "continue", "valid_numbers.append(s)", "import", "random", "range"}
	for _, g := range garbage {
		if words[g] {
			t.Errorf("code garbage %q survived — isValidCandidate should reject it", g)
		}
	}
}

func TestIsValidCandidate_AllowsPasswordSymbols(t *testing.T) {
	// These are real password patterns that MUST pass.
	allowed := []string{
		"Ron2024!",
		"Barcelo#23",
		"ron_barcelo12",
		"admin@2024",
		"P@ssw0rd!",
		"test.123",
		"hello-world",
		"foo+bar",
		"pass&word",
		"abc*xyz",
	}

	for _, w := range allowed {
		if !isValidCandidate(w) {
			t.Errorf("isValidCandidate(%q) = false — real password candidate rejected", w)
		}
	}
}
