package framework

import (
	"fmt"

	"k8s.io/client-go/dynamic"
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

// NewDynamicClient creates a dynamic client from the given rest config.
func NewDynamicClient(restConfig *rest.Config) (dynamic.Interface, error) {
	client, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("creating dynamic client: %w", err)
	}
	return client, nil
}
