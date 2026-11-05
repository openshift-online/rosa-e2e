//go:build E2Etests

package e2e

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDNSFailureDiagnosticsBeforeNamespaceCleanup(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("ARTIFACT_DIR", directory)
	client := fake.NewClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "dns"}})
	cleanup := []func(){func() {
		if err := client.CoreV1().Namespaces().Delete(context.Background(), "dns", metav1.DeleteOptions{}); err != nil {
			t.Error(err)
		}
	}}
	failed := false
	deferDNSFailureDiagnostics(func(callback func()) { cleanup = append(cleanup, callback) }, func() bool { return failed }, client, "dns")
	failed = true
	for index := len(cleanup) - 1; index >= 0; index-- {
		cleanup[index]()
	}
	paths, err := filepath.Glob(filepath.Join(directory, "dns-failure-diagnostics", "*.json"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("paths=%v err=%v", paths, err)
	}
	content, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		ReadStatus map[string]string `json:"readStatus"`
	}
	if err := json.Unmarshal(content, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.ReadStatus["namespace"] != "complete" || !failed {
		t.Fatalf("capture ran after deletion or changed verdict: %v", snapshot.ReadStatus)
	}
}

func TestDNSFailureDiagnosticsOnlyOnFailure(t *testing.T) {
	for _, test := range []struct {
		name   string
		failed bool
	}{{name: "passing"}, {name: "failing", failed: true}} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			t.Setenv("ARTIFACT_DIR", directory)
			client := fake.NewClientset()
			collectDNSFailureDiagnostics(test.failed, client, "dns")
			paths, err := filepath.Glob(filepath.Join(directory, "dns-failure-diagnostics", "*.json"))
			if err != nil || (len(paths) == 1) != test.failed {
				t.Fatalf("paths=%v err=%v", paths, err)
			}
			if !test.failed {
				if len(client.Actions()) != 0 {
					t.Fatal("passing test performed diagnostic reads")
				}
				if _, err := os.Stat(filepath.Join(directory, "dns-failure-diagnostics")); !os.IsNotExist(err) {
					t.Fatal("passing test wrote artifacts")
				}
			}
		})
	}
}
