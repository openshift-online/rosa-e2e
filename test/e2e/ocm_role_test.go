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

// OCM roles are mandatory for ROSA cluster operations (ROSA-637). These specs validate that the
// organization under test has an OCM role linked, so cluster operations succeed after enforcement.
// See ROSAENG-64626.
var _ = Describe("Management Plane: OCM Role Linkage", labels.High, labels.Positive, labels.HCP, labels.Classic, labels.ManagementPlane, func() {
	It("should have an OCM role linked for the test's AWS account", func(ctx context.Context) {
		if cfg.AWSAccountID == "" {
			Skip("AWS_ACCOUNT_ID not set; cannot verify account-specific OCM role linkage")
		}
		By(fmt.Sprintf("Verifying an OCM role is linked for AWS account %s (sts_ocm_role)", cfg.AWSAccountID))
		Expect(verifiers.VerifyOCMRoleLinkedForAccount(ctx, conn, cfg.AWSAccountID)).To(Succeed())
	})

	It("should have at least one OCM role linked to the organization", func(ctx context.Context) {
		By("Verifying the organization has an OCM role linked (sts_ocm_role)")
		Expect(verifiers.VerifyOCMRoleLinked(ctx, conn)).To(Succeed())
	})

	It("should expose linked OCM role ARNs", func(ctx context.Context) {
		By("Resolving the linked OCM role ARNs")
		arns, err := verifiers.GetLinkedOCMRoleARNs(ctx, conn)
		Expect(err).NotTo(HaveOccurred())
		Expect(arns).NotTo(BeEmpty(), "expected at least one linked OCM role ARN")
		for _, a := range arns {
			Expect(a).To(HavePrefix("arn:aws:iam::"), "linked OCM role should be a valid IAM role ARN")
		}
	})
})
