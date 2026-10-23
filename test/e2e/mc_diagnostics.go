//go:build E2Etests

package e2e

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	"github.com/openshift-online/rosa-e2e/pkg/framework"
	"github.com/openshift-online/rosa-e2e/pkg/reporting"
)

func registerMCFailureDiagnostics(tc *framework.TestContext) {
	DeferCleanup(func() {
		collectMCFailureDiagnostics(CurrentSpecReport().Failed(), func() error {
			namespaces := tc.HCPNamespaces()
			if namespaces == nil {
				return nil
			}
			return reporting.WriteMCFailureDiagnostics(context.Background(), tc.MCKubeClient(), tc.MCDynamicClient(), reporting.MCNamespaces{HCPNamespace: namespaces.HCPNamespace, HCNamespace: namespaces.HCNamespace, ClusterName: namespaces.ClusterName})
		})
	})
}

func collectMCFailureDiagnostics(failed bool, collect func() error) {
	if !failed {
		return
	}
	if err := collect(); err != nil {
		GinkgoWriter.Println("WARNING: MC failure diagnostics could not be written")
	}
}
