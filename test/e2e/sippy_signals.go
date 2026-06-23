//go:build E2Etests

package e2e

import (
	"os"
	"path/filepath"
	"slices"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"

	"github.com/openshift-online/rosa-e2e/pkg/reporting"
)

var _ = ReportAfterSuite("Generate Sippy signals", func(report Report) {
	outputDir := os.Getenv("ARTIFACT_DIR")
	if outputDir == "" {
		outputDir = "/tmp"
	}
	outputPath := filepath.Join(outputDir, "junit-rosa-e2e-sippy.xml")

	result := analyzeSuiteReport(report)

	if err := reporting.WriteSippyJUnit(result, outputPath); err != nil {
		GinkgoWriter.Printf("WARNING: failed to write Sippy signals: %v\n", err)
	} else {
		GinkgoWriter.Printf("Sippy signals written to %s\n", outputPath)
	}
})

func analyzeSuiteReport(report Report) reporting.SignalResult {
	result := reporting.SignalResult{
		InstallPassed:        true,
		InfrastructurePassed: true,
	}

	var infraSpecsRan bool
	var upgradeAnyFailed bool

	for _, spec := range report.SpecReports {
		// BeforeSuite failure means the cluster was unreachable
		if spec.LeafNodeType == types.NodeTypeBeforeSuite && spec.Failed() {
			result.InstallPassed = false
			result.InfrastructurePassed = false
			result.InstallFailureMsg = "BeforeSuite failed: " + spec.Failure.Message
			result.InfraFailureMsg = "cluster unreachable (BeforeSuite failed)"
			return result
		}

		if spec.LeafNodeType != types.NodeTypeIt {
			continue
		}
		if spec.State == types.SpecStateSkipped || spec.State == types.SpecStatePending {
			continue
		}

		labels := spec.Labels()

		if slices.Contains(labels, "Area:Upgrade") {
			result.UpgradeRan = true
			if spec.Failed() {
				upgradeAnyFailed = true
				if result.UpgradeFailureMsg == "" {
					result.UpgradeFailureMsg = spec.FullText() + ": " + spec.Failure.Message
				}
			}
		}

		if slices.Contains(labels, "Area:ManagedService") || slices.Contains(labels, "Area:ClusterLifecycle") {
			infraSpecsRan = true
			if spec.Failed() {
				result.InfrastructurePassed = false
				if result.InfraFailureMsg == "" {
					result.InfraFailureMsg = spec.FullText() + ": " + spec.Failure.Message
				}
			}
		}
	}

	if result.UpgradeRan {
		result.UpgradePassed = !upgradeAnyFailed
	}

	// If no infrastructure-related specs ran, default to pass
	if !infraSpecsRan {
		result.InfrastructurePassed = true
	}

	return result
}
