package framework

import (
	"os"
	"testing"
)

func TestResolveKubeconfigUsesEnvironmentFile(t *testing.T) {
	kubeconfig := []byte(`apiVersion: v1
kind: Config
clusters:
- cluster:
    server: https://backplane.example.test
  name: hosted-cluster
contexts:
- context:
    cluster: hosted-cluster
    user: backplane
  name: hosted-cluster
current-context: hosted-cluster
users:
- name: backplane
  user:
    token: test-token
`)

	path := t.TempDir() + "/kubeconfig"
	if err := os.WriteFile(path, kubeconfig, 0o600); err != nil {
		t.Fatalf("writing kubeconfig: %v", err)
	}
	t.Setenv("KUBECONFIG", path)

	cfg, err := resolveKubeconfig("KUBECONFIG", nil, "unused-cluster-id")
	if err != nil {
		t.Fatalf("resolveKubeconfig() returned an error: %v", err)
	}
	if cfg.Host != "https://backplane.example.test" {
		t.Fatalf("Host = %q, want %q", cfg.Host, "https://backplane.example.test")
	}
	if cfg.BearerToken != "test-token" {
		t.Fatalf("BearerToken = %q, want %q", cfg.BearerToken, "test-token")
	}
}
