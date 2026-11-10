package config

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

// ResolveSnapshot turns the lean registry plus the org snapshot into the
// CIStatusConfig that consumers (chai-bot tasks, rosa-eng-dashboard) read.
// A stopgap entry shadows an org team of the same name until it is deleted.
func ResolveSnapshot(lean *LeanCIStatusConfig, snap *OrgSnapshot) (*CIStatusConfig, error) {
	cfg := &CIStatusConfig{
		SippyURL:             lean.SippyURL,
		ComponentProwFilters: lean.ComponentProwFilters,
	}

	teams := map[string]*JiraTeam{}
	for i, lc := range lean.Categories {
		if lc.Team == "" {
			return nil, fmt.Errorf("category[%d] %s: missing team", i, lc.ID)
		}
		jt, ok := teams[lc.Team]
		if !ok {
			jt, ok = resolveTeam(lean, snap, lc.Team)
			if !ok {
				return nil, fmt.Errorf("category %s: team %q not in org snapshot or stopgaps (run `make configs-snapshot` if the org repo changed)", lc.ID, lc.Team)
			}
			teams[lc.Team] = jt
		}

		resolved := *jt
		if lc.SlackChannel != "" {
			resolved.SlackChannel = lc.SlackChannel
		}
		if lc.SlackAlias != "" {
			resolved.SlackAlias = lc.SlackAlias
		}

		cat := StatusCategory{
			ID:         lc.ID,
			Name:       lc.Name,
			Scope:      lc.Scope,
			Components: lc.Components,
			ProwFilter: lc.ProwFilter,
			Team:       &resolved,
			Labels:     lc.Labels,
		}
		if lc.Gating != nil {
			cat.Gating = *lc.Gating
		}
		for _, lj := range lc.Jobs {
			cat.Jobs = append(cat.Jobs, StatusJob{
				Name:      lj.Name,
				ProwJob:   lj.ProwJob,
				Gating:    lj.Gating,
				Informing: lj.Informing,
				Labels:    lj.Labels,
			})
		}
		cfg.Categories = append(cfg.Categories, cat)
	}
	return cfg, nil
}

func resolveTeam(lean *LeanCIStatusConfig, snap *OrgSnapshot, name string) (*JiraTeam, bool) {
	if s, ok := lean.Stopgaps[name]; ok {
		return &JiraTeam{
			ID:           s.JiraTeamID,
			Name:         s.DisplayName,
			SlackChannel: s.SlackChannel,
			SlackAlias:   s.SlackAlias,
		}, true
	}
	if t, ok := snap.Teams[name]; ok {
		return &JiraTeam{
			ID:           t.JiraTeamID,
			Name:         t.DisplayName,
			SlackChannel: t.SlackChannel,
		}, true
	}
	return nil, false
}

// MarshalCIStatus renders a resolved registry deterministically. Both
// configgen and the lint test use this, so their output always matches.
func MarshalCIStatus(cfg *CIStatusConfig) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(cfg); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// MarshalOrgSnapshot renders a snapshot deterministically (map keys and
// lists are sorted before reaching here; only GeneratedAt varies).
func MarshalOrgSnapshot(snap *OrgSnapshot) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(snap); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
