//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package multi_efa_dra

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

// The pinned client-go (v0.23) predates the resource.k8s.io DRA API, so the DRA
// objects are read as unstructured via the dynamic client against the live API
// server (JSON, version-agnostic). resource.k8s.io/v1 is GA on the 1.34+ clusters
// these tests run on.
var (
	podGVR   = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}
	claimGVR = schema.GroupVersionResource{Group: "resource.k8s.io", Version: "v1", Resource: "resourceclaims"}
	sliceGVR = schema.GroupVersionResource{Group: "resource.k8s.io", Version: "v1", Resource: "resourceslices"}
)

// efaburnAllocatedRDMADevice returns the aws.efa.device value (an rdmapXsY name)
// that the EFA actually allocated to efaburn should carry, read independently of
// the metrics pipeline: efaburn pod → status.resourceClaimStatuses →
// ResourceClaim allocation (driver dra.net, a pci device name) → matching
// ResourceSlice device's dra.net/rdmaDevice attribute. Returns ok=false (and logs
// why) on any infra error, so the caller skips rather than false-fails.
func efaburnAllocatedRDMADevice(t *testing.T) (string, bool) {
	t.Helper()
	const (
		namespace = "default"
		driver    = "dra.net"
		attrKey   = "dra.net/rdmaDevice"
	)

	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		kubeconfig = filepath.Join(os.Getenv("HOME"), ".kube", "config")
	}
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		t.Logf("ground truth skipped: building kubeconfig: %v", err)
		return "", false
	}
	dyn, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		t.Logf("ground truth skipped: dynamic client: %v", err)
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// efaburn pod → generated ResourceClaim name.
	pods, err := dyn.Resource(podGVR).Namespace(namespace).List(ctx, metav1.ListOptions{LabelSelector: "app=efaburn"})
	if err != nil || len(pods.Items) == 0 {
		t.Logf("ground truth skipped: listing efaburn pods: err=%v count=%d", err, len(pods.Items))
		return "", false
	}
	claimName := ""
	for _, p := range pods.Items {
		statuses, _, _ := unstructured.NestedSlice(p.Object, "status", "resourceClaimStatuses")
		for _, s := range statuses {
			sm, ok := s.(map[string]interface{})
			if !ok {
				continue
			}
			if n, _, _ := unstructured.NestedString(sm, "resourceClaimName"); n != "" {
				claimName = n
				break
			}
		}
		if claimName != "" {
			break
		}
	}
	if claimName == "" {
		t.Logf("ground truth skipped: no resourceClaimName on efaburn pod status")
		return "", false
	}

	// ResourceClaim allocation → the dra.net device (a pci name) and its pool.
	claim, err := dyn.Resource(claimGVR).Namespace(namespace).Get(ctx, claimName, metav1.GetOptions{})
	if err != nil {
		t.Logf("ground truth skipped: getting ResourceClaim %q: %v", claimName, err)
		return "", false
	}
	results, _, _ := unstructured.NestedSlice(claim.Object, "status", "allocation", "devices", "results")
	var device, pool string
	for _, r := range results {
		rm, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		if d, _, _ := unstructured.NestedString(rm, "driver"); d != driver {
			continue
		}
		device, _, _ = unstructured.NestedString(rm, "device")
		pool, _, _ = unstructured.NestedString(rm, "pool")
		break
	}
	if device == "" {
		t.Logf("ground truth skipped: no %s device in claim %q allocation", driver, claimName)
		return "", false
	}

	// Matching ResourceSlice device → dra.net/rdmaDevice attribute.
	slices, err := dyn.Resource(sliceGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Logf("ground truth skipped: listing ResourceSlices: %v", err)
		return "", false
	}
	for _, sl := range slices.Items {
		if d, _, _ := unstructured.NestedString(sl.Object, "spec", "driver"); d != driver {
			continue
		}
		if pool != "" {
			if pn, _, _ := unstructured.NestedString(sl.Object, "spec", "pool", "name"); pn != pool {
				continue
			}
		}
		devs, _, _ := unstructured.NestedSlice(sl.Object, "spec", "devices")
		for _, d := range devs {
			dm, ok := d.(map[string]interface{})
			if !ok {
				continue
			}
			if n, _, _ := unstructured.NestedString(dm, "name"); n != device {
				continue
			}
			if rdma, _, _ := unstructured.NestedString(dm, "attributes", attrKey, "string"); rdma != "" {
				return rdma, true
			}
		}
	}
	t.Logf("ground truth skipped: no ResourceSlice device %q (driver %s) carrying %s", device, driver, attrKey)
	return "", false
}
