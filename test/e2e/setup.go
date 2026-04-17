//go:build E2Etests

package e2e

import (
	g "github.com/onsi/ginkgo/v2"
	sdk "github.com/openshift-online/ocm-sdk-go"

	"github.com/openshift-online/rosa-e2e/pkg/config"
	"github.com/openshift-online/rosa-e2e/pkg/framework"
)

var (
	cfg  *config.Config
	conn *sdk.Connection
)

var _ = g.BeforeSuite(func() {
	var err error
	cfg, err = config.Load()
	if err != nil {
		panic("failed to load config: " + err.Error())
	}

	conn, err = framework.NewOCMConnection(cfg)
	if err != nil {
		panic("failed to create OCM connection: " + err.Error())
	}

	g.GinkgoWriter.Printf("Connected to OCM at %s (env: %s)\n", cfg.OCMBaseURL(), cfg.OCMEnv)
})

var _ = g.AfterSuite(func() {
	if conn != nil {
		_ = conn.Close()
	}
})
