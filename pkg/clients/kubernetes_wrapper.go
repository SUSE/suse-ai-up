package clients

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// KubeClientWrapper wraps Kubernetes client operations
type KubeClientWrapper struct {
	client    *kubernetes.Clientset
	namespace string
}

// NewKubeClientWrapper creates a new wrapper
func NewKubeClientWrapper(client *kubernetes.Clientset, namespace string) *KubeClientWrapper {
	return &KubeClientWrapper{client: client, namespace: namespace}
}

// GetNamespace returns the configured namespace
func (k *KubeClientWrapper) GetNamespace() string {
	return k.namespace
}

// UpsertDeployment creates or updates a Deployment
func (k *KubeClientWrapper) UpsertDeployment(deployment *appsv1.Deployment, namespace string, ctx context.Context) error {
	_, err := k.client.AppsV1().Deployments(namespace).Create(ctx, deployment, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		_, err = k.client.AppsV1().Deployments(namespace).Update(ctx, deployment, metav1.UpdateOptions{})
	}
	return err
}

// UpsertStatefulSet creates or updates a StatefulSet
func (k *KubeClientWrapper) UpsertStatefulSet(ss *appsv1.StatefulSet, namespace string, ctx context.Context) error {
	_, err := k.client.AppsV1().StatefulSets(namespace).Create(ctx, ss, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		_, err = k.client.AppsV1().StatefulSets(namespace).Update(ctx, ss, metav1.UpdateOptions{})
	}
	return err
}

// ReadStatefulSet reads a StatefulSet
func (k *KubeClientWrapper) ReadStatefulSet(name, namespace string, ctx context.Context) (*appsv1.StatefulSet, error) {
	return k.client.AppsV1().StatefulSets(namespace).Get(ctx, name, metav1.GetOptions{})
}

// DeleteDeployment deletes a Deployment
func (k *KubeClientWrapper) DeleteDeployment(name, namespace string, ctx context.Context) error {
	return k.client.AppsV1().Deployments(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// DeleteStatefulSet deletes a StatefulSet
func (k *KubeClientWrapper) DeleteStatefulSet(name, namespace string, ctx context.Context) error {
	return k.client.AppsV1().StatefulSets(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// UpsertService creates or updates a Service
func (k *KubeClientWrapper) UpsertService(svc *corev1.Service, namespace string, ctx context.Context) error {
	_, err := k.client.CoreV1().Services(namespace).Create(ctx, svc, metav1.CreateOptions{})
	if errors.IsAlreadyExists(err) {
		_, err = k.client.CoreV1().Services(namespace).Update(ctx, svc, metav1.UpdateOptions{})
	}
	return err
}

// DeleteService deletes a Service
func (k *KubeClientWrapper) DeleteService(name, namespace string, ctx context.Context) error {
	return k.client.CoreV1().Services(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// ListPods lists pods with given options
func (k *KubeClientWrapper) ListPods(namespace string, labelSelector, fieldSelector string, ctx context.Context) (*corev1.PodList, error) {
	return k.client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
		FieldSelector: fieldSelector,
	})
}

// GetContainerLogStream gets logs from a pod
func (k *KubeClientWrapper) GetContainerLogStream(podName string, lines int64, namespace string, ctx context.Context) (string, error) {
	req := k.client.CoreV1().Pods(namespace).GetLogs(podName, &corev1.PodLogOptions{
		TailLines: &lines,
	})

	logs, err := req.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get logs: %w", err)
	}
	defer logs.Close()

	var logData []byte
	buf := make([]byte, 4096)
	for {
		n, err := logs.Read(buf)
		if n > 0 {
			logData = append(logData, buf[:n]...)
		}
		if err != nil {
			break
		}
	}

	return string(logData), nil
}
