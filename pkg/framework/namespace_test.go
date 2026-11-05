package framework

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

func TestWaitForDefaultServiceAccount(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		missing  int
		apiError error
		cancel   bool
		want     error
	}{
		{name: "present"},
		{name: "not found then present", missing: 1},
		{name: "timeout", missing: -1, want: context.DeadlineExceeded},
		{name: "canceled", missing: -1, cancel: true, want: context.Canceled},
		{name: "forbidden", apiError: apierrors.NewForbidden(schema.GroupResource{Resource: "serviceaccounts"}, "default", errors.New("denied"))},
		{name: "unauthorized", apiError: apierrors.NewUnauthorized("denied")},
		{name: "unexpected", apiError: errors.New("unexpected API failure")},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			client := fake.NewClientset()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			client.PrependReactor("get", "serviceaccounts", func(action clienttesting.Action) (bool, runtime.Object, error) {
				calls++
				if action.GetNamespace() != "dns" || action.(clienttesting.GetAction).GetName() != "default" {
					t.Fatal("unexpected service account request")
				}
				if test.cancel {
					cancel()
				}
				if test.apiError != nil {
					return true, nil, test.apiError
				}
				if test.missing < 0 || calls <= test.missing {
					return true, nil, apierrors.NewNotFound(schema.GroupResource{Resource: "serviceaccounts"}, "default")
				}
				return true, &corev1.ServiceAccount{}, nil
			})
			err := waitForDefaultServiceAccount(ctx, client.CoreV1().ServiceAccounts("dns"), time.Millisecond, 50*time.Millisecond)
			want := test.want
			if test.apiError != nil {
				want = test.apiError
			}
			if !errors.Is(err, want) {
				t.Fatalf("got %v, want %v", err, want)
			}
			if err != nil && !strings.Contains(err.Error(), "default service account (timeout") {
				t.Fatalf("missing wait context: %v", err)
			}
			if test.apiError != nil && calls != 1 {
				t.Fatalf("unexpected error retried %d times", calls)
			}
			if test.name == "not found then present" && calls != 2 {
				t.Fatalf("got %d requests, want 2", calls)
			}
		})
	}
}
