package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type dnsEvent struct {
	Name          string      `json:"name"`
	ObjectName    string      `json:"objectName"`
	Count         int32       `json:"count"`
	LastTimestamp metav1.Time `json:"lastTimestamp"`
}

type dnsSnapshot struct {
	Namespace             *rolloutIdentity  `json:"namespace,omitempty"`
	DefaultServiceAccount *rolloutIdentity  `json:"defaultServiceAccount,omitempty"`
	Events                []dnsEvent        `json:"events,omitempty"`
	ReadStatus            map[string]string `json:"readStatus"`
}

func WriteDNSFailureDiagnostics(ctx context.Context, client kubernetes.Interface, namespace string) error {
	if client == nil || namespace == "" {
		return errors.New("DNS diagnostics require a client and namespace")
	}
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	snapshot := dnsSnapshot{ReadStatus: make(map[string]string)}
	read := func(resource string, collect func(context.Context) (bool, error)) {
		requestCtx, requestCancel := context.WithTimeout(ctx, 3*time.Second)
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
			snapshot.Events = append(snapshot.Events, dnsEvent{Name: boundedIdentity(event.Name), ObjectName: boundedIdentity(event.InvolvedObject.Name), Count: event.Count, LastTimestamp: event.LastTimestamp})
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
	return writeDiagnosticFile(file, output)
}
