package config

import (
	"testing"
)

const componentDeploymentsPath = "../../configs/component-deployments.yaml"

// TestComponentDeploymentsRules enforces the shape of
// configs/component-deployments.yaml: unique keys, required fields, and a
// strategy/scope pairing that keeps change matching sound.
func TestComponentDeploymentsRules(t *testing.T) {
	cfg, err := LoadComponentDeployments(componentDeploymentsPath)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(cfg.Components) == 0 {
		t.Fatal("no components defined")
	}

	validStrategies := map[string]bool{"ocm": true, "hive": true}
	validScopes := map[string]bool{"environment": true, "sector": true, "shard": true}

	keys := map[string]bool{}
	for i, comp := range cfg.Components {
		if comp.Key == "" {
			t.Errorf("component[%d]: missing key", i)
		}
		if keys[comp.Key] {
			t.Errorf("duplicate component key: %s", comp.Key)
		}
		keys[comp.Key] = true

		if comp.SaasName == "" {
			t.Errorf("component %s: missing saas_name", comp.Key)
		}
		if comp.Strategy == "" {
			t.Errorf("component %s: missing strategy", comp.Key)
		} else if !validStrategies[comp.Strategy] {
			t.Errorf("component %s: invalid strategy %q (want ocm or hive)", comp.Key, comp.Strategy)
		}

		if comp.Scope == "" {
			t.Errorf("component %s: missing scope", comp.Key)
		} else if !validScopes[comp.Scope] {
			t.Errorf("component %s: invalid scope %q (want environment, sector, or shard)", comp.Key, comp.Scope)
		}

		// A strategy/scope pairing that does not match how the component
		// actually deploys would silently mis-scope change matching.
		switch comp.Strategy {
		case "ocm":
			if comp.Scope != "environment" {
				t.Errorf("component %s: strategy ocm should have scope environment, got %q", comp.Key, comp.Scope)
			}
		case "hive":
			if comp.Scope != "shard" && comp.Scope != "sector" {
				t.Errorf("component %s: strategy hive should have scope shard or sector, got %q", comp.Key, comp.Scope)
			}
		}
	}
}
