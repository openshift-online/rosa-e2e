package verifiers

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	authnv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
)

const (
	monitoringNamespace = "openshift-monitoring"
	// prometheusK8sSA already carries the RBAC needed to query the Thanos Querier,
	// so we mint a short-lived token for it rather than creating our own SA/binding.
	prometheusK8sSA = "prometheus-k8s"
	thanosRouteName = "thanos-querier"
)

// alertsResponse models the subset of the Prometheus/Thanos HTTP API
// (/api/v1/alerts) response that we care about.
type alertsResponse struct {
	Status string `json:"status"`
	Data   struct {
		Alerts []prometheusAlert `json:"alerts"`
	} `json:"data"`
	ErrorType string `json:"errorType"`
	Error     string `json:"error"`
}

type prometheusAlert struct {
	Labels map[string]string `json:"labels"`
	State  string            `json:"state"`
}

// defaultExcludedAlerts are alerts that are expected to be present on a healthy
// cluster and should not be treated as failures. Watchdog always fires by design
// (it proves the alerting pipeline is working).
var defaultExcludedAlerts = map[string]bool{
	"Watchdog": true,
}

// VerifyNoCriticalAlerts fails if any critical-severity alert is firing in the
// cluster's in-cluster monitoring stack. It is intended to run after cluster
// creation to catch pre-GA alert bugs.
//
// It queries the Thanos Querier via its public route using a short-lived token
// minted for the prometheus-k8s service account (the API-server service proxy
// cannot be used because the kube-rbac-proxy in front of Thanos rejects
// proxied requests that carry no bearer token).
//
// Alerts named in excludeAlerts (in addition to the built-in allowlist) are ignored.
func VerifyNoCriticalAlerts(ctx context.Context, kubeClient kubernetes.Interface, dynamicClient dynamic.Interface, excludeAlerts ...string) error {
	excluded := make(map[string]bool, len(defaultExcludedAlerts)+len(excludeAlerts))
	for name := range defaultExcludedAlerts {
		excluded[name] = true
	}
	for _, name := range excludeAlerts {
		excluded[strings.TrimSpace(name)] = true
	}

	host, err := thanosQuerierHost(ctx, dynamicClient)
	if err != nil {
		return err
	}

	token, err := mintToken(ctx, kubeClient, monitoringNamespace, prometheusK8sSA)
	if err != nil {
		return err
	}

	alerts, err := fetchAlerts(ctx, host, token)
	if err != nil {
		return err
	}

	var firing []string
	for _, alert := range alerts {
		if alert.State != "firing" {
			continue
		}
		if alert.Labels["severity"] != "critical" {
			continue
		}
		name := alert.Labels["alertname"]
		if excluded[name] {
			continue
		}
		detail := name
		if ns := alert.Labels["namespace"]; ns != "" {
			detail = fmt.Sprintf("%s (namespace=%s)", name, ns)
		}
		firing = append(firing, detail)
	}

	if len(firing) > 0 {
		sort.Strings(firing)
		return fmt.Errorf("critical alerts firing:\n  %s", strings.Join(firing, "\n  "))
	}

	return nil
}

// thanosQuerierHost returns the external host of the thanos-querier route.
func thanosQuerierHost(ctx context.Context, dynamicClient dynamic.Interface) (string, error) {
	gvr := schema.GroupVersionResource{Group: "route.openshift.io", Version: "v1", Resource: "routes"}
	route, err := dynamicClient.Resource(gvr).Namespace(monitoringNamespace).Get(ctx, thanosRouteName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("getting %s route: %w", thanosRouteName, err)
	}
	host, found, err := unstructured.NestedString(route.Object, "spec", "host")
	if err != nil || !found || host == "" {
		return "", fmt.Errorf("%s route has no spec.host", thanosRouteName)
	}
	return host, nil
}

// mintToken requests a short-lived bearer token for the given service account.
func mintToken(ctx context.Context, kubeClient kubernetes.Interface, namespace, saName string) (string, error) {
	expiration := int64(600)
	tr, err := kubeClient.CoreV1().ServiceAccounts(namespace).CreateToken(ctx, saName, &authnv1.TokenRequest{
		Spec: authnv1.TokenRequestSpec{ExpirationSeconds: &expiration},
	}, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("minting token for %s/%s: %w", namespace, saName, err)
	}
	return tr.Status.Token, nil
}

// fetchAlerts queries the Thanos Querier alerts endpoint over its route.
func fetchAlerts(ctx context.Context, host, token string) ([]prometheusAlert, error) {
	reqURL := fmt.Sprintf("https://%s/api/v1/alerts", host)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building alerts request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	// The route serves a cert from the cluster's own ingress CA, which is not in
	// the system trust store; skip verification for this in-cluster health check.
	client := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // in-cluster route, self-signed CA
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("querying thanos-querier alerts: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("thanos-querier alerts returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed alertsResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parsing alerts response: %w", err)
	}
	if parsed.Status != "success" {
		return nil, fmt.Errorf("alerts query returned status %q: %s: %s", parsed.Status, parsed.ErrorType, parsed.Error)
	}
	return parsed.Data.Alerts, nil
}
