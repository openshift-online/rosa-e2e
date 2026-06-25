package config

type CIStatusConfig struct {
	SippyURL   string           `yaml:"sippy_url"`
	Categories []StatusCategory `yaml:"categories"`
}

type StatusCategory struct {
	ID         string      `yaml:"id"`
	Name       string      `yaml:"name"`
	Scope      string      `yaml:"scope,omitempty"`
	Components []string    `yaml:"components,omitempty"`
	ProwFilter string      `yaml:"prow_filter"`
	Jobs       []StatusJob `yaml:"jobs"`
}

type StatusJob struct {
	Name    string `yaml:"name"`
	ProwJob string `yaml:"prow_job"`
}
