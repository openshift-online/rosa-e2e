package framework

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/openshift-online/rosa-e2e/pkg/config"
)

const backplaneLoginTimeout = 2 * time.Minute

var backplaneConfigs = struct {
	sync.Mutex
	items map[string]*rest.Config
}{
	items: make(map[string]*rest.Config),
}

// GetBackplaneClusterConfig creates an isolated backplane session for a cluster.
// The parsed configuration is cached in memory; no user kubeconfig is modified.
func GetBackplaneClusterConfig(ctx context.Context, cfg *config.Config) (*rest.Config, error) {
	cacheKey := cfg.OCMBaseURL() + "/" + cfg.ClusterID

	backplaneConfigs.Lock()
	defer backplaneConfigs.Unlock()
	if cached := backplaneConfigs.items[cacheKey]; cached != nil {
		return rest.CopyConfig(cached), nil
	}

	if cfg.OCMToken == "" {
		return nil, fmt.Errorf("OCM_TOKEN is required for backplane access")
	}
	if _, err := exec.LookPath("ocm"); err != nil {
		return nil, fmt.Errorf("ocm CLI with the backplane plugin is required: %w", err)
	}

	tempDir, err := os.MkdirTemp("", "rosa-e2e-backplane-")
	if err != nil {
		return nil, fmt.Errorf("creating temporary backplane directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	commandCtx, cancel := context.WithTimeout(ctx, backplaneLoginTimeout)
	defer cancel()

	ocmConfigPath := filepath.Join(tempDir, "ocm.json")
	baseKubeconfigPath := filepath.Join(tempDir, "base-kubeconfig")
	commandEnv := append(os.Environ(),
		"OCM_CONFIG="+ocmConfigPath,
		"KUBECONFIG="+baseKubeconfigPath,
	)

	login := exec.CommandContext(commandCtx, "ocm", "login",
		"--url", cfg.OCMBaseURL(),
		"--token", cfg.OCMToken,
	)
	login.Env = commandEnv
	if output, err := login.CombinedOutput(); err != nil {
		return nil, commandError("logging into OCM for backplane access", err, output)
	}

	args := []string{
		"backplane", "login", cfg.ClusterID,
		"--multi",
		"--kube-path", tempDir,
	}
	if proxyURL := resolveBackplaneProxyURL(); proxyURL != "" {
		args = append(args, "--proxy", proxyURL)
	}

	backplaneLogin := exec.CommandContext(commandCtx, "ocm", args...)
	backplaneLogin.Env = commandEnv
	if output, err := backplaneLogin.CombinedOutput(); err != nil {
		return nil, commandError("creating backplane session", err, output)
	}

	kubeconfigPath := filepath.Join(tempDir, cfg.ClusterID, "config")
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return nil, fmt.Errorf("loading backplane kubeconfig: %w", err)
	}

	backplaneConfigs.items[cacheKey] = rest.CopyConfig(restConfig)
	return restConfig, nil
}

func resolveBackplaneProxyURL() string {
	for _, envVar := range []string{"BACKPLANE_PROXY_URL", "HTTPS_PROXY", "https_proxy"} {
		if value := os.Getenv(envVar); value != "" {
			return value
		}
	}

	rawConfig, err := clientcmd.NewDefaultClientConfigLoadingRules().Load()
	if err != nil {
		return ""
	}
	currentContext := rawConfig.Contexts[rawConfig.CurrentContext]
	if currentContext == nil {
		return ""
	}
	cluster := rawConfig.Clusters[currentContext.Cluster]
	if cluster == nil {
		return ""
	}
	return cluster.ProxyURL
}

func commandError(action string, err error, output []byte) error {
	detail := strings.TrimSpace(string(output))
	if detail == "" {
		return fmt.Errorf("%s: %w", action, err)
	}
	return fmt.Errorf("%s: %w: %s", action, err, detail)
}
