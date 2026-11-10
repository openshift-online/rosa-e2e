package config

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LeanCIStatusConfig is the hand-edited source form of the CI status
// registry. Teams are referenced by org file name (the keys of
// configs/generated/org-teams.yaml); identity data (Jira team ID, display
// name) is filled in from the org snapshot at resolve time. Slack fields
// here are overrides that win over the org snapshot; drop them once the org
// team file carries the right channel.
type LeanCIStatusConfig struct {
	SippyURL             string                 `yaml:"sippy_url"`
	ComponentProwFilters map[string]string      `yaml:"component_prow_filters,omitempty"`
	Stopgaps             map[string]StopgapTeam `yaml:"stopgaps,omitempty"`
	Categories           []LeanStatusCategory   `yaml:"categories"`
}

// StopgapTeam carries team identity for teams the org repo does not describe
// yet. Entries are keyed by the org file name the team will get, so deleting
// the entry once the org file exists is the entire migration. The lint stays
// warn-only while any stopgap exists.
type StopgapTeam struct {
	DisplayName  string `yaml:"display_name"`
	JiraTeamID   string `yaml:"jira_team_id"`
	SlackChannel string `yaml:"slack_channel,omitempty"`
	SlackAlias   string `yaml:"slack_alias,omitempty"`
	RemoveBy     string `yaml:"remove_by"`
	Reason       string `yaml:"reason"`
}

type LeanStatusCategory struct {
	ID           string          `yaml:"id"`
	Name         string          `yaml:"name"`
	Scope        string          `yaml:"scope,omitempty"`
	Components   []string        `yaml:"components,omitempty"`
	ProwFilter   string          `yaml:"prow_filter"`
	Gating       *bool           `yaml:"gating"`
	Team         string          `yaml:"team"`
	SlackChannel string          `yaml:"slack_channel,omitempty"`
	SlackAlias   string          `yaml:"slack_alias,omitempty"`
	Labels       []string        `yaml:"labels,omitempty"`
	Jobs         []LeanStatusJob `yaml:"jobs"`
}

type LeanStatusJob struct {
	Name      string   `yaml:"name"`
	ProwJob   string   `yaml:"prow_job"`
	Gating    *bool    `yaml:"gating,omitempty"`
	Informing *bool    `yaml:"informing,omitempty"`
	Labels    []string `yaml:"labels,omitempty"`
}

func LoadLeanCIStatusConfig(path string) (*LeanCIStatusConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read lean registry %s: %w", path, err)
	}
	var cfg LeanCIStatusConfig
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to parse lean registry %s: %w", path, err)
	}
	return &cfg, nil
}
