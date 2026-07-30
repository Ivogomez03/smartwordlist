package ollama

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestGenerateRequest_ThinkSerialization is the regression guard for the
// *bool think field. A plain bool with omitempty would silently drop
// "think":false, which caused qwen3:4b to route all output to the
// "thinking" field and leave "response" empty (0 bytes).
func TestGenerateRequest_ThinkSerialization(t *testing.T) {
	jsonSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"passwords": map[string]any{"type": "array"},
		},
	}

	t.Run("nil think omits field", func(t *testing.T) {
		req := generateRequest{
			Model:  "qwen3:4b",
			Prompt: "test",
			Stream: false,
			Format: jsonSchema,
			Think:  nil,
		}
		b, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		s := string(b)
		if strings.Contains(s, `"think"`) {
			t.Errorf("nil Think should omit the field, but got: %s", s)
		}
		if !strings.Contains(s, `"format"`) {
			t.Errorf("format field missing: %s", s)
		}
	})

	t.Run("false think IS serialized", func(t *testing.T) {
		thinkFalse := false
		req := generateRequest{
			Model:  "qwen3:4b",
			Prompt: "test",
			Stream: false,
			Format: jsonSchema,
			Think:  &thinkFalse,
		}
		b, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		s := string(b)

		// THE regression guard: if "think":false is missing, qwen3:4b would
		// route output to the thinking field and we'd get 0 bytes again.
		if !strings.Contains(s, `"think":false`) {
			t.Errorf(`"think":false must be literally present in the JSON payload, but got: %s`, s)
		}
		// Format must also be present.
		if !strings.Contains(s, `"format"`) {
			t.Errorf("format field missing: %s", s)
		}
	})

	t.Run("format schema is present", func(t *testing.T) {
		req := generateRequest{
			Model:  "qwen3:4b",
			Prompt: "test",
			Stream: false,
			Format: jsonSchema,
		}
		b, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		s := string(b)
		if !strings.Contains(s, `"format"`) {
			t.Errorf("format field missing when Format is set: %s", s)
		}
	})
}

// TestGenerateChunk_ThinkingField verifies the Thinking field survives
// round-trip through JSON deserialization.
func TestGenerateChunk_ThinkingField(t *testing.T) {
	// Simulate a chunk where the model put content in "thinking" instead of
	// "response" (the exact qwen3 bug scenario before think:false).
	input := `{"response":"","thinking":"{\"passwords\":[\"Ron2024!\"]}","done":true}`
	var chunk generateChunk
	if err := json.Unmarshal([]byte(input), &chunk); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if chunk.Thinking == "" {
		t.Error("Thinking field was empty — JSON deserialization lost the thinking content")
	}
	if !strings.Contains(chunk.Thinking, "Ron2024!") {
		t.Errorf("Thinking content wrong: %s", chunk.Thinking)
	}
}
