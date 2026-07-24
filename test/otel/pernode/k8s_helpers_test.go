//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package pernode

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	// agentNamespace is where the operator, agent and Target Allocator run.
	agentNamespace = "amazon-cloudwatch"
	// targetAllocatorDeploymentName is the TA Deployment created by the operator.
	targetAllocatorDeploymentName = "cloudwatch-agent-target-allocator"
)

// k8sGroundTruth holds node data fetched from the Kubernetes API.
type k8sGroundTruth struct {
	nodes map[string]corev1.Node // keyed by metadata.name
}

var (
	groundTruth     *k8sGroundTruth
	groundTruthOnce sync.Once
	groundTruthErr  error
)

func kubeconfigPath() string {
	if kc := os.Getenv("KUBECONFIG"); kc != "" {
		return kc
	}
	return filepath.Join(os.Getenv("HOME"), ".kube", "config")
}

// k8sClientset builds a Kubernetes clientset from the ambient kubeconfig.
func k8sClientset(t *testing.T) *kubernetes.Clientset {
	t.Helper()
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath())
	if err != nil {
		t.Fatalf("building kubeconfig: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		t.Fatalf("creating K8s clientset: %v", err)
	}
	return clientset
}

// getGroundTruth returns the shared ground truth, initializing it on first call.
func getGroundTruth(t *testing.T) *k8sGroundTruth {
	t.Helper()
	groundTruthOnce.Do(func() {
		groundTruth, groundTruthErr = buildGroundTruth()
	})
	if groundTruthErr != nil {
		t.Fatalf("failed to build K8s ground truth: %v", groundTruthErr)
	}
	return groundTruth
}

func buildGroundTruth() (*k8sGroundTruth, error) {
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath())
	if err != nil {
		return nil, fmt.Errorf("building kubeconfig: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("creating K8s clientset: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	nodeList, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("listing nodes: %w", err)
	}
	if len(nodeList.Items) == 0 {
		return nil, fmt.Errorf("K8s API returned 0 nodes")
	}

	gt := &k8sGroundTruth{nodes: make(map[string]corev1.Node, len(nodeList.Items))}
	for _, n := range nodeList.Items {
		n := n
		gt.nodes[n.Name] = n
	}
	return gt, nil
}

// dynamicClient builds a dynamic client from the ambient kubeconfig, used to read
// CustomResourceDefinition objects (their ownership metadata) without pulling in the
// typed apiextensions clientset.
func dynamicClient(t *testing.T) dynamic.Interface {
	t.Helper()
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath())
	if err != nil {
		t.Fatalf("building kubeconfig: %v", err)
	}
	dyn, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		t.Fatalf("building dynamic client: %v", err)
	}
	return dyn
}

// crdManagedByHelm fetches a CustomResourceDefinition (named "<resource>.<group>"
// from the given GVR) and reports whether it exists and, if so, whether it carries
// app.kubernetes.io/managed-by=Helm plus its meta.helm.sh/release-namespace. This
// distinguishes a CRD this chart's Helm release installed from one served by a
// pre-existing prometheus-operator (which discovery alone cannot tell apart).
func crdManagedByHelm(t *testing.T, dyn dynamic.Interface, gvr schema.GroupVersionResource) (present bool, managedByHelm bool, releaseNamespace string) {
	t.Helper()
	crdName := gvr.Resource + "." + gvr.Group
	obj, err := dyn.Resource(gvrCRD).Get(context.Background(), crdName, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, false, ""
		}
		t.Fatalf("getting CRD %s: %v", crdName, err)
	}
	return true,
		obj.GetLabels()["app.kubernetes.io/managed-by"] == "Helm",
		obj.GetAnnotations()["meta.helm.sh/release-namespace"]
}

// gvrServiceMonitor / gvrPodMonitor identify the community CRDs we expect bundled.
var (
	gvrServiceMonitor = schema.GroupVersionResource{Group: "monitoring.coreos.com", Version: "v1", Resource: "servicemonitors"}
	gvrPodMonitor     = schema.GroupVersionResource{Group: "monitoring.coreos.com", Version: "v1", Resource: "podmonitors"}
	// gvrCRD is the apiextensions CustomResourceDefinition resource, used to read a
	// bundled CRD's object metadata (ownership labels) via the dynamic client.
	gvrCRD = schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}
)

// targetAllocatorDeployment fetches the Target Allocator Deployment.
func targetAllocatorDeployment(t *testing.T, clientset *kubernetes.Clientset) *appsv1.Deployment {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dep, err := clientset.AppsV1().Deployments(agentNamespace).Get(ctx, targetAllocatorDeploymentName, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("getting Target Allocator deployment %s/%s: %v", agentNamespace, targetAllocatorDeploymentName, err)
	}
	return dep
}

// targetAllocatorPods lists the Target Allocator pods.
func targetAllocatorPods(t *testing.T, clientset *kubernetes.Clientset) []corev1.Pod {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	list, err := clientset.CoreV1().Pods(agentNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=" + targetAllocatorDeploymentName,
	})
	if err != nil {
		t.Fatalf("listing Target Allocator pods: %v", err)
	}
	return list.Items
}

// totalRestarts sums container restart counts across the given pods.
func totalRestarts(pods []corev1.Pod) int32 {
	var total int32
	for _, p := range pods {
		for _, cs := range p.Status.ContainerStatuses {
			total += cs.RestartCount
		}
	}
	return total
}
