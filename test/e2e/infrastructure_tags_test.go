//go:build E2Etests

package e2e

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift-online/rosa-e2e/pkg/framework"
	"github.com/openshift-online/rosa-e2e/pkg/labels"
	"github.com/openshift-online/rosa-e2e/pkg/verifiers"
)

var _ = Describe("ROSA HCP Infrastructure Tags", labels.High, labels.Positive, labels.HCP, labels.ManagedService, func() {
	It("should have correct tags on EBS volumes", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not configured, skipping infrastructure tags test")
		}

		tc := framework.NewTestContext(cfg, conn)

		By("Initializing AWS clients")
		err := tc.InitAWSClients(ctx)
		if err != nil || !tc.HasAWSAccess() {
			Skip("AWS credentials not available, skipping EBS tags test")
		}

		By("Verifying EBS volumes have required cluster ownership tag")
		expectedTags := map[string]string{
			fmt.Sprintf("kubernetes.io/cluster/%s", cfg.ClusterID): "owned",
		}
		Expect(verifiers.VerifyEBSVolumesTags(ctx, tc.EC2Client(), cfg.ClusterID, expectedTags)).To(Succeed())
	})
})
