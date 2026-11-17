package config

import (
	_ "embed"
	"fmt"

	"gopkg.in/yaml.v3"
)

// The component deployments registry, embedded so Go consumers
// (rosa-eng-dashboard) read it through the module instead of hand-copying the
// yaml. `make configs` keeps it in sync with
// configs/component-deployments.yaml, and the lint test fails when the two
// drift.
//
//go:embed generated/component-deployments.yaml
var componentDeploymentsYAML []byte

// GeneratedComponentDeploymentsYAML returns the component deployments
// registry as raw yaml, for consumers that parse it into their own types.
// Returns a copy so a consumer mutating the bytes cannot corrupt the registry
// for others.
func GeneratedComponentDeploymentsYAML() []byte {
	return append([]byte(nil), componentDeploymentsYAML...)
}

// ParseGeneratedComponentDeployments parses the embedded registry.
func ParseGeneratedComponentDeployments() (*ComponentDeploymentsConfig, error) {
	var cfg ComponentDeploymentsConfig
	if err := yaml.Unmarshal(componentDeploymentsYAML, &cfg); err != nil {
		return nil, fmt.Errorf("parsing embedded component deployments: %w", err)
	}
	return &cfg, nil
}
