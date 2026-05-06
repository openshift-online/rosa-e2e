//go:build E2Etests

package e2e

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift-online/rosa-e2e/pkg/framework"
	"github.com/openshift-online/rosa-e2e/pkg/labels"
	"github.com/openshift-online/rosa-e2e/pkg/verifiers"
)

var _ = Describe("ROSA IAM Validation", labels.Critical, labels.Positive, labels.HCP, labels.Classic, labels.ManagedService, func() {
	It("should have zero AccessDenied events in CloudTrail", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not configured, skipping IAM validation test")
		}

		tc := framework.NewTestContext(cfg, conn)

		By("Initializing AWS clients")
		err := tc.InitAWSClients(ctx)
		if err != nil || !tc.HasAWSAccess() {
			Skip("AWS credentials not available, skipping CloudTrail test")
		}

		By("Querying CloudTrail for AccessDenied events in the last 24 hours")
		Expect(verifiers.VerifyNoAccessDenied(ctx, tc.CloudTrailClient(), cfg.ClusterID, 24*time.Hour)).To(Succeed())
	})
})
