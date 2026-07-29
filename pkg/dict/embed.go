// Package dict embeds curated password dictionaries into the binary so
// they are available at runtime without external file dependencies.
//
// Dictionaries are loaded via //go:embed and parsed as one-word-per-line
// text files. The LoadDictionaries function returns a map keyed by the
// base filename (e.g. "common" for data/common.txt).
package dict

import (
	"embed"
	"fmt"
	"path/filepath"
	"strings"
)

//go:embed data/*.txt
var dictFS embed.FS

// LoadDictionaries reads all embedded dictionary files and returns them as a
// map from dictionary name (base filename without extension) to word list.
// Each file is expected to contain one word per line. Blank lines and lines
// starting with '#' are treated as comments and ignored.
func LoadDictionaries() (map[string][]string, error) {
	entries, err := dictFS.ReadDir("data")
	if err != nil {
		return nil, fmt.Errorf("dict: read embedded dir: %w", err)
	}

	result := make(map[string][]string, len(entries))

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".txt") {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".txt")
		data, err := dictFS.ReadFile(filepath.Join("data", entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("dict: read %s: %w", entry.Name(), err)
		}

		words := parseLines(string(data))
		if len(words) > 0 {
			result[name] = words
		}
	}

	return result, nil
}

// parseLines splits a text into lines, trimming whitespace and filtering out
// blank lines and comment lines (those starting with '#').
func parseLines(text string) []string {
	lines := strings.Split(text, "\n")
	words := make([]string, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Normalize to lowercase for consistent dictionary matching.
		words = append(words, strings.ToLower(line))
	}

	return words
}
