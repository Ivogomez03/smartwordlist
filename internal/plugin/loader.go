// Package plugin provides YAML/TOML rule file loading with validation and
// native Go plugin isolation via recover().
package plugin

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"

	"github.com/Ivogomez03/smartwordlist/internal/generation"
)

// knownRuleKeys defines the valid top-level keys in a rules file. Any key
// not in this set triggers a warning (but is not fatal).
var knownRuleKeys = map[string]bool{
	"leet_map":        true,
	"suffixes":        true,
	"prefixes":        true,
	"year_range":      true,
	"case_variations": true,
}

// LoadRulesFile loads mutation rules from a YAML (.yaml, .yml) or TOML
// (.toml) file.  The file extension determines the parser; unknown
// extensions produce an error.
//
// Validation checks:
//   - File existence (clear error message)
//   - Parse errors (malformed YAML/TOML → wrapped error)
//   - Required fields present and non-empty
//   - Unknown top-level keys → warning to stderr, not fatal
func LoadRulesFile(path string) (*generation.MutationRules, error) {
	// File existence check with a descriptive message.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("plugin: rules file not found: %s", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("plugin: read rules file %s: %w", path, err)
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".yaml", ".yml":
		return loadYAML(data, path)
	case ".toml":
		return loadTOML(data, path)
	default:
		return nil, fmt.Errorf(
			"plugin: unsupported rules file extension %q for %s (must be .yaml, .yml, or .toml)",
			ext, path,
		)
	}
}

// loadYAML parses data as YAML, warns on unknown keys, and validates required
// fields.
func loadYAML(data []byte, path string) (*generation.MutationRules, error) {
	// First pass: detect unknown keys.
	var raw map[string]interface{}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("plugin: parse YAML in %s: %w", path, err)
	}
	for key := range raw {
		if !knownRuleKeys[key] {
			fmt.Fprintf(os.Stderr,
				"plugin: warning: unknown key %q in %s — ignored\n", key, path)
		}
	}

	var rules generation.MutationRules
	if err := yaml.Unmarshal(data, &rules); err != nil {
		return nil, fmt.Errorf("plugin: parse YAML in %s: %w", path, err)
	}
	if err := validateRules(&rules, path); err != nil {
		return nil, err
	}
	return &rules, nil
}

// loadTOML parses data as TOML, warns on unknown keys via undecoded metadata,
// and validates required fields.
func loadTOML(data []byte, path string) (*generation.MutationRules, error) {
	var rules generation.MutationRules
	md, err := toml.Decode(string(data), &rules)
	if err != nil {
		return nil, fmt.Errorf("plugin: parse TOML in %s: %w", path, err)
	}

	// Warn on any top-level keys that were not decoded into the struct.
	for _, k := range md.Undecoded() {
		parts := strings.SplitN(k.String(), ".", 2)
		if len(parts) > 0 && !knownRuleKeys[parts[0]] {
			fmt.Fprintf(os.Stderr,
				"plugin: warning: unknown key %q in %s — ignored\n", parts[0], path)
		}
	}

	if err := validateRules(&rules, path); err != nil {
		return nil, err
	}
	return &rules, nil
}

// validateRules checks that each required top-level key is present and
// non-empty in the loaded rules.
func validateRules(rules *generation.MutationRules, path string) error {
	if len(rules.LeetMap) == 0 {
		return fmt.Errorf(
			"plugin: missing or empty required field %q in %s", "leet_map", path)
	}
	if len(rules.Suffixes) == 0 {
		return fmt.Errorf(
			"plugin: missing or empty required field %q in %s", "suffixes", path)
	}
	if len(rules.Prefixes) == 0 {
		return fmt.Errorf(
			"plugin: missing or empty required field %q in %s", "prefixes", path)
	}
	if rules.YearRange.Start == 0 && rules.YearRange.End == 0 {
		return fmt.Errorf(
			"plugin: missing or empty required field %q in %s", "year_range", path)
	}
	if rules.YearRange.Start > rules.YearRange.End {
		return fmt.Errorf(
			"plugin: invalid %q in %s: start (%d) is after end (%d)",
			"year_range", path, rules.YearRange.Start, rules.YearRange.End)
	}
	return nil
}
