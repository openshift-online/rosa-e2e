package framework

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/ptr"
)

// CreateTestPVC creates a PersistentVolumeClaim with the given storage class and returns its name.
func CreateTestPVC(ctx context.Context, client kubernetes.Interface, namespace, storageClass string) (pvcName string, err error) {
	pvcName = "test-pvc"
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				},
			},
		},
	}

	if storageClass != "" {
		pvc.Spec.StorageClassName = &storageClass
	}

	_, err = client.CoreV1().PersistentVolumeClaims(namespace).Create(ctx, pvc, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("creating PVC: %w", err)
	}

	return pvcName, nil
}

// CreateTestPodWithPVC creates a pod that mounts the given PVC at /data and writes a test file.
func CreateTestPodWithPVC(ctx context.Context, client kubernetes.Interface, namespace, pvcName string) (podName string, err error) {
	podName = "test-pod-pvc"
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: namespace,
		},
		Spec: corev1.PodSpec{
			SecurityContext: &corev1.PodSecurityContext{
				RunAsNonRoot: ptr.To(true),
				RunAsUser:    ptr.To(int64(1000)),
				FSGroup:      ptr.To(int64(1000)),
				SeccompProfile: &corev1.SeccompProfile{
					Type: corev1.SeccompProfileTypeRuntimeDefault,
				},
			},
			Containers: []corev1.Container{
				{
					Name:    "busybox",
					Image:   "registry.access.redhat.com/ubi9/ubi-minimal:latest",
					Command: []string{"/bin/sh"},
					Args: []string{
						"-c",
						"echo 'test data' > /data/test.txt && sleep 3600",
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "data",
							MountPath: "/data",
						},
					},
					SecurityContext: &corev1.SecurityContext{
						AllowPrivilegeEscalation: ptr.To(false),
						Capabilities: &corev1.Capabilities{
							Drop: []corev1.Capability{"ALL"},
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "data",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvcName,
						},
					},
				},
			},
		},
	}

	_, err = client.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("creating pod with PVC: %w", err)
	}

	return podName, nil
}

// CreateKMSStorageClass creates a gp3 storage class whose dynamically
// provisioned volumes are encrypted with the given customer-managed KMS key,
// and returns a cleanup function that deletes it.
func CreateKMSStorageClass(ctx context.Context, client kubernetes.Interface, name, kmsKeyARN string) (cleanup func() error, err error) {
	sc := &storagev1.StorageClass{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Provisioner:       "ebs.csi.aws.com",
		VolumeBindingMode: ptr.To(storagev1.VolumeBindingImmediate),
		ReclaimPolicy:     ptr.To(corev1.PersistentVolumeReclaimDelete),
		Parameters: map[string]string{
			"type":      "gp3",
			"encrypted": "true",
			"kmsKeyId":  kmsKeyARN,
		},
	}

	_, err = client.StorageV1().StorageClasses().Create(ctx, sc, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("creating storage class %s: %w", name, err)
	}

	cleanup = func() error {
		if deleteErr := client.StorageV1().StorageClasses().Delete(context.Background(), name, metav1.DeleteOptions{}); deleteErr != nil {
			return fmt.Errorf("deleting storage class %s: %w", name, deleteErr)
		}
		return nil
	}

	return cleanup, nil
}

// GetPVCVolumeID resolves the EBS volume ID backing a bound PVC.
func GetPVCVolumeID(ctx context.Context, client kubernetes.Interface, namespace, pvcName string) (string, error) {
	pvc, err := client.CoreV1().PersistentVolumeClaims(namespace).Get(ctx, pvcName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("getting PVC %s/%s: %w", namespace, pvcName, err)
	}
	if pvc.Spec.VolumeName == "" {
		return "", fmt.Errorf("PVC %s/%s is not bound to a volume yet", namespace, pvcName)
	}

	pv, err := client.CoreV1().PersistentVolumes().Get(ctx, pvc.Spec.VolumeName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("getting PV %s: %w", pvc.Spec.VolumeName, err)
	}
	if pv.Spec.CSI == nil || pv.Spec.CSI.VolumeHandle == "" {
		return "", fmt.Errorf("PV %s has no CSI volume handle", pv.Name)
	}

	return pv.Spec.CSI.VolumeHandle, nil
}
