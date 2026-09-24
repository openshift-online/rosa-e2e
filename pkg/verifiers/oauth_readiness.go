package verifiers

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	oauthAPIServerNamespace = "openshift-oauth-apiserver"
	oauthAPIServerLabel     = "app=openshift-oauth-apiserver"
	oauthWellKnownPath      = "/.well-known/oauth-authorization-server"
)

// VerifyOAuthServerReadiness waits for the oauth-apiserver pods to be Ready
// and for the OAuth well-known endpoint to return HTTP 200. This catches
// the gap where the authentication ClusterOperator reports Available=True
// but the underlying oauth-apiserver pods are still restarting after an
// etcd startup race condition.
//
// The function polls with a fixed interval until both conditions are met
// or the provided timeout expires.
func VerifyOAuthServerReadiness(ctx context.Context, kubeClient kubernetes.Interface, apiServerURL string, timeout time.Duration) error {
	if err := waitForOAuthPodReadiness(ctx, kubeClient, timeout); err != nil {
		return fmt.Errorf("oauth-apiserver pod readiness: %w", err)
	}

	if err := waitForOAuthWellKnownEndpoint(ctx, apiServerURL, timeout); err != nil {
		return fmt.Errorf("oauth well-known endpoint: %w", err)
	}

	return nil
}

// waitForOAuthPodReadiness polls the openshift-oauth-apiserver namespace
// for pods with label app=openshift-oauth-apiserver and waits until all
// pods have all containers Ready. Returns an error if no pods are found
// or if the timeout expires before all pods are ready.
func waitForOAuthPodReadiness(ctx context.Context, kubeClient kubernetes.Interface, timeout time.Duration) error {
	const pollInterval = 15 * time.Second

	deadline := time.Now().Add(timeout)

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s waiting for oauth-apiserver pods to be ready", timeout)
		}

		pods, err := kubeClient.CoreV1().Pods(oauthAPIServerNamespace).List(ctx, metav1.ListOptions{
			LabelSelector: oauthAPIServerLabel,
		})
		if err != nil {
			return fmt.Errorf("listing oauth-apiserver pods: %w", err)
		}

		if len(pods.Items) == 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(pollInterval):
				continue
			}
		}

		allReady := true
		for _, pod := range pods.Items {
			if !isPodContainersReady(pod.Status.ContainerStatuses) {
				allReady = false
				break
			}
		}

		if allReady {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

// isPodContainersReady returns true if all containers in the pod have Ready=true.
func isPodContainersReady(statuses []corev1.ContainerStatus) bool {
	if len(statuses) == 0 {
		return false
	}
	for _, cs := range statuses {
		if !cs.Ready {
			return false
		}
	}
	return true
}

// waitForOAuthWellKnownEndpoint probes the /.well-known/oauth-authorization-server
// endpoint on the API server and waits for an HTTP 200 response. If the endpoint
// returns 500 (common during etcd race conditions), the function retries until
// the timeout expires.
func waitForOAuthWellKnownEndpoint(ctx context.Context, apiServerURL string, timeout time.Duration) error {
	const pollInterval = 10 * time.Second

	endpoint := strings.TrimRight(apiServerURL, "/") + oauthWellKnownPath
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	deadline := time.Now().Add(timeout)

	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out after %s waiting for OAuth well-known endpoint at %s", timeout, endpoint)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return fmt.Errorf("creating request for %s: %w", endpoint, err)
		}

		resp, reqErr := client.Do(req)
		if reqErr == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}
