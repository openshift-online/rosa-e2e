package config

type CIStatusConfig struct {
	SippyURL             string            `yaml:"sippy_url"`
	ComponentProwFilters map[string]string  `yaml:"component_prow_filters,omitempty"`
	Categories           []StatusCategory   `yaml:"categories"`
}

type StatusCategory struct {
	ID         string      `yaml:"id"`
	Name       string      `yaml:"name"`
	Scope      string      `yaml:"scope,omitempty"`
	Components []string    `yaml:"components,omitempty"`
	ProwFilter string      `yaml:"prow_filter"`
	Team       *JiraTeam   `yaml:"team,omitempty"`
	Labels     []string    `yaml:"labels,omitempty"`
	Jobs       []StatusJob `yaml:"jobs"`
}

type StatusJob struct {
	Name    string    `yaml:"name"`
	ProwJob string    `yaml:"prow_job"`
	Team    *JiraTeam `yaml:"team,omitempty"`
	Labels  []string  `yaml:"labels,omitempty"`
}

type JiraTeam struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
}
