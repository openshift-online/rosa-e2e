//go:build E2Etests

package e2e

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	"github.com/openshift-online/rosa-e2e/pkg/framework"
	"github.com/openshift-online/rosa-e2e/pkg/reporting"
)

func registerMCFailureDiagnostics(tc *framework.TestContext) {
	deferMCFailureDiagnostics(func(cleanup func()) { DeferCleanup(cleanup) }, func() bool { return CurrentSpecReport().Failed() }, func() error {
		namespaces := tc.HCPNamespaces()
		if namespaces == nil {
			return errors.New("MC diagnostic namespaces unavailable")
		}
		return reporting.WriteMCFailureDiagnostics(context.Background(), tc.MCKubeClient(), tc.MCDynamicClient(), reporting.MCNamespaces{HCPNamespace: namespaces.HCPNamespace, HCNamespace: namespaces.HCNamespace, ClusterName: namespaces.ClusterName})
	})
}

func deferMCFailureDiagnostics(register func(func()), failed func() bool, collect func() error) {
	register(func() { collectMCFailureDiagnostics(failed(), collect) })
}

func collectMCFailureDiagnostics(failed bool, collect func() error) {
	if !failed {
		return
	}
	if err := collect(); err != nil {
		GinkgoWriter.Println("WARNING: MC failure diagnostics could not be written")
	}
}
