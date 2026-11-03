package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCIStatusNewScheduledCoverageIsInformational(t *testing.T) {
	expected := map[string]string{
		"periodics-rosa-hcp-e2e-aws-vpc-cni-staging":                                           "rosa-e2e-stg",
		"periodics-rosa-hcp-external-oidc-e2e-staging":                                         "rosa-e2e-stg",
		"periodics-rosa-hcp-e2e-private-staging":                                               "rosa-e2e-stg",
		"periodics-rosa-hcp-e2e-privatelink-connectivity-mc-health-staging":                    "rosa-e2e-stg",
		"periodics-rosa-classic-sts-e2e-privatelink-staging":                                   "rosa-e2e-stg",
		"ho-upgrade-rosa-hcp-ho-upgrade-staging":                                               "rosa-e2e-stg",
		"periodics-rosa-hcp-e2e-integration-stable-4-20":                                       "rosa-e2e-int",
		"periodics-rosa-hcp-e2e-integration-stable-4-21":                                       "rosa-e2e-int",
		"periodics-rosa-hcp-e2e-integration-stable-4-22":                                       "rosa-e2e-int",
		"periodics-rosa-hcp-e2e-integration-candidate-5-0":                                     "rosa-e2e-int",
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

func TestCIStatusReportOffline(t *testing.T) {
	config := loadCIStatusConfig(t)
	temporary := t.TempDir()
	requests := filepath.Join(temporary, "requests")
	if err := os.Mkdir(requests, 0700); err != nil {
		t.Fatal(err)
	}
	noData := "periodic-ci-openshift-online-rosa-e2e-main-periodics-rosa-hcp-e2e-staging-candidate-5-0"
	failing := "periodic-ci-openshift-online-rosa-e2e-main-ocm-fvt-osd-gcp-staging-ocm-fvt-periodic-cs-osd-gcp-firewall-rules-staging-main"
	fixture := `#!/bin/bash
set -euo pipefail
job="${!#}"
job="${job##*/}"
touch "${REQUESTS}/${job}"
if [[ "${job}" == "${NO_DATA_JOB}" ]]; then
  printf 'var allBuilds = [];'
elif [[ "${job}" == "${FAILING_JOB}" ]]; then
  printf 'var allBuilds = [{"ID":"123","Result":"FAILURE"}];'
else
  printf 'var allBuilds = [{"ID":"123","Result":"SUCCESS"}];'
fi
`
	if err := os.WriteFile(filepath.Join(temporary, "curl"), []byte(fixture), 0700); err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), "bash", "../../scripts/ci-status-report.sh")
	command.Env = append(os.Environ(), "PATH="+temporary+":"+os.Getenv("PATH"), "REQUESTS="+requests, "NO_DATA_JOB="+noData, "FAILING_JOB="+failing)
	output, err := command.CombinedOutput()
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
		t.Fatalf("expected report failure for the fixture's failed job, got %v\n%s", err, output)
	}
	total, gating := 0, 0
	for _, category := range config.Categories {
		for _, job := range category.Jobs {
			total++
			gates := category.Gating
			if job.Gating != nil {
				gates = *job.Gating
			}
			if gates {
				gating++
			}
			if _, err := os.Stat(filepath.Join(requests, job.ProwJob)); err != nil {
				t.Errorf("report did not request configured job %s: %v", job.ProwJob, err)
			}
			if job.ProwJob == noData && (!gates || category.ID != "rosa-e2e-stg") {
				t.Error("renamed HCP job must retain its category and gate")
			}
			if job.ProwJob == failing && (gates || category.ID != "ocm-fvt-gcp-stg") {
				t.Error("new GCP coverage must be informational in its existing category")
			}
		}
	}
	entries, err := os.ReadDir(requests)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != total {
		t.Errorf("requested %d distinct jobs, want %d", len(entries), total)
	}
	for _, expected := range []string{
		fmt.Sprintf("Passing: %d/%d", total-2, total),
		fmt.Sprintf("Failing: 1/%d", total),
		fmt.Sprintf("Other:   1/%d (running/no data)", total),
		fmt.Sprintf("Gating:      %d/%d passing (100%%)  [1 no-data/running]", gating-1, gating-1),
		fmt.Sprintf("Non-gating:  %d/%d passing", total-gating-1, total-gating),
	} {
		if !strings.Contains(string(output), expected) {
			t.Errorf("report missing %q\n%s", expected, output)
		}
	}
}
