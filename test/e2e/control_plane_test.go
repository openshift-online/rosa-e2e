//go:build E2Etests

package e2e

import (
	"context"
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/openshift-online/rosa-e2e/pkg/framework"
	"github.com/openshift-online/rosa-e2e/pkg/labels"
)

var _ = Describe("Control Plane: OCM API Health", labels.Critical, labels.Positive, labels.HCP, labels.ControlPlane, func() {
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

		By("Querying available ROSA HCP versions")
		resp, err := tc.Connection().ClustersMgmt().V1().Versions().List().
			Search("rosa_enabled='true' AND hosted_control_plane_enabled='true' AND enabled='true'").
			Size(5).
			SendContext(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Status()).To(Equal(http.StatusOK))
		Expect(resp.Total()).To(BeNumerically(">", 0))

		GinkgoWriter.Printf("Found %d available ROSA HCP versions\n", resp.Total())
	})
})

var _ = Describe("Control Plane: OSDFM Health", labels.Critical, labels.Positive, labels.HCP, labels.ControlPlane, func() {
	It("should list service clusters", func(ctx context.Context) {
		tc := framework.NewTestContext(cfg, conn)

		By("Querying OSDFM for service clusters")
		resp, err := tc.Connection().Get().
			Path("/api/osd_fleet_mgmt/v1/service_clusters").
			Parameter("size", 1).
			SendContext(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Status()).To(Equal(http.StatusOK))
	})

	It("should list management clusters", func(ctx context.Context) {
		tc := framework.NewTestContext(cfg, conn)

		By("Querying OSDFM for management clusters")
		resp, err := tc.Connection().Get().
			Path("/api/osd_fleet_mgmt/v1/management_clusters").
			Parameter("size", 1).
			SendContext(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Status()).To(Equal(http.StatusOK))
	})

	It("should report management clusters as ready in test region", func(ctx context.Context) {
		tc := framework.NewTestContext(cfg, conn)
		region := tc.Config().AWSRegion

		By(fmt.Sprintf("Querying OSDFM for ready MCs in %s", region))
		resp, err := tc.Connection().Get().
			Path("/api/osd_fleet_mgmt/v1/management_clusters").
			Parameter("search", fmt.Sprintf("status='ready' AND region='%s'", region)).
			SendContext(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.Status()).To(Equal(http.StatusOK))
	})
})

var _ = Describe("Control Plane: Cluster Service Health", labels.High, labels.Positive, labels.HCP, labels.ControlPlane, func() {
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
