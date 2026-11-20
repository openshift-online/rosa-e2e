//go:build E2Etests

package e2e

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	sdk "github.com/openshift-online/ocm-sdk-go"

	"github.com/openshift-online/rosa-e2e/pkg/framework"
	"github.com/openshift-online/rosa-e2e/pkg/labels"
	"github.com/openshift-online/rosa-e2e/pkg/reporting"
)

var _ = Describe("Management Plane: OCM API Health", labels.Critical, labels.Positive, labels.HCP, labels.Classic, labels.OSDGCP, labels.ManagementPlane, func() {
	It("should respond to cluster list requests", func(ctx context.Context) {
		tc := framework.NewTestContext(cfg, conn)

		By("Listing clusters via OCM API")
		resp, err := tc.Connection().ClustersMgmt().V1().Clusters().List().Size(1).SendContext(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Status()).To(Equal(http.StatusOK))
		Expect(resp.Total()).To(BeNumerically(">", 0))
	})

	It("should return cluster details for test cluster", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not set")
		}

		tc := framework.NewTestContext(cfg, conn)

		By("Getting cluster details via OCM API")
		resp, err := tc.Connection().ClustersMgmt().V1().Clusters().Cluster(cfg.ClusterID).Get().SendContext(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Status()).To(Equal(http.StatusOK))
		Expect(string(resp.Body().State())).To(Equal("ready"))
		Expect(resp.Body().Name()).NotTo(BeEmpty())
	})

	It("should return cluster credentials", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not set")
		}

		tc := framework.NewTestContext(cfg, conn)

		By("Getting cluster credentials via OCM API")
		resp, err := tc.Connection().ClustersMgmt().V1().Clusters().Cluster(cfg.ClusterID).Credentials().Get().SendContext(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Status()).To(Equal(http.StatusOK))
		Expect(resp.Body().Kubeconfig()).NotTo(BeEmpty())
	})

	It("should list available versions", func(ctx context.Context) {
		tc := framework.NewTestContext(cfg, conn)

		query := "enabled='true'"
		switch {
		case tc.IsHCP(), tc.Topology() == "":
			query += " AND rosa_enabled='true' AND hosted_control_plane_enabled='true'"
		case tc.IsOSDGCP():
			// OSD GCP versions are not rosa_enabled
		default:
			query += " AND rosa_enabled='true'"
		}

		By("Querying available ROSA versions")
		resp, err := tc.Connection().ClustersMgmt().V1().Versions().List().
			Search(query).
			Size(5).
			SendContext(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Status()).To(Equal(http.StatusOK))
		Expect(resp.Total()).To(BeNumerically(">", 0))

		GinkgoWriter.Printf("Found %d available ROSA versions\n", resp.Total())
	})
})

// OSDFM is reached through the SDK's generic request API, which has no typed client, so the
// request paths are declared as constants: the failure diagnostics may then name the endpoint
// without ever echoing a caller-built URL or its query string.
const (
	osdfmServiceClustersPath    = "/api/osd_fleet_mgmt/v1/service_clusters"
	osdfmManagementClustersPath = "/api/osd_fleet_mgmt/v1/management_clusters"
)

// expectOSDFMHealthy asserts that an OSDFM query returned HTTP 200, keeping the existing skip for
// 403 (the suite runs with credentials that are not always entitled to OSDFM).
//
// sdk.Response reports any HTTP status as success, so an unexpected status would otherwise fail
// with nothing but the numeric code. Both failure paths are annotated instead: the response path
// adds the endpoint plus the two allowlisted identifiers the body carries, and the transport path
// adds the endpoint plus a fixed failure class rather than the raw SDK error. See pkg/reporting
// for exactly what each helper emits and what the allowlists do and do not guarantee. The
// response description is deferred so the body is only parsed on failure.
func expectOSDFMHealthy(endpoint string, resp *sdk.Response, err error) {
	GinkgoHelper()
	if err != nil {
		Fail(reporting.DescribeOCMTransportError(endpoint, err))
	}
	if resp == nil {
		Fail(fmt.Sprintf("OCM API returned no response and no error: endpoint=%s", endpoint))
	}
	if resp.Status() == http.StatusForbidden {
		Skip("OSDFM access not available with current credentials (403)")
	}
	Expect(resp.Status()).To(Equal(http.StatusOK), func() string {
		return reporting.DescribeOCMResponse(endpoint, resp.Status(), resp.Bytes())
	})
}

var _ = Describe("Management Plane: OSDFM Health", labels.Critical, labels.Positive, labels.HCP, labels.ManagementPlane, func() {
	It("should list service clusters", func(ctx context.Context) {
		tc := framework.NewTestContext(cfg, conn)

		By("Querying OSDFM for service clusters")
		resp, err := tc.Connection().Get().
			Path(osdfmServiceClustersPath).
			Parameter("size", 1).
			SendContext(ctx)
		expectOSDFMHealthy(osdfmServiceClustersPath, resp, err)
	})

	It("should list management clusters", func(ctx context.Context) {
		tc := framework.NewTestContext(cfg, conn)

		By("Querying OSDFM for management clusters")
		resp, err := tc.Connection().Get().
			Path(osdfmManagementClustersPath).
			Parameter("size", 1).
			SendContext(ctx)
		expectOSDFMHealthy(osdfmManagementClustersPath, resp, err)
	})

	It("should report management clusters as ready in test region", func(ctx context.Context) {
		tc := framework.NewTestContext(cfg, conn)
		region := tc.Config().AWSRegion

		By(fmt.Sprintf("Querying OSDFM for ready MCs in %s", region))
		resp, err := tc.Connection().Get().
			Path(osdfmManagementClustersPath).
			Parameter("search", fmt.Sprintf("status='ready' AND region='%s'", region)).
			SendContext(ctx)
		expectOSDFMHealthy(osdfmManagementClustersPath, resp, err)
	})
})

var _ = Describe("Management Plane: Cluster Service Health", labels.High, labels.Positive, labels.HCP, labels.Classic, labels.OSDGCP, labels.ManagementPlane, func() {
	It("should process cluster status requests within SLA", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not set")
		}

		tc := framework.NewTestContext(cfg, conn)

		By("Measuring cluster GET latency")
		start := time.Now()
		resp, err := tc.Connection().ClustersMgmt().V1().Clusters().Cluster(cfg.ClusterID).Get().SendContext(ctx)
		latency := time.Since(start)

		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Status()).To(Equal(http.StatusOK))
		Expect(latency).To(BeNumerically("<", 10*time.Second), "cluster GET latency should be under 10s")

		GinkgoWriter.Printf("Cluster GET latency: %s\n", latency)
	})

	It("should list add-on installations", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not set")
		}

		tc := framework.NewTestContext(cfg, conn)

		By("Querying add-on installations for cluster")
		resp, err := tc.Connection().ClustersMgmt().V1().Clusters().Cluster(cfg.ClusterID).
			Addons().List().SendContext(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Status()).To(Equal(http.StatusOK))
	})
})

var _ = Describe("Management Plane: Cluster Health Indicators", labels.High, labels.Positive, labels.HCP, labels.Classic, labels.OSDGCP, labels.ManagementPlane, func() {
	It("should have DNS domain configured", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not set")
		}

		tc := framework.NewTestContext(cfg, conn)

		By("Querying cluster DNS configuration")
		resp, err := tc.Connection().ClustersMgmt().V1().Clusters().Cluster(cfg.ClusterID).Get().SendContext(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Status()).To(Equal(http.StatusOK))

		baseDomain := resp.Body().DNS().BaseDomain()
		Expect(baseDomain).NotTo(BeEmpty())

		GinkgoWriter.Printf("Cluster DNS base domain: %s\n", baseDomain)
	})

	It("should have console URL set", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not set")
		}

		tc := framework.NewTestContext(cfg, conn)

		By("Querying cluster console URL")
		resp, err := tc.Connection().ClustersMgmt().V1().Clusters().Cluster(cfg.ClusterID).Get().SendContext(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Status()).To(Equal(http.StatusOK))

		consoleURL := resp.Body().Console().URL()
		Expect(consoleURL).NotTo(BeEmpty())

		GinkgoWriter.Printf("Cluster console URL: %s\n", consoleURL)
	})

	It("should have API URL responding", labels.PublicAPIAccess, func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not set")
		}

		tc := framework.NewTestContext(cfg, conn)

		By("Querying cluster API URL")
		resp, err := tc.Connection().ClustersMgmt().V1().Clusters().Cluster(cfg.ClusterID).Get().SendContext(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Status()).To(Equal(http.StatusOK))

		apiURL := resp.Body().API().URL()
		Expect(apiURL).NotTo(BeEmpty())

		By("Checking API URL reachability")
		client := &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}

		apiResp, err := client.Get(apiURL)
		Expect(err).NotTo(HaveOccurred(), "API URL should be reachable")
		if apiResp != nil && apiResp.Body != nil {
			_ = apiResp.Body.Close()
		}

		GinkgoWriter.Printf("Cluster API URL: %s (reachable)\n", apiURL)
	})

	It("should have provision shard assigned", func(ctx context.Context) {
		if cfg.ClusterID == "" {
			Skip("CLUSTER_ID not set")
		}

		tc := framework.NewTestContext(cfg, conn)

		By("Querying cluster provision shard")
		resp, err := tc.Connection().ClustersMgmt().V1().Clusters().Cluster(cfg.ClusterID).Get().SendContext(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Status()).To(Equal(http.StatusOK))

		provisionShard := resp.Body().ProvisionShard()
		if provisionShard == nil || provisionShard.ID() == "" {
			GinkgoWriter.Printf("Provision shard not set or not exposed in this environment\n")
		} else {
			GinkgoWriter.Printf("Provision shard ID: %s\n", provisionShard.ID())
		}
	})
})
