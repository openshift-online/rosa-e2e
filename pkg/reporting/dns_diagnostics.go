package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	dnsDiagnosticsTotalTimeout   = 12 * time.Second
	dnsDiagnosticsRequestTimeout = 3 * time.Second
)

// Event reasons and object kinds are machine identifiers (e.g. FailedCreate, Pod); anything else
// may be free-form text and is dropped.
var diagnosticIdentifier = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{0,63}$`)

type dnsEvent struct {
	Name         string      `json:"name"`
	Type         string      `json:"type,omitempty"`
	Reason       string      `json:"reason,omitempty"`
	ObjectKind   string      `json:"objectKind,omitempty"`
	ObjectName   string      `json:"objectName"`
	Count        int32       `json:"count"`
	LastObserved metav1.Time `json:"lastObserved"`
}

type dnsSnapshot struct {
	CapturedAt            time.Time         `json:"capturedAt"`
	Namespace             *rolloutIdentity  `json:"namespace,omitempty"`
	NamespacePhase        string            `json:"namespacePhase,omitempty"`
	DefaultServiceAccount *rolloutIdentity  `json:"defaultServiceAccount,omitempty"`
	Events                []dnsEvent        `json:"events,omitempty"`
	ReadStatus            map[string]string `json:"readStatus"`
}

func safeIdentifier(value string) string {
	if diagnosticIdentifier.MatchString(value) {
		return value
	}
	return ""
}

func eventType(value string) string {
	if value == corev1.EventTypeNormal || value == corev1.EventTypeWarning {
		return value
	}
	return ""
}

func namespacePhase(phase corev1.NamespacePhase) string {
	if phase == corev1.NamespaceActive || phase == corev1.NamespaceTerminating {
		return string(phase)
	}
	return "Unknown"
}

// eventOccurrence returns the latest observation time and occurrence count. Events recorded
// through events.k8s.io leave the legacy count and lastTimestamp empty and keep them in series.
func eventOccurrence(event corev1.Event) (metav1.Time, int32) {
	observed, count := event.LastTimestamp, event.Count
	if observed.IsZero() && !event.EventTime.IsZero() {
		observed = metav1.NewTime(event.EventTime.Time)
	}
	if series := event.Series; series != nil {
		count = max(count, series.Count)
		if series.LastObservedTime.After(observed.Time) {
			observed = metav1.NewTime(series.LastObservedTime.Time)
		}
	}
	return observed, count
}

// WriteDNSFailureDiagnostics records the namespace, default ServiceAccount and bounded event
// identities for a failed DNS spec under ARTIFACT_DIR/dns-failure-diagnostics. Collection is
// best-effort: individual read failures are recorded in readStatus, never returned.
func WriteDNSFailureDiagnostics(ctx context.Context, client kubernetes.Interface, namespace string) error {
	return writeDNSFailureDiagnostics(ctx, client, namespace, dnsDiagnosticsTotalTimeout, dnsDiagnosticsRequestTimeout)
}

func writeDNSFailureDiagnostics(ctx context.Context, client kubernetes.Interface, namespace string, totalTimeout, requestTimeout time.Duration) error {
	if client == nil || namespace == "" {
		return errors.New("DNS diagnostics require a client and namespace")
	}
	ctx, cancel := context.WithTimeout(ctx, totalTimeout)
	defer cancel()
	snapshot := dnsSnapshot{CapturedAt: time.Now().UTC(), ReadStatus: make(map[string]string)}
	read := func(resource string, collect func(context.Context) (bool, error)) {
		requestCtx, requestCancel := context.WithTimeout(ctx, requestTimeout)
		defer requestCancel()
		truncated, err := collect(requestCtx)
		switch {
		case apierrors.IsNotFound(err):
			snapshot.ReadStatus[resource] = "not-found"
		case err != nil:
			snapshot.ReadStatus[resource] = readStatus(err)
		case truncated:
			snapshot.ReadStatus[resource] = "truncated"
		default:
			snapshot.ReadStatus[resource] = "complete"
		}
	}
	read("namespace", func(ctx context.Context) (bool, error) {
		value, err := client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
		if err == nil {
			identity := diagnosticIdentity(value.ObjectMeta)
			snapshot.Namespace = &identity
			snapshot.NamespacePhase = namespacePhase(value.Status.Phase)
		}
		return false, err
	})
	read("defaultServiceAccount", func(ctx context.Context) (bool, error) {
		value, err := client.CoreV1().ServiceAccounts(namespace).Get(ctx, "default", metav1.GetOptions{})
		if err == nil {
			identity := diagnosticIdentity(value.ObjectMeta)
			snapshot.DefaultServiceAccount = &identity
		}
		return false, err
	})
	read("events", func(ctx context.Context) (bool, error) {
		list, err := client.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{Limit: diagnosticLimit})
		if err != nil {
			return false, err
		}
		for _, event := range list.Items[:min(len(list.Items), diagnosticLimit)] {
			observed, count := eventOccurrence(event)
			snapshot.Events = append(snapshot.Events, dnsEvent{
				Name:         boundedIdentity(event.Name),
				Type:         eventType(event.Type),
				Reason:       safeIdentifier(event.Reason),
				ObjectKind:   safeIdentifier(event.InvolvedObject.Kind),
				ObjectName:   boundedIdentity(event.InvolvedObject.Name),
				Count:        count,
				LastObserved: observed,
			})
		}
		return list.Continue != "" || len(list.Items) > diagnosticLimit, nil
	})
	output, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return errors.New("DNS diagnostics serialization failed")
	}
	directory := os.Getenv("ARTIFACT_DIR")
	if directory == "" {
		directory = "/tmp"
	}
	directory = filepath.Join(directory, "dns-failure-diagnostics")
	if err := os.MkdirAll(directory, 0755); err != nil {
		return errors.New("DNS diagnostics directory unavailable")
	}
	file, err := os.CreateTemp(directory, "snapshot-*.json")
	if err != nil {
		return errors.New("DNS diagnostics file unavailable")
	}
	if err := writeDiagnosticFile(file, output); err != nil {
		return errors.New("DNS diagnostics write failed")
	}
	return nil
}
