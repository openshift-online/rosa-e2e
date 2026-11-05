package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	clienttesting "k8s.io/client-go/testing"
)

func TestDNSFailureDiagnostics(t *testing.T) {
	for _, test := range []struct {
		name            string
		missing, denied bool
	}{
		{name: "present"}, {name: "missing", missing: true}, {name: "access denied", denied: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			t.Setenv("ARTIFACT_DIR", directory)
			private := "private-data-must-not-be-captured"
			client := fake.NewClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "dns", Annotations: map[string]string{"private": private}}})
			if !test.missing {
				_, err := client.CoreV1().ServiceAccounts("dns").Create(context.Background(), &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "default"}, Secrets: []corev1.ObjectReference{{Name: private}}}, metav1.CreateOptions{})
				if err != nil {
					t.Fatal(err)
				}
			}
			if test.denied {
				client.PrependReactor("get", "serviceaccounts", func(clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "serviceaccounts"}, "default", errors.New(private))
				})
			}
			for index := 0; index < diagnosticLimit+1; index++ {
				_, err := client.CoreV1().Events("dns").Create(context.Background(), &corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("event-%d", index)}, Message: private, Reason: private}, metav1.CreateOptions{})
				if err != nil {
					t.Fatal(err)
				}
			}
			if err := WriteDNSFailureDiagnostics(context.Background(), client, "dns"); err != nil {
				t.Fatal(err)
			}
			paths, err := filepath.Glob(filepath.Join(directory, "dns-failure-diagnostics", "*.json"))
			if err != nil || len(paths) != 1 {
				t.Fatalf("paths=%v err=%v", paths, err)
			}
			content, err := os.ReadFile(paths[0])
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(content), private) {
				t.Fatal("private data leaked")
			}
			var snapshot dnsSnapshot
			if err := json.Unmarshal(content, &snapshot); err != nil {
				t.Fatal(err)
			}
			want := "complete"
			if test.missing {
				want = "not-found"
			}
			if test.denied {
				want = "access-denied"
			}
			if snapshot.Namespace == nil || snapshot.Namespace.Name != "dns" || snapshot.ReadStatus["defaultServiceAccount"] != want || snapshot.ReadStatus["events"] != "truncated" || len(snapshot.Events) != diagnosticLimit {
				t.Fatalf("unexpected snapshot: %+v", snapshot)
			}
			for _, action := range client.Actions() {
				if action.GetVerb() == "get" || action.GetVerb() == "list" {
					if action.GetResource().Resource != "namespaces" && action.GetNamespace() != "dns" {
						t.Fatalf("out of scope read: %v", action)
					}
				}
			}
		})
	}
}

func TestDNSFailureDiagnosticsWriteError(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ARTIFACT_DIR", file)
	if err := WriteDNSFailureDiagnostics(context.Background(), fake.NewClientset(), "dns"); err == nil {
		t.Fatal("expected write error")
	}
}

func TestDNSFailureDiagnosticsDeadline(t *testing.T) {
	t.Setenv("ARTIFACT_DIR", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) { <-request.Context().Done() }))
	defer server.Close()
	client, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	if err := WriteDNSFailureDiagnostics(ctx, client, "dns"); err != nil {
		t.Fatal(err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("diagnostics ignored parent deadline")
	}
	paths, err := filepath.Glob(filepath.Join(os.Getenv("ARTIFACT_DIR"), "dns-failure-diagnostics", "*.json"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("paths=%v err=%v", paths, err)
	}
	content, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	var snapshot dnsSnapshot
	if err := json.Unmarshal(content, &snapshot); err != nil {
		t.Fatal(err)
	}
	for resource, status := range snapshot.ReadStatus {
		if status != "timeout" {
			t.Errorf("%s status=%s", resource, status)
		}
	}
}
