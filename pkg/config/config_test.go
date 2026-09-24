package config

import (
	"strings"
	"testing"
)

const commercialTokenURL = "https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/token"

func TestLoadAcceptsOCMClientCredentials(t *testing.T) {
	t.Setenv("OCM_TOKEN", "")
	t.Setenv("OCM_CLIENT_ID", "client-id")
	t.Setenv("OCM_CLIENT_SECRET", "client-secret")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned an error: %v", err)
	}
	if cfg.OCMClientID != "client-id" {
		t.Errorf("OCMClientID = %q, want %q", cfg.OCMClientID, "client-id")
	}
	if cfg.OCMClientSecret != "client-secret" {
		t.Errorf("OCMClientSecret = %q, want %q", cfg.OCMClientSecret, "client-secret")
	}
}

func TestLoadRejectsIncompleteOCMClientCredentials(t *testing.T) {
	t.Setenv("OCM_TOKEN", "")
	t.Setenv("OCM_CLIENT_ID", "client-id")
	t.Setenv("OCM_CLIENT_SECRET", "")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() succeeded with incomplete OCM client credentials")
	}
	if !strings.Contains(err.Error(), "OCM_CLIENT_ID and OCM_CLIENT_SECRET") {
		t.Fatalf("Load() error = %q, want client credential guidance", err)
	}
}

func TestLoadTokenURLDefault(t *testing.T) {
	t.Setenv("CLUSTER_CONFIG", "")
	t.Setenv("OCM_TOKEN", "dummy-token") // Load() requires a token
	t.Setenv("OCM_TOKEN_URL", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.OCMTokenURL != commercialTokenURL {
		t.Errorf("OCMTokenURL = %q, want commercial default %q", cfg.OCMTokenURL, commercialTokenURL)
	}
}

func TestLoadTokenURLOverride(t *testing.T) {
	t.Setenv("CLUSTER_CONFIG", "")
	t.Setenv("OCM_TOKEN", "dummy-token") // Load() requires a token
	t.Setenv("OCM_TOKEN_URL", "https://example.test/oauth2/token")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.OCMTokenURL != "https://example.test/oauth2/token" {
		t.Errorf("OCMTokenURL = %q, want env override", cfg.OCMTokenURL)
	}
}
