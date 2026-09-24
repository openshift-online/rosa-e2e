//go:build E2Etests

package e2e

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/openshift-online/rosa-e2e/pkg/framework"
	"github.com/openshift-online/rosa-e2e/pkg/labels"
)

var _ = Describe("ROSA HCP Zero Egress", labels.HCP, labels.ZeroEgress, func() {
	It("should report zero egress and PrivateLink enabled", labels.Critical, labels.Positive, labels.ManagedService, func(ctx context.Context) {
		tc := requireZeroEgressCluster()

		By("Querying the cluster configuration from OCM")
		resp, err := tc.Connection().ClustersMgmt().V1().Clusters().Cluster(cfg.ClusterID).Get().SendContext(ctx)
		Expect(err).NotTo(HaveOccurred())

		zeroEgressEnabled, err := framework.IsZeroEgressEnabled(resp.Body())
		Expect(err).NotTo(HaveOccurred())
		Expect(zeroEgressEnabled).To(BeTrue(), "zero egress should be enabled")
		Expect(resp.Body().AWS().PrivateLink()).To(BeTrue(), "zero egress clusters must use PrivateLink")
	})

	It("should have all required AWS VPC endpoints", labels.Critical, labels.Positive, labels.Infrastructure, func(ctx context.Context) {
		tc := requireZeroEgressCluster()
		initZeroEgressAWS(ctx, tc)

		By("Resolving the worker VPC from the configured subnets")
		vpcID, err := resolveWorkerVPC(ctx, tc.EC2Client(), cfg.SubnetIDs)
		Expect(err).NotTo(HaveOccurred())

		By("Checking the zero-egress endpoint contract")
		endpoints, err := availableVPCEndpoints(ctx, tc.EC2Client(), vpcID)
		Expect(err).NotTo(HaveOccurred())

		requiredServices := []string{"sts", "ecr.api", "ecr.dkr", "s3"}
		for _, service := range requiredServices {
			serviceName := fmt.Sprintf("com.amazonaws.%s.%s", cfg.AWSRegion, service)
			endpoint, found := endpoints[serviceName]
			Expect(found).To(BeTrue(), "required VPC endpoint %s is missing or unavailable", serviceName)

			if endpoint.VpcEndpointType == types.VpcEndpointTypeInterface {
				Expect(aws.ToBool(endpoint.PrivateDnsEnabled)).To(BeTrue(), "%s must have private DNS enabled", serviceName)
				for _, subnetID := range cfg.SubnetIDs {
					Expect(endpoint.SubnetIds).To(ContainElement(subnetID), "%s must be attached to worker subnet %s", serviceName, subnetID)
				}
			}
		}

		s3Endpoint := endpoints[fmt.Sprintf("com.amazonaws.%s.s3", cfg.AWSRegion)]
		if s3Endpoint.VpcEndpointType == types.VpcEndpointTypeGateway {
			By("Checking the S3 gateway endpoint route-table associations")
			routeTables, err := workerRouteTables(ctx, tc.EC2Client(), vpcID, cfg.SubnetIDs)
			Expect(err).NotTo(HaveOccurred())
			for subnetID, routeTable := range routeTables {
				Expect(s3Endpoint.RouteTableIds).To(ContainElement(aws.ToString(routeTable.RouteTableId)),
					"S3 gateway endpoint must be associated with the route table for subnet %s", subnetID)
			}
		}
	})

	It("should mirror protected release registries to regional ECR", labels.High, labels.Positive, labels.ManagedService, func(ctx context.Context) {
		tc := requireZeroEgressCluster()

		By("Initializing hosted cluster clients")
		Expect(tc.InitHCClients()).To(Succeed())

		By("Reading ImageDigestMirrorSet configuration")
		gvr := schema.GroupVersionResource{
			Group:    "config.openshift.io",
			Version:  "v1",
			Resource: "imagedigestmirrorsets",
		}
		list, err := tc.HCDynamicClient().Resource(gvr).List(ctx, metav1.ListOptions{})
		Expect(err).NotTo(HaveOccurred())

		expectedHostSuffix := fmt.Sprintf(".dkr.ecr.%s.amazonaws.com", cfg.AWSRegion)

		var protectedMirrors []string
		for _, item := range list.Items {
			rules, found, err := unstructured.NestedSlice(item.Object, "spec", "imageDigestMirrors")
			Expect(err).NotTo(HaveOccurred())
			if !found {
				continue
			}
			for _, rule := range rules {
				ruleMap, ok := rule.(map[string]interface{})
				Expect(ok).To(BeTrue(), "invalid imageDigestMirrors entry in %s", item.GetName())
				source, _, _ := unstructured.NestedString(ruleMap, "source")
				if !strings.HasPrefix(source, "quay.io/openshift-release-dev/") {
					continue
				}
				mirrors, _, err := unstructured.NestedStringSlice(ruleMap, "mirrors")
				Expect(err).NotTo(HaveOccurred())
				protectedMirrors = append(protectedMirrors, mirrors...)
			}
		}

		Expect(protectedMirrors).NotTo(BeEmpty(), "no mirrors found for protected OpenShift release registries")
		for _, mirror := range protectedMirrors {
			host := strings.Split(strings.TrimPrefix(strings.TrimPrefix(mirror, "https://"), "http://"), "/")[0]
			Expect(host).To(HaveSuffix(expectedHostSuffix), "mirror %q does not use regional ECR", mirror)
		}
	})

})

func requireZeroEgressCluster() *framework.TestContext {
	GinkgoHelper()

	if cfg.ClusterID == "" {
		Skip("CLUSTER_ID not set, skipping zero egress tests")
	}

	tc := framework.NewTestContext(cfg, conn)
	if !tc.IsHCP() {
		Skip("zero egress tests only apply to ROSA HCP clusters")
	}

	enabled, err := tc.IsZeroEgress()
	Expect(err).NotTo(HaveOccurred())
	if !enabled {
		Skip("target cluster does not have zero egress enabled")
	}

	return tc
}

func initZeroEgressAWS(ctx context.Context, tc *framework.TestContext) {
	GinkgoHelper()

	Expect(cfg.SubnetIDs).NotTo(BeEmpty(), "SUBNET_IDS must be set for zero egress infrastructure tests")
	if err := tc.InitAWSClients(ctx); err != nil || !tc.HasAWSAccess() {
		Skip(fmt.Sprintf("AWS credentials not available, skipping zero egress infrastructure test: %v", err))
	}
}

func resolveWorkerVPC(ctx context.Context, client *ec2.Client, subnetIDs []string) (string, error) {
	resp, err := client.DescribeSubnets(ctx, &ec2.DescribeSubnetsInput{SubnetIds: subnetIDs})
	if err != nil {
		return "", fmt.Errorf("describing worker subnets: %w", err)
	}
	if len(resp.Subnets) != len(subnetIDs) {
		return "", fmt.Errorf("resolved %d of %d configured worker subnets", len(resp.Subnets), len(subnetIDs))
	}

	vpcID := ""
	for _, subnet := range resp.Subnets {
		if vpcID == "" {
			vpcID = aws.ToString(subnet.VpcId)
		}
		if aws.ToString(subnet.VpcId) != vpcID {
			return "", fmt.Errorf("worker subnets span multiple VPCs")
		}
	}
	if vpcID == "" {
		return "", fmt.Errorf("worker subnets have no VPC ID")
	}
	return vpcID, nil
}

func availableVPCEndpoints(ctx context.Context, client *ec2.Client, vpcID string) (map[string]types.VpcEndpoint, error) {
	paginator := ec2.NewDescribeVpcEndpointsPaginator(client, &ec2.DescribeVpcEndpointsInput{
		Filters: []types.Filter{{Name: aws.String("vpc-id"), Values: []string{vpcID}}},
	})

	endpoints := make(map[string]types.VpcEndpoint)
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describing VPC endpoints: %w", err)
		}
		for _, endpoint := range page.VpcEndpoints {
			if strings.EqualFold(string(endpoint.State), string(types.StateAvailable)) {
				endpoints[aws.ToString(endpoint.ServiceName)] = endpoint
			}
		}
	}
	return endpoints, nil
}

func workerRouteTables(ctx context.Context, client *ec2.Client, vpcID string, subnetIDs []string) (map[string]types.RouteTable, error) {
	resp, err := client.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{
		Filters: []types.Filter{{Name: aws.String("association.subnet-id"), Values: subnetIDs}},
	})
	if err != nil {
		return nil, fmt.Errorf("describing worker route tables: %w", err)
	}

	result := make(map[string]types.RouteTable, len(subnetIDs))
	for _, routeTable := range resp.RouteTables {
		for _, association := range routeTable.Associations {
			subnetID := aws.ToString(association.SubnetId)
			if subnetID != "" {
				result[subnetID] = routeTable
			}
		}
	}

	if len(result) != len(subnetIDs) {
		mainResp, err := client.DescribeRouteTables(ctx, &ec2.DescribeRouteTablesInput{
			Filters: []types.Filter{
				{Name: aws.String("vpc-id"), Values: []string{vpcID}},
				{Name: aws.String("association.main"), Values: []string{"true"}},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("describing main route table: %w", err)
		}
		if len(mainResp.RouteTables) != 1 {
			return nil, fmt.Errorf("expected one main route table for VPC %s, found %d", vpcID, len(mainResp.RouteTables))
		}
		for _, subnetID := range subnetIDs {
			if _, found := result[subnetID]; !found {
				result[subnetID] = mainResp.RouteTables[0]
			}
		}

	}
	return result, nil
}
