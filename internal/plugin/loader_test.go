package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRulesFile_ValidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "valid.yaml")

	content := `
leet_map:
  a:
    - "4"
    - "@"
  e:
    - "3"
suffixes:
  - "123"
  - "!"
prefixes:
  - "admin_"
year_range:
  start: 2020
  end: 2026
case_variations:
  - lower
  - upper
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	rules, err := LoadRulesFile(path)
	if err != nil {
		t.Fatalf("LoadRulesFile: unexpected error: %v", err)
	}
	if rules == nil {
		t.Fatal("expected non-nil rules")
	}
	if len(rules.LeetMap) == 0 {
		t.Error("expected non-empty leet_map")
	}
	if len(rules.Suffixes) != 2 {
		t.Errorf("expected 2 suffixes, got %d", len(rules.Suffixes))
	}
	if len(rules.Prefixes) != 1 {
		t.Errorf("expected 1 prefix, got %d", len(rules.Prefixes))
	}
	if rules.YearRange.Start != 2020 || rules.YearRange.End != 2026 {
		t.Errorf("expected year_range 2020-2026, got %d-%d",
			rules.YearRange.Start, rules.YearRange.End)
	}
}

func TestLoadRulesFile_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.yaml")

	content := `this is not: valid: yaml: [malformed`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadRulesFile(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
	if !strings.Contains(err.Error(), "parse YAML") {
		t.Errorf("expected 'parse YAML' in error, got: %v", err)
	}
}

func TestLoadRulesFile_MissingRequiredFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing_fields.yaml")

	content := `
leet_map: {}
suffixes:
  - "123"
prefixes:
  - "!"
year_range:
  start: 2020
  end: 2026
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadRulesFile(path)
	if err == nil {
		t.Fatal("expected error for empty leet_map, got nil")
	}
	if !strings.Contains(err.Error(), "leet_map") {
		t.Errorf("expected 'leet_map' mention in error, got: %v", err)
	}
}

// TestLoadRulesFile_InvertedYearRange guards against a rules file with
// start > end silently producing zero year-based candidates with no
// indication to the user that the range is misconfigured.
func TestLoadRulesFile_InvertedYearRange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inverted_year_range.yaml")

	content := `
leet_map:
  a:
    - "4"
suffixes:
  - "123"
prefixes:
  - "!"
year_range:
  start: 2030
  end: 2015
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadRulesFile(path)
	if err == nil {
		t.Fatal("expected error for inverted year_range (start > end), got nil")
	}
	if !strings.Contains(err.Error(), "year_range") {
		t.Errorf("expected 'year_range' mention in error, got: %v", err)
	}
}

func TestLoadRulesFile_UnknownKeysWarning(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unknown_keys.yaml")

	// Unknown keys should produce a warning but not an error (as long as
	// required fields are present).
	content := `
leet_map:
  a:
    - "4"
suffixes:
  - "123"
prefixes:
  - "!"
year_range:
  start: 2020
  end: 2026
unknown_field: "should warn"
another_unknown: 42
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	rules, err := LoadRulesFile(path)
	if err != nil {
		t.Fatalf("LoadRulesFile should not fail on unknown keys: %v", err)
	}
	if rules == nil {
		t.Fatal("expected non-nil rules despite unknown keys")
	}
}

func TestLoadRulesFile_UnsupportedExtension(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.json")

	content := `{"leet_map": {}}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadRulesFile(path)
	if err == nil {
		t.Fatal("expected error for unsupported extension .json")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("expected 'unsupported' in error, got: %v", err)
	}
}

func TestLoadRulesFile_FileNotFound(t *testing.T) {
	_, err := LoadRulesFile("/nonexistent/path/rules.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestLoadRulesFile_TOML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "valid.toml")

	content := `
suffixes = ["123", "!"]
prefixes = ["admin_"]
case_variations = ["lower", "upper"]

[leet_map]
a = ["4", "@"]
e = ["3"]

[year_range]
start = 2020
end = 2026
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	rules, err := LoadRulesFile(path)
	if err != nil {
		t.Fatalf("LoadRulesFile(TOML): unexpected error: %v", err)
	}
	if rules == nil {
		t.Fatal("expected non-nil rules for TOML")
	}
	if len(rules.LeetMap) == 0 {
		t.Error("expected non-empty leet_map from TOML")
	}
}

func TestSafeCall_PanicIsolation(t *testing.T) {
	// safeCall must recover from panics and return an error instead of crashing.
	t.Run("panicking function returns error", func(t *testing.T) {
		err := safeCall(func() error {
			panic("boom")
		})
		if err == nil {
			t.Fatal("expected error from panicking function, got nil")
		}
		if !strings.Contains(err.Error(), "plugin panic") {
			t.Errorf("expected 'plugin panic' in error, got: %v", err)
		}
		if !strings.Contains(err.Error(), "boom") {
			t.Errorf("expected panic value 'boom' in error, got: %v", err)
		}
	})

	t.Run("non-panicking function returns normally", func(t *testing.T) {
		err := safeCall(func() error {
			return nil
		})
		if err != nil {
			t.Fatalf("unexpected error from normal function: %v", err)
		}
	})

	t.Run("error-returning function returns error", func(t *testing.T) {
		const want = "something went wrong"
		err := safeCall(func() error {
			return fmt.Errorf("%s", want)
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "something went wrong") {
			t.Errorf("expected error message, got: %v", err)
		}
	})
}

func TestLoadRulesFile_TOML_InvalidSyntax(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.toml")

	content := `this is [ not valid toml = `
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadRulesFile(path)
	if err == nil {
		t.Fatal("expected error for invalid TOML")
	}
	if !strings.Contains(err.Error(), "parse TOML") {
		t.Errorf("expected 'parse TOML' in error, got: %v", err)
	}
}
