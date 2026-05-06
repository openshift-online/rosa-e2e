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

var _ = Describe("ROSA Infrastructure Tags", labels.High, labels.Positive, labels.HCP, labels.Classic, labels.ManagedService, func() {
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

		By("Resolving cluster identifier for EBS volume tags")
		// HCP clusters tag volumes with the OCM cluster ID; Classic clusters use the infra ID.
		clusterTag := cfg.ClusterID
		if tc.IsClassic() {
			infraID, err := tc.ResolveInfraID()
			Expect(err).NotTo(HaveOccurred(), "failed to resolve infra ID from OCM")
			clusterTag = infraID
		}

		By("Verifying EBS volumes have required cluster ownership tag")
		expectedTags := map[string]string{
			fmt.Sprintf("kubernetes.io/cluster/%s", clusterTag): "owned",
		}
		Expect(verifiers.VerifyEBSVolumesTags(ctx, tc.EC2Client(), clusterTag, expectedTags)).To(Succeed())
	})
})
