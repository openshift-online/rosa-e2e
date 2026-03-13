//go:build E2Etests

package e2e

import (
	. "github.com/onsi/ginkgo/v2"

	"github.com/openshift-online/rosa-e2e/pkg/labels"
)

var _ = Describe("Infrastructure: Service Cluster Health", labels.High, labels.Positive, labels.HCP, labels.Infrastructure, func() {
	PIt("should have ACM hub running")

	PIt("should have cert-manager operational")

	PIt("should have Hive controllers running")

	PIt("should have MCE components healthy")
})
