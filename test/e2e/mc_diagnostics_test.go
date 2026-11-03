//go:build E2Etests

package e2e

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/onsi/ginkgo/v2"
	"github.com/openshift-online/rosa-e2e/pkg/reporting"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

func TestMCFailureDiagnosticsDoesNotChangeVerdict(t *testing.T) {
	for _, test := range []struct {
		name   string
		failed bool
		err    error
	}{
		{name: "passing"},
		{name: "failing", failed: true},
		{name: "diagnostics error", failed: true, err: errors.New("private error must not be logged")},
	} {
		t.Run(test.name, func(t *testing.T) {
			var warning bytes.Buffer
			ginkgo.GinkgoWriter.TeeTo(&warning)
			defer ginkgo.GinkgoWriter.ClearTeeWriters()
			called := false
			collectMCFailureDiagnostics(test.failed, func() error { called = true; return test.err })
			if called != test.failed {
				t.Fatalf("collector called=%v for failed=%v", called, test.failed)
			}
			if strings.Contains(warning.String(), "private error") {
				t.Fatal("private error leaked to warning output")
			}
			if test.err != nil && !strings.Contains(warning.String(), "WARNING") {
				t.Fatal("missing best-effort warning")
			}
		})
	}
}

func TestMCFailureDiagnosticsCleanupExecution(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "pass", true: "fail"}[failed], func(t *testing.T) {
			directory := t.TempDir()
			t.Setenv("ARTIFACT_DIR", directory)
			var cleanup func()
			verdict := false
			deferMCFailureDiagnostics(func(callback func()) { cleanup = callback }, func() bool { return verdict }, func() error {
				return reporting.WriteMCFailureDiagnostics(context.Background(), fake.NewClientset(), dynamicfake.NewSimpleDynamicClient(runtime.NewScheme()), reporting.MCNamespaces{HCPNamespace: "hcp", HCNamespace: "clusters", ClusterName: "example"})
			})
			if cleanup == nil {
				t.Fatal("failure cleanup was not registered")
			}
			verdict = failed
			cleanup()
			paths, err := filepath.Glob(filepath.Join(directory, "mc-failure-diagnostics", "*.json"))
			if err != nil || (len(paths) == 1) != failed || verdict != failed {
				t.Fatalf("cleanup artifact=%v verdict=%v err=%v", paths, verdict, err)
			}
			if !failed {
				if _, err := os.Stat(filepath.Join(directory, "mc-failure-diagnostics")); !os.IsNotExist(err) {
					t.Fatal("passing spec performed capture")
				}
			}
		})
	}
}
