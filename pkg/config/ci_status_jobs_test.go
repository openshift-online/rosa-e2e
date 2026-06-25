package config

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

const ciStatusJobsPath = "../../configs/ci-status-jobs.yaml"

func loadCIStatusConfig(t *testing.T) CIStatusConfig {
	t.Helper()
	data, err := os.ReadFile(ciStatusJobsPath)
	if err != nil {
		t.Fatalf("failed to read %s: %v", ciStatusJobsPath, err)
	}
	var cfg CIStatusConfig
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		t.Fatalf("failed to parse %s: %v", ciStatusJobsPath, err)
	}
	return cfg
}

func TestCIStatusJobsParses(t *testing.T) {
	cfg := loadCIStatusConfig(t)
	if len(cfg.Categories) == 0 {
		t.Fatal("no categories defined")
	}
}

func TestCIStatusJobsRequiredFields(t *testing.T) {
	cfg := loadCIStatusConfig(t)
	for i, cat := range cfg.Categories {
		if cat.ID == "" {
			t.Errorf("category[%d]: missing id", i)
		}
		if cat.Name == "" {
			t.Errorf("category[%d] (%s): missing name", i, cat.ID)
		}
		if len(cat.Jobs) == 0 {
			t.Errorf("category %s: no jobs defined", cat.ID)
		}
		for j, job := range cat.Jobs {
			if job.Name == "" {
				t.Errorf("category %s job[%d]: missing name", cat.ID, j)
			}
			if job.ProwJob == "" {
				t.Errorf("category %s job[%d]: missing prow_job", cat.ID, j)
			}
		}
	}
}

func TestCIStatusJobsNoDuplicates(t *testing.T) {
	cfg := loadCIStatusConfig(t)

	catIDs := make(map[string]bool)
	for _, cat := range cfg.Categories {
		if catIDs[cat.ID] {
			t.Errorf("duplicate category id: %s", cat.ID)
		}
		catIDs[cat.ID] = true
	}

	prowJobs := make(map[string]string)
	for _, cat := range cfg.Categories {
		for _, job := range cat.Jobs {
			if prev, ok := prowJobs[job.ProwJob]; ok {
				t.Errorf("duplicate prow_job %s (in %s and %s)", job.ProwJob, prev, cat.ID)
			}
			prowJobs[job.ProwJob] = cat.ID
		}
	}
}

func TestCIStatusJobsNamingConvention(t *testing.T) {
	cfg := loadCIStatusConfig(t)
	for _, cat := range cfg.Categories {
		for _, job := range cat.Jobs {
			if !strings.HasPrefix(job.ProwJob, "periodic-ci-") {
				t.Errorf("category %s job %s: prow_job %q does not start with periodic-ci-", cat.ID, job.Name, job.ProwJob)
			}
		}
	}
}

func TestCIStatusJobsExistInProw(t *testing.T) {
	if os.Getenv("VALIDATE_PROW") == "" {
		t.Skip("set VALIDATE_PROW=1 to validate job names against Prow (requires network)")
	}

	cfg := loadCIStatusConfig(t)
	client := &http.Client{Timeout: 10 * time.Second}

	var failures []string
	for _, cat := range cfg.Categories {
		for _, job := range cat.Jobs {
			jobURL := fmt.Sprintf("https://prow.ci.openshift.org/job-history/gs/test-platform-results/logs/%s", job.ProwJob)
			req, err := http.NewRequestWithContext(t.Context(), http.MethodHead, jobURL, nil)
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s/%s: invalid request: %v", cat.ID, job.Name, err))
				continue
			}
			resp, err := client.Do(req)
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s/%s: request failed: %v", cat.ID, job.Name, err))
				continue
			}
			_ = resp.Body.Close()
			if resp.StatusCode < 200 || resp.StatusCode >= 400 {
				failures = append(failures, fmt.Sprintf("%s/%s: prow_job %s returned HTTP %d", cat.ID, job.Name, job.ProwJob, resp.StatusCode))
			}
		}
	}
	for _, f := range failures {
		t.Error(f)
	}
}
