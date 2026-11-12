//go:build E2Etests

package e2e

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift-online/rosa-e2e/pkg/framework"
	"github.com/openshift-online/rosa-e2e/pkg/labels"
)

// sharedVPCTestContext carries the cross-account shared VPC inputs published by
// the provisioning steps (ROSAENG-63353): the private hosted zone and the
// network-manager role live in a separate AWS account (the "shared account")
// from the cluster's installer role.
type sharedVPCTestContext struct {
	tc              *framework.TestContext
	hostedZoneID    string
	roleARN         string
	baseDomain      string
	installerRole   string
	sharedCredsFile string
}

// setupSharedVPC resolves the shared VPC inputs from the environment and
// initialises the AWS client for the shared account, skipping on clusters that
// were not provisioned with a shared VPC.
func setupSharedVPC(ctx context.Context) *sharedVPCTestContext {
	if cfg.ClusterID == "" {
		Skip("CLUSTER_ID not set")
	}

	hostedZoneID := os.Getenv("SHARED_VPC_HOSTED_ZONE_ID")
	roleARN := os.Getenv("SHARED_VPC_ROLE_ARN")
	baseDomain := os.Getenv("SHARED_VPC_BASE_DOMAIN")
	if hostedZoneID == "" || roleARN == "" || baseDomain == "" {
		Skip("Cluster is not provisioned in a shared VPC (SHARED_VPC_* environment not set)")
	}

	sharedCredsFile := os.Getenv("SHARED_VPC_AWS_SHARED_CREDENTIALS_FILE")
	if sharedCredsFile == "" {
		Skip("Shared VPC account credentials not available (SHARED_VPC_AWS_SHARED_CREDENTIALS_FILE not set)")
	}

	tc := framework.NewTestContext(cfg, conn)
	return &sharedVPCTestContext{
		tc:              tc,
		hostedZoneID:    hostedZoneID,
		roleARN:         roleARN,
		baseDomain:      baseDomain,
		installerRole:   os.Getenv("SHARED_VPC_INSTALLER_ROLE_ARN"),
		sharedCredsFile: sharedCredsFile,
	}
}

// sharedAccountConfig loads an AWS configuration authenticated as the shared
// VPC account, distinct from the cluster account credentials the framework
// uses by default.
func (s *sharedVPCTestContext) sharedAccountConfig(ctx context.Context) aws.Config {
	resp, err := s.tc.Connection().ClustersMgmt().V1().Clusters().Cluster(cfg.ClusterID).Get().SendContext(ctx)
	Expect(err).NotTo(HaveOccurred())
	region := resp.Body().Region().ID()
	Expect(region).NotTo(BeEmpty(), "cluster region not reported by OCM")

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithSharedConfigFiles([]string{s.sharedCredsFile}),
	)
	Expect(err).NotTo(HaveOccurred(), "loading shared VPC account AWS config")
	return awsCfg
}

// phzRecordNames returns every record name (trailing-dot form) in the shared
// account's private hosted zone.
func (s *sharedVPCTestContext) phzRecordNames(ctx context.Context) []string {
	client := route53.NewFromConfig(s.sharedAccountConfig(ctx))
	var names []string
	paginator := route53.NewListResourceRecordSetsPaginator(client,
		&route53.ListResourceRecordSetsInput{HostedZoneId: aws.String(s.hostedZoneID)})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		Expect(err).NotTo(HaveOccurred(), "listing shared VPC private hosted zone record sets")
		for _, rs := range page.ResourceRecordSets {
			names = append(names, aws.ToString(rs.Name))
		}
	}
	Expect(names).NotTo(BeEmpty(), "shared VPC private hosted zone has no record sets")
	return names
}

func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	Expect(err).NotTo(HaveOccurred(), "parsing %s", rawURL)
	Expect(u.Hostname()).NotTo(BeEmpty())
	return strings.ToLower(u.Hostname())
}

// ── Customer plane: cross-account DNS through the shared private zone ──────

var _ = Describe("Shared VPC Cross-Account DNS",
	labels.High, labels.Positive, labels.HCP, labels.Classic, labels.SharedVPC,
	func() {
		It("should serve the cluster's API and ingress records from the cross-account private hosted zone", func(ctx context.Context) {
			s := setupSharedVPC(ctx)

			By("Querying cluster endpoints from OCM")
			resp, err := s.tc.Connection().ClustersMgmt().V1().Clusters().Cluster(cfg.ClusterID).Get().SendContext(ctx)
			Expect(err).NotTo(HaveOccurred())

			By("Validating the cluster DNS base domain is the shared VPC domain")
			Expect(resp.Body().DNS().BaseDomain()).To(Equal(s.baseDomain),
				"cluster base domain should be the shared VPC dns domain")

			clusterName := resp.Body().Name()
			apiHost := hostOf(resp.Body().API().URL())
			consoleHost := hostOf(resp.Body().Console().URL())
			ingressSuffix := "apps." + clusterName + "." + s.baseDomain + "."

			By("Listing record sets in the shared account private hosted zone")
			names := s.phzRecordNames(ctx)

			By("Validating the API endpoint record exists in the shared private hosted zone")
			Expect(names).To(ContainElement(apiHost+"."),
				"private hosted zone %s should contain the API record", s.hostedZoneID)

			By("Validating the wildcard ingress record exists in the shared private hosted zone")
			var foundIngress bool
			for _, name := range names {
				if strings.HasSuffix(name, ingressSuffix) {
					foundIngress = true
					break
				}
			}
			Expect(foundIngress).To(BeTrue(),
				"private hosted zone %s should contain a wildcard record for %s", s.hostedZoneID, ingressSuffix)
			GinkgoWriter.Printf("Ingress records serve %s (console: %s)\n", ingressSuffix, consoleHost)
		})
	})

// ── Management plane: cross-account role trust wiring ──────────────────────

var _ = Describe("Shared VPC Role Trust",
	labels.Medium, labels.Positive, labels.HCP, labels.Classic, labels.ManagementPlane, labels.SharedVPC,
	func() {
		It("should allow the cluster's installer role to assume the shared VPC role", func(ctx context.Context) {
			s := setupSharedVPC(ctx)
			if s.installerRole == "" {
				Skip("SHARED_VPC_INSTALLER_ROLE_ARN not set by the provisioning steps")
			}

			By("Reading the shared VPC role trust policy")
			roleName := s.roleARN[strings.LastIndex(s.roleARN, "/")+1:]
			client := iam.NewFromConfig(s.sharedAccountConfig(ctx))
			out, err := client.GetRole(ctx, &iam.GetRoleInput{RoleName: aws.String(roleName)})
			Expect(err).NotTo(HaveOccurred(), "getting shared VPC role %s", roleName)

			var policy struct {
				Statement []struct {
					Effect    string `json:"Effect"`
					Action    string `json:"Action"`
					Principal struct {
						AWS   json.RawMessage `json:"AWS"`
					} `json:"Principal"`
				} `json:"Statement"`
			}
			Expect(json.Unmarshal([]byte(aws.ToString(out.Role.AssumeRolePolicyDocument)), &policy)).To(Succeed())

			By("Validating the installer role is a trusted principal")
			var principals []string
			for _, stmt := range policy.Statement {
				if stmt.Effect != "Allow" || !strings.Contains(stmt.Action, "sts:AssumeRole") {
					continue
				}
				var single string
				if json.Unmarshal(stmt.Principal.AWS, &single) == nil {
					principals = append(principals, single)
					continue
				}
				var list []string
				Expect(json.Unmarshal(stmt.Principal.AWS, &list)).To(Succeed())
				principals = append(principals, list...)
			}
			GinkgoWriter.Printf("Shared VPC role %s trusts: %v\n", roleName, principals)
			Expect(principals).To(ContainElement(s.installerRole),
				"shared VPC role trust policy should include the cluster installer role")
		})
	})
