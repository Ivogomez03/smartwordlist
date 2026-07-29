package plugin

import (
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/Ivogomez03/smartwordlist/internal/generation"
)

//go:embed default_rules.yaml
var defaultRulesYAML []byte

// LoadDefaultRules returns the embedded default mutation rules. It is used
// as a fallback when the user has not provided a custom --rules file and the
// bundled defaults/rules.yaml is not found on disk (common with go install).
func LoadDefaultRules() (*generation.MutationRules, error) {
	var rules generation.MutationRules
	if err := yaml.Unmarshal(defaultRulesYAML, &rules); err != nil {
		return nil, fmt.Errorf("plugin: embedded rules: %w", err)
	}
	if err := validateRules(&rules, "<embedded>"); err != nil {
		return nil, err
	}
	return &rules, nil
}
