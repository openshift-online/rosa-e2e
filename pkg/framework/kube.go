package framework

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// NewKubeClient creates a Kubernetes clientset from the given rest config.
func NewKubeClient(restConfig *rest.Config) (kubernetes.Interface, error) {
	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("creating kubernetes client: %w", err)
	}
	return client, nil
}
