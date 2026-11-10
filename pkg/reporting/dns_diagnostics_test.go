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

const privateDiagnosticValue = "private-data-must-not-be-captured"

func readDNSDiagnosticFile(t *testing.T, directory string) (dnsSnapshot, string) {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(directory, "dns-failure-diagnostics", "*.json"))
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
	return snapshot, string(content)
}

func TestDNSFailureDiagnostics(t *testing.T) {
	for _, test := range []struct {
		name                                     string
		namespaceMissing, accountMissing, denied bool
		wantNamespace, wantAccount               string
	}{
		{name: "present", wantNamespace: "complete", wantAccount: "complete"},
		{name: "service account missing", accountMissing: true, wantNamespace: "complete", wantAccount: "not-found"},
		{name: "service account access denied", denied: true, wantNamespace: "complete", wantAccount: "access-denied"},
		{name: "namespace missing", namespaceMissing: true, accountMissing: true, wantNamespace: "not-found", wantAccount: "not-found"},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			t.Setenv("ARTIFACT_DIR", directory)
			client := fake.NewClientset()
			if !test.namespaceMissing {
				namespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "dns", Annotations: map[string]string{"private": privateDiagnosticValue}}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceActive}}
				if _, err := client.CoreV1().Namespaces().Create(context.Background(), namespace, metav1.CreateOptions{}); err != nil {
					t.Fatal(err)
				}
			}
			if !test.accountMissing {
				account := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "default"}, Secrets: []corev1.ObjectReference{{Name: privateDiagnosticValue}}}
				if _, err := client.CoreV1().ServiceAccounts("dns").Create(context.Background(), account, metav1.CreateOptions{}); err != nil {
					t.Fatal(err)
				}
			}
			if test.denied {
				client.PrependReactor("get", "serviceaccounts", func(clienttesting.Action) (bool, runtime.Object, error) {
					return true, nil, apierrors.NewForbidden(schema.GroupResource{Resource: "serviceaccounts"}, "default", errors.New(privateDiagnosticValue))
				})
			}
			for index := 0; index < diagnosticLimit+1; index++ {
				event := &corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("event-%d", index)}, Message: privateDiagnosticValue, Reason: privateDiagnosticValue, Type: privateDiagnosticValue}
				if _, err := client.CoreV1().Events("dns").Create(context.Background(), event, metav1.CreateOptions{}); err != nil {
					t.Fatal(err)
				}
			}
			started := time.Now().UTC().Add(-time.Second)

			if err := WriteDNSFailureDiagnostics(context.Background(), client, "dns"); err != nil {
				t.Fatal(err)
			}

			snapshot, content := readDNSDiagnosticFile(t, directory)
			if strings.Contains(content, privateDiagnosticValue) {
				t.Fatal("private data leaked")
			}
			if snapshot.CapturedAt.Before(started) || snapshot.CapturedAt.After(time.Now().UTC().Add(time.Second)) {
				t.Fatalf("unexpected capture time %v", snapshot.CapturedAt)
			}
			if snapshot.ReadStatus["namespace"] != test.wantNamespace || snapshot.ReadStatus["defaultServiceAccount"] != test.wantAccount || snapshot.ReadStatus["events"] != "truncated" {
				t.Fatalf("unexpected read status: %v", snapshot.ReadStatus)
			}
			if test.namespaceMissing != (snapshot.Namespace == nil) {
				t.Fatalf("unexpected namespace identity: %+v", snapshot.Namespace)
			}
			if !test.namespaceMissing && (snapshot.Namespace.Name != "dns" || snapshot.NamespacePhase != "Active") {
				t.Fatalf("unexpected namespace: %+v phase=%q", snapshot.Namespace, snapshot.NamespacePhase)
			}
			if (test.wantAccount == "complete") != (snapshot.DefaultServiceAccount != nil) {
				t.Fatalf("unexpected service account identity: %+v", snapshot.DefaultServiceAccount)
			}
			if len(snapshot.Events) != diagnosticLimit {
				t.Fatalf("got %d events, want %d", len(snapshot.Events), diagnosticLimit)
			}
			for _, event := range snapshot.Events {
				if event.Reason != "" || event.Type != "" {
					t.Fatalf("free-form event fields captured: %+v", event)
				}
			}
			for _, action := range client.Actions() {
				if (action.GetVerb() == "get" || action.GetVerb() == "list") && action.GetResource().Resource != "namespaces" && action.GetNamespace() != "dns" {
					t.Fatalf("out of scope read: %v", action)
				}
			}
		})
	}
}

func TestDNSFailureDiagnosticsEvents(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("ARTIFACT_DIR", directory)
	observed := time.Date(2026, 11, 4, 3, 5, 47, 0, time.UTC)
	client := fake.NewClientset(
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "dns"}, Status: corev1.NamespaceStatus{Phase: corev1.NamespaceTerminating}},
		&corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: "allowlisted", Namespace: "dns"}, Type: corev1.EventTypeWarning, Reason: "FailedCreate", InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "dns-test-pod"}, Count: 2, LastTimestamp: metav1.NewTime(observed)},
		&corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: "free-form", Namespace: "dns"}, Type: privateDiagnosticValue, Reason: "private reason: " + privateDiagnosticValue, InvolvedObject: corev1.ObjectReference{Kind: privateDiagnosticValue}},
		&corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: "event-time-only", Namespace: "dns"}, Type: corev1.EventTypeNormal, EventTime: metav1.NewMicroTime(observed)},
		&corev1.Event{ObjectMeta: metav1.ObjectMeta{Name: "series", Namespace: "dns"}, Type: corev1.EventTypeWarning, Reason: "FailedScheduling", EventTime: metav1.NewMicroTime(observed), Series: &corev1.EventSeries{Count: 7, LastObservedTime: metav1.NewMicroTime(observed.Add(30 * time.Second))}},
	)

	if err := WriteDNSFailureDiagnostics(context.Background(), client, "dns"); err != nil {
		t.Fatal(err)
	}

	snapshot, content := readDNSDiagnosticFile(t, directory)
	if strings.Contains(content, privateDiagnosticValue) {
		t.Fatal("private data leaked")
	}
	if snapshot.NamespacePhase != "Terminating" || snapshot.ReadStatus["events"] != "complete" {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	events := make(map[string]dnsEvent)
	for _, event := range snapshot.Events {
		events[event.Name] = event
	}
	if got := events["allowlisted"]; got.Type != "Warning" || got.Reason != "FailedCreate" || got.ObjectKind != "Pod" || got.ObjectName != "dns-test-pod" || got.Count != 2 || !got.LastObserved.Time.Equal(observed) {
		t.Fatalf("unexpected allowlisted event: %+v", got)
	}
	if got := events["free-form"]; got.Type != "" || got.Reason != "" || got.ObjectKind != "" {
		t.Fatalf("free-form event fields captured: %+v", got)
	}
	if got := events["event-time-only"]; got.Type != "Normal" || !got.LastObserved.Time.Equal(observed) {
		t.Fatalf("event time not used: %+v", got)
	}
	if got := events["series"]; got.Reason != "FailedScheduling" || got.Count != 7 || !got.LastObserved.Time.Equal(observed.Add(30*time.Second)) {
		t.Fatalf("event series not used: %+v", got)
	}
}

func TestDNSDiagnosticAllowlists(t *testing.T) {
	for value, want := range map[string]string{"FailedCreate": "FailedCreate", "Pod": "Pod", "": "", "Failed Create": "", "1Pod": "", "a-b": "", strings.Repeat("a", 64): strings.Repeat("a", 64), strings.Repeat("a", 65): ""} {
		if got := safeIdentifier(value); got != want {
			t.Errorf("safeIdentifier(%q)=%q, want %q", value, got, want)
		}
	}
	for value, want := range map[string]string{"Normal": "Normal", "Warning": "Warning", "Error": "", "": ""} {
		if got := eventType(value); got != want {
			t.Errorf("eventType(%q)=%q, want %q", value, got, want)
		}
	}
	for phase, want := range map[corev1.NamespacePhase]string{corev1.NamespaceActive: "Active", corev1.NamespaceTerminating: "Terminating", "": "Unknown", "Other": "Unknown"} {
		if got := namespacePhase(phase); got != want {
			t.Errorf("namespacePhase(%q)=%q, want %q", phase, got, want)
		}
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
	if err := WriteDNSFailureDiagnostics(context.Background(), nil, "dns"); err == nil {
		t.Fatal("expected missing client error")
	}
	if err := WriteDNSFailureDiagnostics(context.Background(), fake.NewClientset(), ""); err == nil {
		t.Fatal("expected missing namespace error")
	}
}

func hangingClient(t *testing.T) kubernetes.Interface {
	t.Helper()
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		select {
		case <-request.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(func() {
		close(release)
		server.Close()
	})
	client, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestDNSFailureDiagnosticsInternalTimeouts(t *testing.T) {
	for _, test := range []struct {
		name                         string
		totalTimeout, requestTimeout time.Duration
	}{
		{name: "total deadline", totalTimeout: 60 * time.Millisecond, requestTimeout: time.Hour},
		{name: "request deadline", totalTimeout: time.Hour, requestTimeout: 30 * time.Millisecond},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			t.Setenv("ARTIFACT_DIR", directory)
			client := hangingClient(t)
			done := make(chan error, 1)
			go func() {
				done <- writeDNSFailureDiagnostics(context.Background(), client, "dns", test.totalTimeout, test.requestTimeout)
			}()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("diagnostics ignored their internal deadline")
			}
			snapshot, _ := readDNSDiagnosticFile(t, directory)
			for _, resource := range []string{"namespace", "defaultServiceAccount", "events"} {
				if status := snapshot.ReadStatus[resource]; status != "timeout" {
					t.Errorf("%s status=%s", resource, status)
				}
			}
		})
	}
}

func TestDNSFailureDiagnosticsDeadline(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("ARTIFACT_DIR", directory)
	client := hangingClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()

	if err := WriteDNSFailureDiagnostics(ctx, client, "dns"); err != nil {
		t.Fatal(err)
	}

	if time.Since(started) > time.Second {
		t.Fatal("diagnostics ignored parent deadline")
	}
	snapshot, _ := readDNSDiagnosticFile(t, directory)
	for _, resource := range []string{"namespace", "defaultServiceAccount", "events"} {
		if status := snapshot.ReadStatus[resource]; status != "timeout" {
			t.Errorf("%s status=%s", resource, status)
		}
	}
}
