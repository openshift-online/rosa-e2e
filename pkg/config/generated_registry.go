package config

import (
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

// The generated CI status registry, embedded so Go consumers (rosa-eng-dashboard)
// read it through the module instead of hand-copying the yaml. `make configs`
// keeps it in sync with configs/generated/ci-status-jobs.yaml, and the lint
// test fails when the two drift.
//
//go:embed generated/ci-status-jobs.yaml
var generatedCIStatusJobsYAML []byte

// GeneratedCIStatusJobsYAML returns the generated CI status registry as raw
// yaml, for consumers that parse it into their own types. Returns a copy so
// a consumer mutating the bytes cannot corrupt the registry for others.
func GeneratedCIStatusJobsYAML() []byte {
	return append([]byte(nil), generatedCIStatusJobsYAML...)
}

// ParseGeneratedCIStatusConfig parses the embedded generated registry.
func ParseGeneratedCIStatusConfig() (*CIStatusConfig, error) {
	var cfg CIStatusConfig
	if err := yaml.Unmarshal(generatedCIStatusJobsYAML, &cfg); err != nil {
		return nil, fmt.Errorf("parsing embedded generated registry: %w", err)
	}
	return &cfg, nil
}
