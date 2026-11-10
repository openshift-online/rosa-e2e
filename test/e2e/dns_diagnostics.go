//go:build E2Etests

package e2e

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	"github.com/openshift-online/rosa-e2e/pkg/reporting"
	"k8s.io/client-go/kubernetes"
)

// registerDNSFailureDiagnostics must be called after the namespace cleanup is registered:
// DeferCleanup runs callbacks in reverse order, so the snapshot is taken before deletion.
func registerDNSFailureDiagnostics(client kubernetes.Interface, namespace string) {
	ginkgo.DeferCleanup(func() {
		if !ginkgo.CurrentSpecReport().Failed() {
			return
		}
		if err := reporting.WriteDNSFailureDiagnostics(context.Background(), client, namespace); err != nil {
			ginkgo.GinkgoWriter.Println("WARNING: DNS failure diagnostics could not be written")
		}
	})
}
