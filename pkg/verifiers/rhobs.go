package verifiers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/openshift-online/rosa-e2e/pkg/config"
)

// RHOBSProbe represents a probe in the RHOBS API response.
type RHOBSProbe struct {
	ID        string                 `json:"id"`
	StaticURL string                 `json:"static_url"`
	Labels    map[string]interface{} `json:"labels"`
	Status    string                 `json:"status"`
}

// RHOBSConfig holds RHOBS API access configuration.
type RHOBSConfig struct {
	ProbeAPIURL      string
	MetricsAPIURL    string
	OIDCClientID     string
	OIDCClientSecret string
	OIDCIssuerURL    string
}

// NewRHOBSConfig creates RHOBS config from test suite config.
func NewRHOBSConfig(cfg *config.Config) *RHOBSConfig {
	return &RHOBSConfig{
		ProbeAPIURL:      cfg.RHOBSProbeAPIURL,
		MetricsAPIURL:    cfg.RHOBSMetricsAPIURL,
		OIDCClientID:     cfg.RHOBSOIDCClientID,
		OIDCClientSecret: cfg.RHOBSOIDCClientSecret,
		OIDCIssuerURL:    cfg.RHOBSOIDCIssuerURL,
	}
}

// IsConfigured returns true if RHOBS API access is configured.
func (rc *RHOBSConfig) IsConfigured() bool {
	return rc.ProbeAPIURL != "" && rc.OIDCClientID != "" && rc.OIDCClientSecret != ""
}

// getOIDCAccessToken fetches an OIDC access token using client credentials flow.
func getOIDCAccessToken(cfg *RHOBSConfig) (string, error) {
	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", cfg.OIDCClientID)
	data.Set("client_secret", cfg.OIDCClientSecret)

	req, err := http.NewRequest("POST", cfg.OIDCIssuerURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to get access token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("OIDC token endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode token response: %w", err)
	}

	return tokenResp.AccessToken, nil
}

// queryRHOBSProbes queries the RHOBS API for probes matching a label selector.
func queryRHOBSProbes(cfg *RHOBSConfig, labelSelector string) ([]RHOBSProbe, error) {
	// Get OIDC access token
	accessToken, err := getOIDCAccessToken(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to get OIDC access token: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}

	reqURL := cfg.ProbeAPIURL
	if labelSelector != "" {
		reqURL += "?label_selector=" + url.QueryEscape(labelSelector)
	}

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query RHOBS API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("RHOBS API returned status %d: %s", resp.StatusCode, string(body))
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Try to decode as array
	var probes []RHOBSProbe
	if err := json.Unmarshal(bodyBytes, &probes); err != nil {
		// Try wrapped response format
		var wrapper struct {
			Probes []RHOBSProbe `json:"probes"`
		}
		if err2 := json.Unmarshal(bodyBytes, &wrapper); err2 == nil {
			return wrapper.Probes, nil
		}
		return nil, fmt.Errorf("failed to decode RHOBS API response: %w", err)
	}

	return probes, nil
}

// VerifyRHOBSProbeExists verifies that a probe exists for the cluster with the correct private label.
func VerifyRHOBSProbeExists(ctx context.Context, clusterID string, expectedPrivate bool, cfg *RHOBSConfig) error {
	probes, err := queryRHOBSProbes(cfg, fmt.Sprintf("cluster-id=%s", clusterID))
	if err != nil {
		return fmt.Errorf("querying RHOBS API for cluster %s: %w", clusterID, err)
	}

	if len(probes) == 0 {
		return fmt.Errorf("no probe found for cluster %s", clusterID)
	}

	if len(probes) > 1 {
		return fmt.Errorf("expected 1 probe for cluster %s, found %d", clusterID, len(probes))
	}

	probe := probes[0]

	// Verify cluster-id label
	clusterIDLabel, ok := probe.Labels["cluster-id"].(string)
	if !ok || clusterIDLabel != clusterID {
		return fmt.Errorf("probe has incorrect cluster-id label: got %v, want %s", probe.Labels["cluster-id"], clusterID)
	}

	// Verify private label
	privateLabel, ok := probe.Labels["private"].(string)
	if !ok {
		return fmt.Errorf("probe missing private label")
	}

	expectedPrivateStr := "false"
	if expectedPrivate {
		expectedPrivateStr = "true"
	}

	if privateLabel != expectedPrivateStr {
		return fmt.Errorf("probe has incorrect private label: got %s, want %s", privateLabel, expectedPrivateStr)
	}

	return nil
}

// VerifyProbeLabels verifies that the probe has the expected labels.
func VerifyProbeLabels(ctx context.Context, clusterID string, expectedLabels map[string]string, cfg *RHOBSConfig) error {
	probes, err := queryRHOBSProbes(cfg, fmt.Sprintf("cluster-id=%s", clusterID))
	if err != nil {
		return fmt.Errorf("querying RHOBS API: %w", err)
	}

	if len(probes) == 0 {
		return fmt.Errorf("no probe found for cluster %s", clusterID)
	}

	probe := probes[0]

	for key, expectedValue := range expectedLabels {
		actualValue, ok := probe.Labels[key].(string)
		if !ok {
			return fmt.Errorf("probe missing label %s", key)
		}
		if actualValue != expectedValue {
			return fmt.Errorf("probe label %s: got %s, want %s", key, actualValue, expectedValue)
		}
	}

	return nil
}

// queryRHOBSMetrics queries the RHOBS metrics API for a PromQL query.
func queryRHOBSMetrics(cfg *RHOBSConfig, query string) (map[string]interface{}, error) {
	accessToken, err := getOIDCAccessToken(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to get OIDC access token: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}

	reqURL := cfg.MetricsAPIURL + "/api/v1/query?query=" + url.QueryEscape(query)

	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query RHOBS metrics API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("RHOBS metrics API returned status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode metrics response: %w", err)
	}

	return result, nil
}

// VerifyProbeSuccessMetrics verifies that probe_success metrics exist for the cluster.
func VerifyProbeSuccessMetrics(ctx context.Context, clusterID string, cfg *RHOBSConfig) error {
	query := fmt.Sprintf(`probe_success{_id="%s"}`, clusterID)
	result, err := queryRHOBSMetrics(cfg, query)
	if err != nil {
		return fmt.Errorf("querying probe_success metrics: %w", err)
	}

	// Check if we got data back
	status, ok := result["status"].(string)
	if !ok || status != "success" {
		return fmt.Errorf("metrics query failed: %v", result)
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("no data in metrics response")
	}

	resultData, ok := data["result"].([]interface{})
	if !ok || len(resultData) == 0 {
		return fmt.Errorf("no probe_success metrics found for cluster %s", clusterID)
	}

	return nil
}

// VerifyRecordingRules verifies that RHOBS recording rules are evaluating for the cluster.
func VerifyRecordingRules(ctx context.Context, clusterID string, cfg *RHOBSConfig) error {
	// Check for sre:hcp:probe_active recording rule
	probeActiveQuery := fmt.Sprintf(`sre:hcp:probe_active{_id="%s"}`, clusterID)
	result, err := queryRHOBSMetrics(cfg, probeActiveQuery)
	if err != nil {
		return fmt.Errorf("querying sre:hcp:probe_active: %w", err)
	}

	status, ok := result["status"].(string)
	if !ok || status != "success" {
		return fmt.Errorf("sre:hcp:probe_active query failed: %v", result)
	}

	data, ok := result["data"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("no data in sre:hcp:probe_active response")
	}

	resultData, ok := data["result"].([]interface{})
	if !ok || len(resultData) == 0 {
		return fmt.Errorf("sre:hcp:probe_active recording rule not evaluating for cluster %s", clusterID)
	}

	// Check for sre:hcp:blackbox_probe_active recording rule
	blackboxQuery := fmt.Sprintf(`sre:hcp:blackbox_probe_active{_id="%s"}`, clusterID)
	result, err = queryRHOBSMetrics(cfg, blackboxQuery)
	if err != nil {
		return fmt.Errorf("querying sre:hcp:blackbox_probe_active: %w", err)
	}

	status, ok = result["status"].(string)
	if !ok || status != "success" {
		return fmt.Errorf("sre:hcp:blackbox_probe_active query failed: %v", result)
	}

	data, ok = result["data"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("no data in sre:hcp:blackbox_probe_active response")
	}

	resultData, ok = data["result"].([]interface{})
	if !ok || len(resultData) == 0 {
		return fmt.Errorf("sre:hcp:blackbox_probe_active recording rule not evaluating for cluster %s", clusterID)
	}

	return nil
}
