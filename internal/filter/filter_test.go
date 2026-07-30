package filter

import (
	"testing"
)

func TestIsJunkWord_True(t *testing.T) {
	junk := []string{
		"the", "and", "page", "www", "com",
		"javascript", "enable", "cookie",
		"nginx", "wordpress", "react", "docker",
		"privacy", "policy", "contact", "login",
	}
	for _, w := range junk {
		if !IsJunkWord(w) {
			t.Errorf("IsJunkWord(%q) = false, want true", w)
		}
	}
}

func TestIsJunkWord_False(t *testing.T) {
	valid := []string{
		"acme", "cerrobayo", "patagonia", "widget",
		"cloud", "security", "enterprise", "barcelona",
	}
	for _, w := range valid {
		if IsJunkWord(w) {
			t.Errorf("IsJunkWord(%q) = true, want false", w)
		}
	}
}

func TestIsJunkWord_CaseInsensitive(t *testing.T) {
	if !IsJunkWord("THE") {
		t.Error("IsJunkWord('THE') should be true (case insensitive)")
	}
	if !IsJunkWord("Nginx") {
		t.Error("IsJunkWord('Nginx') should be true (case insensitive)")
	}
}

func TestIsJunkWord_WhitespaceTrimmed(t *testing.T) {
	if !IsJunkWord("  the  ") {
		t.Error("IsJunkWord should trim whitespace")
	}
}

func TestIsJunkCandidate_Whitespace(t *testing.T) {
	if !IsJunkCandidate("pass word") {
		t.Error("IsJunkCandidate('pass word') should be true")
	}
	if !IsJunkCandidate("pass\tword") {
		t.Error("IsJunkCandidate with tab should be true")
	}
}

func TestIsJunkCandidate_TooShort(t *testing.T) {
	if !IsJunkCandidate("ab") {
		t.Error("IsJunkCandidate('ab') should be true (< 4 chars)")
	}
	if IsJunkCandidate("abcd") {
		t.Error("IsJunkCandidate('abcd') should be false (>= 4 chars with letters)")
	}
}

func TestIsJunkCandidate_AllDigits(t *testing.T) {
	if !IsJunkCandidate("12345") {
		t.Error("IsJunkCandidate('12345') should be true (all digits)")
	}
}

func TestIsJunkCandidate_NoLetters(t *testing.T) {
	if !IsJunkCandidate("!@#$%") {
		t.Error("IsJunkCandidate('!@#$%') should be true (no letters)")
	}
}

func TestIsJunkCandidate_Valid(t *testing.T) {
	if IsJunkCandidate("Acme2026!") {
		t.Error("IsJunkCandidate('Acme2026!') should be false")
	}
	if IsJunkCandidate("pass") {
		t.Error("IsJunkCandidate('pass') should be false")
	}
}

func TestIsAllDigits(t *testing.T) {
	if !IsAllDigits("12345") {
		t.Error("IsAllDigits('12345') = false")
	}
	if IsAllDigits("123a5") {
		t.Error("IsAllDigits('123a5') = true")
	}
	if IsAllDigits("") {
		t.Error("IsAllDigits('') = true")
	}
}

func TestDeduplicate(t *testing.T) {
	input := []string{"a", "b", "a", "c", "b", ""}
	result := Deduplicate(input)
	expected := []string{"a", "b", "c"}
	if len(result) != len(expected) {
		t.Fatalf("Deduplicate len = %d, want %d", len(result), len(expected))
	}
	for i, v := range expected {
		if result[i] != v {
			t.Errorf("Deduplicate[%d] = %q, want %q", i, result[i], v)
		}
	}
}

func TestDeduplicate_Nil(t *testing.T) {
	result := Deduplicate(nil)
	if len(result) != 0 {
		t.Errorf("Deduplicate(nil) len = %d, want 0", len(result))
	}
}

func TestDeduplicate_Empty(t *testing.T) {
	result := Deduplicate([]string{})
	if len(result) != 0 {
		t.Errorf("Deduplicate(empty) len = %d, want 0", len(result))
	}
}
