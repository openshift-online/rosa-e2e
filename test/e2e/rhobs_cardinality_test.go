//go:build E2Etests

package e2e

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift-online/rosa-e2e/pkg/labels"
	"github.com/openshift-online/rosa-e2e/pkg/verifiers"
)

var _ = Describe("RHOBS Cardinality Gate", Label("rhobs-cardinality"), labels.High, labels.Positive, func() {
	const (
		maxDeltaPct        = 10.0
		maxSeriesPerMetric = 100000.0
	)

	var rhobsConfig *verifiers.RHOBSConfig

	BeforeEach(func() {
		rhobsConfig = verifiers.NewRHOBSConfig(cfg)
		if rhobsConfig.MetricsAPIURL == "" {
			Skip("RHOBS_METRICS_API_URL not configured")
		}
	})

	It("should not exceed cardinality thresholds after deployment", func(ctx context.Context) {
		By(fmt.Sprintf("Running cardinality checks against %s (max delta: %.0f%%, max series per metric: %.0f)", rhobsConfig.MetricsAPIURL, maxDeltaPct, maxSeriesPerMetric))

		results, err := verifiers.VerifyCardinalityThreshold(ctx, rhobsConfig, maxDeltaPct, maxSeriesPerMetric)
		Expect(err).NotTo(HaveOccurred(), "cardinality check queries should succeed")

		var failures []string
		for _, r := range results {
			if r.Baseline > 0 {
				GinkgoWriter.Printf("  %s: current=%.0f baseline=%.0f delta=%.1f%% threshold=%.0f%% %s\n",
					r.MetricName, r.Current, r.Baseline, r.DeltaPct, r.Threshold, passOrFail(r.Passed))
			} else {
				GinkgoWriter.Printf("  %s: count=%.0f threshold=%.0f %s\n",
					r.MetricName, r.Current, r.Threshold, passOrFail(r.Passed))
			}
			if !r.Passed {
				if r.Baseline > 0 {
					failures = append(failures, fmt.Sprintf("%s: %.1f%% increase (%.0f -> %.0f, threshold %.0f%%)",
						r.MetricName, r.DeltaPct, r.Baseline, r.Current, r.Threshold))
				} else {
					failures = append(failures, fmt.Sprintf("%s: %.0f series exceeds threshold %.0f",
						r.MetricName, r.Current, r.Threshold))
				}
			}
		}

		Expect(failures).To(BeEmpty(), "cardinality thresholds exceeded:\n%s", joinLines(failures))
	})
})

func passOrFail(passed bool) string {
	if passed {
		return "PASS"
	}
	return "FAIL"
}

func joinLines(lines []string) string {
	result := ""
	for _, l := range lines {
		result += "  - " + l + "\n"
	}
	return result
}
