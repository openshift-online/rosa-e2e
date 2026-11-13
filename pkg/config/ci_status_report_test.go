package config

import (
	"strings"
	"testing"
)

func TestCIStatusNewScheduledCoverageIsInformational(t *testing.T) {
	expected := map[string]string{
		"periodics-rosa-hcp-e2e-aws-vpc-cni-staging":                                           "rosa-cx-e2e",
		"periodics-rosa-hcp-external-oidc-e2e-staging":                                         "rosa-cx-e2e",
		"periodics-rosa-hcp-e2e-private-staging":                                               "rosa-e2e-stg",
		"periodics-rosa-hcp-e2e-privatelink-connectivity-mc-health-staging":                    "rosa-e2e-stg",
		"periodics-rosa-classic-sts-e2e-privatelink-staging":                                   "rosa-e2e-stg",
		"ho-upgrade-rosa-hcp-ho-upgrade-staging":                                               "rosa-e2e-stg",
		"periodics-rosa-hcp-e2e-integration-stable-4-20":                                       "rosa-e2e-int",
		"periodics-rosa-hcp-e2e-integration-stable-4-21":                                       "rosa-e2e-int",
		"periodics-rosa-hcp-e2e-integration-stable-4-22":                                       "rosa-e2e-int",
		"periodics-rosa-hcp-e2e-integration-stable-5-0":                                        "rosa-e2e-int",
		"ho-upgrade-rosa-hcp-ho-upgrade-integration":                                           "rosa-e2e-int",
		"ocm-fvt-osd-gcp-staging-ocm-fvt-periodic-cs-osd-gcp-psc-wif-sv-staging-main":          "ocm-fvt-gcp-stg",
		"ocm-fvt-osd-gcp-staging-ocm-fvt-periodic-cs-osd-gcp-wif-version-pruning-staging-main": "ocm-fvt-gcp-stg",
		"ocm-fvt-osd-gcp-staging-ocm-fvt-periodic-cs-osd-gcp-firewall-rules-staging-main":      "ocm-fvt-gcp-stg",
	}
	for _, category := range loadCIStatusConfig(t).Categories {
		for _, job := range category.Jobs {
			suffix := strings.TrimPrefix(job.ProwJob, "periodic-ci-openshift-online-rosa-e2e-main-")
			if categoryID, ok := expected[suffix]; ok {
				gates := category.Gating
				if job.Gating != nil {
					gates = *job.Gating
				}
				if category.ID != categoryID || gates {
					t.Errorf("%s: want category %s and informational coverage, got %s, gating=%t", job.ProwJob, categoryID, category.ID, gates)
				}
				delete(expected, suffix)
			}
		}
	}
	for suffix := range expected {
		t.Errorf("missing scheduled coverage: %s", suffix)
	}
}

func TestCIStatusRetiredJobsAbsent(t *testing.T) {
	retired := map[string]bool{
		"periodic-ci-openshift-online-rosa-e2e-main-periodics-rosa-hcp-e2e-staging-nightly-5-0":                     true,
		"periodic-ci-openshift-online-rosa-e2e-main-periodics-rosa-hcp-e2e-staging-candidate-5-0":                   true,
		"periodic-ci-openshift-online-rosa-e2e-main-periodics-rosa-hcp-e2e-integration-candidate-5-0":               true,
		"periodic-ci-openshift-online-rosa-e2e-main-periodics-rosa-classic-sts-e2e-staging-candidate-5-0":           true,
		"periodic-ci-openshift-online-rosa-e2e-main-periodics-fips-rosa-classic-sts-fips-e2e-staging-candidate-5-0": true,
		"periodic-ci-openshift-aws-vpce-operator-main-rosa-sts-e2e-promotion-int":                                   true,
		"periodic-ci-openshift-aws-vpce-operator-main-rosa-sts-e2e-promotion-stage":                                 true,
	}
	for _, category := range loadCIStatusConfig(t).Categories {
		for _, job := range category.Jobs {
			if retired[job.ProwJob] {
				t.Errorf("category %s contains retired job %s", category.ID, job.ProwJob)
			}
			if strings.Contains(job.ProwJob, "main-periodics-rosa-hcp-fips-") ||
				strings.Contains(job.ProwJob, "main-periodics-rosa-classic-sts-fips-") {
				t.Errorf("category %s contains pre-split FIPS job %s", category.ID, job.ProwJob)
			}
		}
	}
}

func TestCIStatusIntegrationOwnership(t *testing.T) {
	var stagingTeam, integrationTeam *JiraTeam
	for _, category := range loadCIStatusConfig(t).Categories {
		switch category.ID {
		case "rosa-e2e-stg":
			stagingTeam = category.Team
		case "rosa-e2e-int":
			integrationTeam = category.Team
		}
	}
	if stagingTeam == nil || integrationTeam == nil {
		t.Fatal("both platform environments must have category ownership")
	}
	if *stagingTeam != *integrationTeam {
		t.Error("integration must resolve the existing platform owner mapping")
	}
	if integrationTeam.ID == "" || integrationTeam.Name == "" || integrationTeam.SlackAlias == "" {
		t.Error("integration ownership must include identification and escalation metadata")
	}
}
