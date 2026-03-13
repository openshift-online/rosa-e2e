package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds all configuration for the e2e test suite.
type Config struct {
	// OCM connection settings
	OCMEnv   string `yaml:"ocm_env"`
	OCMToken string `yaml:"-"` // never serialize tokens

	// Existing cluster (skip provisioning when set)
	ClusterID string `yaml:"cluster_id"`

	// AWS infrastructure (pre-provisioned)
	AWSRegion          string   `yaml:"aws_region"`
	AWSAccountID       string   `yaml:"aws_account_id"`
	SubnetIDs          []string `yaml:"subnet_ids"`
	OIDCConfigID       string   `yaml:"oidc_config_id"`
	AccountRolePrefix  string   `yaml:"account_role_prefix"`
	OperatorRolePrefix string   `yaml:"operator_role_prefix"`
	BillingAccountID   string   `yaml:"billing_account_id"`
	CreatorARN         string   `yaml:"creator_arn"`

	// Cluster parameters
	ClusterNamePrefix  string `yaml:"cluster_name_prefix"`
	ComputeMachineType string `yaml:"compute_machine_type"`
	ComputeNodes       int    `yaml:"compute_nodes"`
	ChannelGroup       string `yaml:"channel_group"`
	OpenShiftVersion   string `yaml:"openshift_version"`

	// Management cluster access (for HCP namespace checks)
	ManagementClusterID string `yaml:"management_cluster_id"`

	// Persistent sector mode
	SectorName string `yaml:"sector_name"`
}

// OCMBaseURL returns the OCM API URL for the configured environment.
func (c *Config) OCMBaseURL() string {
	if url := os.Getenv("OCM_BASE_URL"); url != "" {
		return url
	}
	switch c.OCMEnv {
	case "production", "prod":
		return "https://api.openshift.com"
	case "staging", "stage":
		return "https://api.stage.openshift.com"
	default:
		return "https://api.integration.openshift.com"
	}
}

// Load reads configuration from an optional YAML file and environment variables.
// Environment variables take precedence over YAML values.
func Load() (*Config, error) {
	cfg := &Config{
		OCMEnv:             "integration",
		AWSRegion:          "us-east-1",
		ClusterNamePrefix:  "e2e",
		ComputeMachineType: "m5.xlarge",
		ComputeNodes:       2,
		ChannelGroup:       "stable",
	}

	if configPath := os.Getenv("CLUSTER_CONFIG"); configPath != "" {
		data, err := os.ReadFile(configPath)
		if err != nil {
			return nil, fmt.Errorf("reading config file %s: %w", configPath, err)
		}
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing config file %s: %w", configPath, err)
		}
	}

	// Environment variables override YAML values
	if v := os.Getenv("OCM_ENV"); v != "" {
		cfg.OCMEnv = v
	}
	if v := os.Getenv("OCM_TOKEN"); v != "" {
		cfg.OCMToken = v
	}
	if v := os.Getenv("CLUSTER_ID"); v != "" {
		cfg.ClusterID = v
	}
	if v := os.Getenv("AWS_REGION"); v != "" {
		cfg.AWSRegion = v
	}
	if v := os.Getenv("AWS_ACCOUNT_ID"); v != "" {
		cfg.AWSAccountID = v
	}
	if v := os.Getenv("SUBNET_IDS"); v != "" {
		cfg.SubnetIDs = strings.Split(v, ",")
	}
	if v := os.Getenv("OIDC_CONFIG_ID"); v != "" {
		cfg.OIDCConfigID = v
	}
	if v := os.Getenv("ACCOUNT_ROLE_PREFIX"); v != "" {
		cfg.AccountRolePrefix = v
	}
	if v := os.Getenv("OPERATOR_ROLE_PREFIX"); v != "" {
		cfg.OperatorRolePrefix = v
	}
	if v := os.Getenv("BILLING_ACCOUNT_ID"); v != "" {
		cfg.BillingAccountID = v
	}
	if v := os.Getenv("CREATOR_ARN"); v != "" {
		cfg.CreatorARN = v
	}
	if v := os.Getenv("CLUSTER_NAME_PREFIX"); v != "" {
		cfg.ClusterNamePrefix = v
	}
	if v := os.Getenv("COMPUTE_MACHINE_TYPE"); v != "" {
		cfg.ComputeMachineType = v
	}
	if v := os.Getenv("COMPUTE_NODES"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid COMPUTE_NODES value %q: %w", v, err)
		}
		cfg.ComputeNodes = n
	}
	if v := os.Getenv("CHANNEL_GROUP"); v != "" {
		cfg.ChannelGroup = v
	}
	if v := os.Getenv("OPENSHIFT_VERSION"); v != "" {
		cfg.OpenShiftVersion = v
	}
	if v := os.Getenv("MANAGEMENT_CLUSTER_ID"); v != "" {
		cfg.ManagementClusterID = v
	}
	if v := os.Getenv("SECTOR_NAME"); v != "" {
		cfg.SectorName = v
	}

	if cfg.OCMToken == "" {
		return nil, fmt.Errorf("OCM_TOKEN environment variable is required")
	}

	return cfg, nil
}
