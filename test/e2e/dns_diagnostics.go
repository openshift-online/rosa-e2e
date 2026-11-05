//go:build E2Etests

package e2e

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	"github.com/openshift-online/rosa-e2e/pkg/reporting"
	"k8s.io/client-go/kubernetes"
)

func deferDNSFailureDiagnostics(register func(func()), failed func() bool, client kubernetes.Interface, namespace string) {
	register(func() { collectDNSFailureDiagnostics(failed(), client, namespace) })
}

func collectDNSFailureDiagnostics(failed bool, client kubernetes.Interface, namespace string) {
	if !failed {
		return
	}
	if err := reporting.WriteDNSFailureDiagnostics(context.Background(), client, namespace); err != nil {
		ginkgo.GinkgoWriter.Println("WARNING: DNS failure diagnostics could not be written")
	}
}
