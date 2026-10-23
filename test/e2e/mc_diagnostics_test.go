//go:build E2Etests

package e2e

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/onsi/ginkgo/v2"
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
