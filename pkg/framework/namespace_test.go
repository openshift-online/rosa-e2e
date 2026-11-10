package framework

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

var serviceAccountsResource = schema.GroupResource{Resource: "serviceaccounts"}

func TestWaitForDefaultServiceAccount(t *testing.T) {
	t.Parallel()
	unexpected := errors.New("connection refused")
	for _, test := range []struct {
		name      string
		notFound  int
		apiErr    error
		cancel    bool
		timeout   time.Duration
		wantCalls int
		check     func(error) bool
		wantText  string
	}{
		{name: "present", wantCalls: 1},
		{name: "created after not found", notFound: 2, wantCalls: 3},
		{name: "timeout while not found", notFound: -1, timeout: 200 * time.Millisecond, check: func(err error) bool { return errors.Is(err, context.DeadlineExceeded) }, wantText: "still not found"},
		{name: "canceled while not found", notFound: -1, cancel: true, wantCalls: 1, check: func(err error) bool { return errors.Is(err, context.Canceled) }, wantText: "still not found"},
		{name: "deadline during request", apiErr: &url.Error{Op: "Get", URL: "https://api.example.test", Err: context.DeadlineExceeded}, wantCalls: 1, check: func(err error) bool { return errors.Is(err, context.DeadlineExceeded) }, wantText: "waiting for default ServiceAccount in namespace dns (timeout"},
		{name: "forbidden", apiErr: apierrors.NewForbidden(serviceAccountsResource, "default", errors.New("denied")), wantCalls: 1, check: apierrors.IsForbidden, wantText: "getting default ServiceAccount"},
		{name: "forbidden after not found", notFound: 1, apiErr: apierrors.NewForbidden(serviceAccountsResource, "default", errors.New("denied")), wantCalls: 2, check: apierrors.IsForbidden, wantText: "getting default ServiceAccount"},
		{name: "unauthorized", apiErr: apierrors.NewUnauthorized("denied"), wantCalls: 1, check: apierrors.IsUnauthorized, wantText: "getting default ServiceAccount"},
		{name: "internal error", apiErr: apierrors.NewInternalError(errors.New("boom")), wantCalls: 1, check: apierrors.IsInternalError, wantText: "getting default ServiceAccount"},
		{name: "server timeout", apiErr: apierrors.NewServerTimeout(serviceAccountsResource, "get", 1), wantCalls: 1, check: apierrors.IsServerTimeout, wantText: "getting default ServiceAccount"},
		{name: "unexpected error", apiErr: unexpected, wantCalls: 1, check: func(err error) bool { return errors.Is(err, unexpected) }, wantText: "getting default ServiceAccount"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client := fake.NewClientset()
			calls := 0
			client.PrependReactor("get", "serviceaccounts", func(action clienttesting.Action) (bool, runtime.Object, error) {
				calls++
				if action.GetNamespace() != "dns" || action.(clienttesting.GetAction).GetName() != "default" {
					t.Errorf("unexpected request %s/%s", action.GetNamespace(), action.(clienttesting.GetAction).GetName())
				}
				if test.cancel {
					cancel()
				}
				if test.notFound < 0 || calls <= test.notFound {
					return true, nil, apierrors.NewNotFound(serviceAccountsResource, "default")
				}
				if test.apiErr != nil {
					return true, nil, test.apiErr
				}
				return true, &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "dns"}}, nil
			})
			timeout := test.timeout
			if timeout == 0 {
				timeout = 5 * time.Second
			}

			err := waitForDefaultServiceAccount(ctx, client, "dns", time.Millisecond, timeout)

			if test.check == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			} else {
				if err == nil || !test.check(err) {
					t.Fatalf("unexpected error: %v", err)
				}
				if !strings.Contains(err.Error(), "namespace dns") || !strings.Contains(err.Error(), test.wantText) {
					t.Fatalf("error lacks context: %v", err)
				}
			}
			if test.wantCalls > 0 && calls != test.wantCalls {
				t.Fatalf("got %d requests, want %d", calls, test.wantCalls)
			}
			if test.timeout > 0 && calls < 2 {
				t.Fatalf("NotFound was not retried: %d requests", calls)
			}
		})
	}
}

func TestCreateTestNamespace(t *testing.T) {
	t.Parallel()
	forbidden := func(resource string) error {
		return apierrors.NewForbidden(schema.GroupResource{Resource: resource}, "dns", errors.New("denied"))
	}
	for _, test := range []struct {
		name               string
		reactorVerb        string
		reactorResource    string
		wantErr            func(error) bool
		wantCleanup        bool
		wantServiceAccount int
	}{
		{name: "ready", wantCleanup: true, wantServiceAccount: 1},
		{name: "service account forbidden", reactorVerb: "get", reactorResource: "serviceaccounts", wantErr: apierrors.IsForbidden, wantCleanup: true, wantServiceAccount: 1},
		{name: "namespace forbidden", reactorVerb: "create", reactorResource: "namespaces", wantErr: apierrors.IsForbidden},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := fake.NewClientset(&corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "dns"}})
			if test.reactorVerb != "" {
				client.PrependReactor(test.reactorVerb, test.reactorResource, func(clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, forbidden(test.reactorResource)
				})
			}

			cleanup, err := CreateTestNamespace(context.Background(), client, "dns")

			if test.wantErr == nil && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if test.wantErr != nil && (err == nil || !test.wantErr(err)) {
				t.Fatalf("unexpected error: %v", err)
			}
			serviceAccountReads := 0
			for _, action := range client.Actions() {
				if action.GetVerb() == "get" && action.GetResource().Resource == "serviceaccounts" {
					serviceAccountReads++
				}
			}
			if serviceAccountReads != test.wantServiceAccount {
				t.Fatalf("got %d service account reads, want %d", serviceAccountReads, test.wantServiceAccount)
			}
			if (cleanup != nil) != test.wantCleanup {
				t.Fatalf("cleanup returned=%v, want %v", cleanup != nil, test.wantCleanup)
			}
			if cleanup == nil {
				return
			}
			namespace, err := client.CoreV1().Namespaces().Get(context.Background(), "dns", metav1.GetOptions{})
			if err != nil {
				t.Fatalf("namespace not created: %v", err)
			}
			if namespace.Labels["e2e-test"] != "true" || namespace.Labels["created-by"] != "rosa-e2e" {
				t.Fatalf("unexpected labels: %v", namespace.Labels)
			}
			if err := cleanup(); err != nil {
				t.Fatalf("cleanup failed: %v", err)
			}
			if _, err := client.CoreV1().Namespaces().Get(context.Background(), "dns", metav1.GetOptions{}); !apierrors.IsNotFound(err) {
				t.Fatalf("namespace not deleted: %v", err)
			}
		})
	}
}
