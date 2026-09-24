package verifiers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	sdk "github.com/openshift-online/ocm-sdk-go"
	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
)

// externalAuthAPITimeout bounds the OCM API calls so an unavailable API cannot block a spec
// indefinitely when the caller's context has no deadline of its own.
const externalAuthAPITimeout = 30 * time.Second

// ExternalAuthEnabled reports whether the cluster has external authentication (external auth
// provider / bring-your-own identity provider) enabled via its external_auth_config.
//
// External authentication is a ROSA HCP-only feature (ROSAENG-63355).
func ExternalAuthEnabled(ctx context.Context, conn *sdk.Connection, clusterID string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, externalAuthAPITimeout)
	defer cancel()

	resp, err := conn.ClustersMgmt().V1().Clusters().Cluster(clusterID).
		ExternalAuthConfig().Get().SendContext(ctx)
	if err != nil {
		return false, fmt.Errorf("getting external auth config for cluster %s: %w", clusterID, err)
	}
	return resp.Body().Enabled(), nil
}

// VerifyExternalAuthProviders validates that the cluster has external authentication enabled and
// at least one external auth provider configured, and that each provider is well-formed: it must
// declare an https issuer URL, at least one audience, and must not report a failed state.
//
// This covers the "cluster with external-auth-providers-enabled" and "external auth provider
// configured and functional" acceptance criteria of ROSAENG-63355 from the OCM API's perspective.
// Verifying an end-to-end user login with a real token minted by the identity provider requires
// IdP credentials and is out of scope for this API-driven suite.
func VerifyExternalAuthProviders(ctx context.Context, conn *sdk.Connection, clusterID string) error {
	ctx, cancel := context.WithTimeout(ctx, externalAuthAPITimeout)
	defer cancel()

	cfgResp, err := conn.ClustersMgmt().V1().Clusters().Cluster(clusterID).
		ExternalAuthConfig().Get().SendContext(ctx)
	if err != nil {
		return fmt.Errorf("getting external auth config for cluster %s: %w", clusterID, err)
	}
	if !cfgResp.Body().Enabled() {
		return fmt.Errorf("external authentication is not enabled on cluster %s", clusterID)
	}

	listResp, err := conn.ClustersMgmt().V1().Clusters().Cluster(clusterID).
		ExternalAuthConfig().ExternalAuths().List().SendContext(ctx)
	if err != nil {
		return fmt.Errorf("listing external auth providers for cluster %s: %w", clusterID, err)
	}

	providers := listResp.Items().Slice()
	if len(providers) == 0 {
		return fmt.Errorf("external auth is enabled on cluster %s but no providers are configured", clusterID)
	}

	for _, p := range providers {
		if err := validateExternalAuthProvider(p); err != nil {
			return fmt.Errorf("external auth provider %q: %w", p.ID(), err)
		}
	}
	return nil
}

// validateExternalAuthProvider checks a single external auth provider is well-formed.
func validateExternalAuthProvider(p *cmv1.ExternalAuth) error {
	issuer := p.Issuer()
	if issuer == nil {
		return fmt.Errorf("no issuer configured")
	}
	if !strings.HasPrefix(issuer.URL(), "https://") {
		return fmt.Errorf("issuer URL %q is not a valid https URL", issuer.URL())
	}
	if len(issuer.Audiences()) == 0 {
		return fmt.Errorf("issuer has no audiences configured")
	}
	// When the service reports a state, treat only explicit failures as invalid; unknown or
	// transitional states are tolerated to avoid false negatives across API versions.
	if status := p.Status(); status != nil {
		if state := status.State(); state != nil {
			switch strings.ToLower(state.Value()) {
			case "error", "failed":
				return fmt.Errorf("provider is in state %q: %s", state.Value(), status.Message())
			}
		}
	}
	return nil
}

// VerifyExternalAuthIssuerReachable actively probes each configured provider's OIDC issuer: it
// fetches the issuer's discovery document and the JWKS it advertises, and requires the JWKS to
// contain at least one signing key. This is a live health check of the issuer the cluster is
// actually configured with — beyond the static config validation in VerifyExternalAuthProviders —
// and needs no IdP credentials, so it can run against any external-auth-enabled cluster.
//
// Note: reachability is checked from the test runner, which is not necessarily the same network
// path the cluster's apiserver uses to reach the issuer.
func VerifyExternalAuthIssuerReachable(ctx context.Context, conn *sdk.Connection, clusterID string) error {
	ctx, cancel := context.WithTimeout(ctx, externalAuthAPITimeout)
	defer cancel()

	listResp, err := conn.ClustersMgmt().V1().Clusters().Cluster(clusterID).
		ExternalAuthConfig().ExternalAuths().List().SendContext(ctx)
	if err != nil {
		return fmt.Errorf("listing external auth providers for cluster %s: %w", clusterID, err)
	}

	providers := listResp.Items().Slice()
	if len(providers) == 0 {
		return fmt.Errorf("no external auth providers configured on cluster %s", clusterID)
	}

	httpClient := &http.Client{Timeout: externalAuthAPITimeout}
	for _, p := range providers {
		issuer := p.Issuer()
		if issuer == nil || issuer.URL() == "" {
			return fmt.Errorf("external auth provider %q has no issuer URL", p.ID())
		}
		if err := probeOIDCIssuer(ctx, httpClient, issuer.URL()); err != nil {
			return fmt.Errorf("external auth provider %q issuer %q: %w", p.ID(), issuer.URL(), err)
		}
	}
	return nil
}

// probeOIDCIssuer fetches an issuer's OIDC discovery document and its advertised JWKS, requiring at
// least one signing key to be published.
func probeOIDCIssuer(ctx context.Context, client *http.Client, issuer string) error {
	discoveryURL := strings.TrimRight(issuer, "/") + "/.well-known/openid-configuration"
	var doc struct {
		JWKSURI string `json:"jwks_uri"`
	}
	if err := getJSON(ctx, client, discoveryURL, &doc); err != nil {
		return fmt.Errorf("fetching OIDC discovery document: %w", err)
	}
	if doc.JWKSURI == "" {
		return fmt.Errorf("OIDC discovery document has no jwks_uri")
	}

	var jwks struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if err := getJSON(ctx, client, doc.JWKSURI, &jwks); err != nil {
		return fmt.Errorf("fetching JWKS: %w", err)
	}
	if len(jwks.Keys) == 0 {
		return fmt.Errorf("JWKS at %s contains no signing keys", doc.JWKSURI)
	}
	return nil
}

// getJSON performs a GET and decodes a JSON response, returning an error on any non-200 status.
func getJSON(ctx context.Context, client *http.Client, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned %s: %s", url, resp.Status, strings.TrimSpace(string(body)))
	}
	return json.Unmarshal(body, out)
}
