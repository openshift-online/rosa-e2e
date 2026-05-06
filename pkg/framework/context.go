package framework

import (
	"context"
	"fmt"
	"os"

	awscfg "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudtrail"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	sdk "github.com/openshift-online/ocm-sdk-go"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/openshift-online/rosa-e2e/pkg/config"
)

// TestContext provides per-test access to configuration, OCM connection,
// and optional AWS/MC clients.
type TestContext struct {
	cfg  *config.Config
	conn *sdk.Connection

	// Hosted cluster clients (set when CLUSTER_ID is configured)
	hcRestConfig    *rest.Config
	hcKubeClient    kubernetes.Interface
	hcDynamicClient dynamic.Interface

	// Management cluster client (set when MANAGEMENT_CLUSTER_ID is configured)
	mcRestConfig    *rest.Config
	mcKubeClient    kubernetes.Interface
	mcDynamicClient dynamic.Interface

	// Service cluster client (set when SERVICE_CLUSTER_ID is configured)
	scRestConfig    *rest.Config
	scKubeClient    kubernetes.Interface
	scDynamicClient dynamic.Interface

	// Resolved HCP namespaces on the MC (set after ResolveHCPNamespaces)
	hcpNamespaces *HCPNamespaces

	// Detected cluster topology (lazy-initialized)
	topology string

	// AWS clients (set when AWS credentials are available)
	ec2Client        *ec2.Client
	cloudtrailClient *cloudtrail.Client
}

// NewTestContext creates a new test context with the given configuration and OCM connection.
func NewTestContext(cfg *config.Config, conn *sdk.Connection) *TestContext {
	return &TestContext{
		cfg:  cfg,
		conn: conn,
	}
}

// Config returns the test configuration.
func (tc *TestContext) Config() *config.Config {
	return tc.cfg
}

// Connection returns the OCM SDK connection.
func (tc *TestContext) Connection() *sdk.Connection {
	return tc.conn
}

// InitHCClients initializes kube and dynamic clients for the hosted cluster.
func (tc *TestContext) InitHCClients() error {
	restCfg, err := GetClusterCredentials(tc.conn, tc.cfg.ClusterID)
	if err != nil {
		return fmt.Errorf("getting HC credentials: %w", err)
	}
	tc.hcRestConfig = restCfg

	kubeClient, err := NewKubeClient(restCfg)
	if err != nil {
		return fmt.Errorf("creating HC kube client: %w", err)
	}
	tc.hcKubeClient = kubeClient

	dynClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("creating HC dynamic client: %w", err)
	}
	tc.hcDynamicClient = dynClient
	return nil
}

// InitMCClients initializes kube and dynamic clients for the management cluster.
// If MC_KUBECONFIG is set, uses that kubeconfig file (e.g. from backplane).
// Otherwise falls back to OCM credentials API.
func (tc *TestContext) InitMCClients() error {
	restCfg, err := resolveKubeconfig("MC_KUBECONFIG", tc.conn, tc.cfg.ManagementClusterID)
	if err != nil {
		return fmt.Errorf("getting MC credentials: %w", err)
	}
	tc.mcRestConfig = restCfg

	kubeClient, err := NewKubeClient(restCfg)
	if err != nil {
		return fmt.Errorf("creating MC kube client: %w", err)
	}
	tc.mcKubeClient = kubeClient

	dynClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("creating MC dynamic client: %w", err)
	}
	tc.mcDynamicClient = dynClient

	// Resolve HCP namespaces if we have a cluster name or ID (HCP topology only)
	if tc.cfg.ClusterID != "" && tc.Topology() == "hcp" {
		clusterName := tc.resolveClusterName()
		ns, err := ResolveHCPNamespaces(context.Background(), dynClient, clusterName, tc.cfg.ClusterID)
		if err != nil {
			// Non-fatal: some tests can still work without resolved namespaces
			return nil
		}
		tc.hcpNamespaces = ns
	}

	return nil
}

// resolveClusterName gets the cluster name from OCM.
func (tc *TestContext) resolveClusterName() string {
	resp, err := tc.conn.ClustersMgmt().V1().Clusters().Cluster(tc.cfg.ClusterID).Get().Send()
	if err != nil {
		return ""
	}
	return resp.Body().Name()
}

// ResolveInfraID gets the cluster infrastructure ID from OCM.
func (tc *TestContext) ResolveInfraID() (string, error) {
	resp, err := tc.conn.ClustersMgmt().V1().Clusters().Cluster(tc.cfg.ClusterID).Get().Send()
	if err != nil {
		return "", fmt.Errorf("getting cluster from OCM: %w", err)
	}
	infraID := resp.Body().InfraID()
	if infraID == "" {
		return "", fmt.Errorf("cluster %s has no infra ID", tc.cfg.ClusterID)
	}
	return infraID, nil
}

// HCPNamespaces returns the resolved HCP namespace info, or nil if not resolved.
func (tc *TestContext) HCPNamespaces() *HCPNamespaces {
	return tc.hcpNamespaces
}

// InitAWSClients initializes EC2 and CloudTrail clients using the default credential chain.
// Returns an error if credentials cannot be resolved (checks with STS GetCallerIdentity).
func (tc *TestContext) InitAWSClients(ctx context.Context) error {
	cfg, err := awscfg.LoadDefaultConfig(ctx, awscfg.WithRegion(tc.cfg.AWSRegion))
	if err != nil {
		return fmt.Errorf("loading AWS config: %w", err)
	}

	// Validate credentials are actually available by retrieving them eagerly.
	creds, err := cfg.Credentials.Retrieve(ctx)
	if err != nil || !creds.HasKeys() {
		return fmt.Errorf("no valid AWS credentials found: %w", err)
	}

	tc.ec2Client = ec2.NewFromConfig(cfg)
	tc.cloudtrailClient = cloudtrail.NewFromConfig(cfg)
	return nil
}

// HCKubeClient returns the hosted cluster kube client, or nil if not initialized.
func (tc *TestContext) HCKubeClient() kubernetes.Interface {
	return tc.hcKubeClient
}

// HCDynamicClient returns the hosted cluster dynamic client, or nil if not initialized.
func (tc *TestContext) HCDynamicClient() dynamic.Interface {
	return tc.hcDynamicClient
}

// MCKubeClient returns the management cluster kube client, or nil if not initialized.
func (tc *TestContext) MCKubeClient() kubernetes.Interface {
	return tc.mcKubeClient
}

// MCDynamicClient returns the management cluster dynamic client, or nil if not initialized.
func (tc *TestContext) MCDynamicClient() dynamic.Interface {
	return tc.mcDynamicClient
}

// EC2Client returns the EC2 client, or nil if not initialized.
func (tc *TestContext) EC2Client() *ec2.Client {
	return tc.ec2Client
}

// CloudTrailClient returns the CloudTrail client, or nil if not initialized.
func (tc *TestContext) CloudTrailClient() *cloudtrail.Client {
	return tc.cloudtrailClient
}

// InitSCClients initializes kube and dynamic clients for the service cluster.
// If SC_KUBECONFIG is set, uses that kubeconfig file. Otherwise uses OCM credentials API.
func (tc *TestContext) InitSCClients() error {
	restCfg, err := resolveKubeconfig("SC_KUBECONFIG", tc.conn, tc.cfg.ServiceClusterID)
	if err != nil {
		return fmt.Errorf("getting SC credentials: %w", err)
	}
	tc.scRestConfig = restCfg

	kubeClient, err := NewKubeClient(restCfg)
	if err != nil {
		return fmt.Errorf("creating SC kube client: %w", err)
	}
	tc.scKubeClient = kubeClient

	dynClient, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return fmt.Errorf("creating SC dynamic client: %w", err)
	}
	tc.scDynamicClient = dynClient
	return nil
}

// SCKubeClient returns the service cluster kube client, or nil if not initialized.
func (tc *TestContext) SCKubeClient() kubernetes.Interface {
	return tc.scKubeClient
}

// SCDynamicClient returns the service cluster dynamic client, or nil if not initialized.
func (tc *TestContext) SCDynamicClient() dynamic.Interface {
	return tc.scDynamicClient
}

// Topology returns the cluster topology, detecting it from OCM if needed.
// Returns "hcp", "classic", or "osd-gcp".
func (tc *TestContext) Topology() string {
	if tc.topology != "" {
		return tc.topology
	}

	if tc.cfg.ClusterTopology != "" {
		tc.topology = tc.cfg.ClusterTopology
		return tc.topology
	}

	if tc.cfg.ClusterID != "" {
		topo, err := DetectClusterTopology(tc.conn, tc.cfg.ClusterID)
		if err == nil {
			tc.topology = topo
			return tc.topology
		}
	}

	return ""
}

// IsHCP returns true if the cluster topology is ROSA HCP.
func (tc *TestContext) IsHCP() bool {
	return tc.Topology() == "hcp"
}

// IsClassic returns true if the cluster topology is ROSA Classic STS.
func (tc *TestContext) IsClassic() bool {
	return tc.Topology() == "classic"
}

// IsOSDGCP returns true if the cluster topology is OSD on GCP.
func (tc *TestContext) IsOSDGCP() bool {
	return tc.Topology() == "osd-gcp"
}

// HasSCAccess returns true if the service cluster ID is configured.
func (tc *TestContext) HasSCAccess() bool {
	return tc.cfg.ServiceClusterID != ""
}

// HasMCAccess returns true if the management cluster ID is configured.
func (tc *TestContext) HasMCAccess() bool {
	return tc.cfg.ManagementClusterID != ""
}

// HasAWSAccess returns true if AWS clients were successfully initialized.
func (tc *TestContext) HasAWSAccess() bool {
	return tc.ec2Client != nil
}

// resolveKubeconfig loads a rest.Config from a kubeconfig file env var, or falls back to OCM credentials.
func resolveKubeconfig(envVar string, conn *sdk.Connection, clusterID string) (*rest.Config, error) {
	if kubeconfigPath := os.Getenv(envVar); kubeconfigPath != "" {
		cfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("loading kubeconfig from %s=%s: %w", envVar, kubeconfigPath, err)
		}
		return cfg, nil
	}
	return GetClusterCredentials(conn, clusterID)
}
