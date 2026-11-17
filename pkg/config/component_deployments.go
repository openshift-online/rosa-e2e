package config

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// ComponentDeployment maps a ROSA component to its app-interface saas file
// and records how it rolls out. Strategy describes how fetch targets map to
// environments (cluster prefix vs namespace path); scope describes the
// deployment granularity used for change matching, e.g. by change-observability.
type ComponentDeployment struct {
	Key              string   `yaml:"key"`
	SaasName         string   `yaml:"saas_name"`
	Strategy         string   `yaml:"strategy"` // ocm | hive
	Scope            string   `yaml:"scope"`    // environment | sector | shard
	E2ESaas          string   `yaml:"e2e_saas,omitempty"`
	SaasDir          string   `yaml:"saas_dir,omitempty"`
	CanaryTargets    []string `yaml:"canary_targets,omitempty"`
	CanaryNamespaces []string `yaml:"canary_namespaces,omitempty"`
}

type ComponentDeploymentsConfig struct {
	Components []ComponentDeployment `yaml:"components"`
}

func LoadComponentDeployments(path string) (*ComponentDeploymentsConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read component deployments %s: %w", path, err)
	}
	var cfg ComponentDeploymentsConfig
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to parse component deployments %s: %w", path, err)
	}
	return &cfg, nil
}
