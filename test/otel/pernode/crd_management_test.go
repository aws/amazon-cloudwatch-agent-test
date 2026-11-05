//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package pernode

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Ownership metadata the operator writes on the ServiceMonitor/PodMonitor CRDs it
// creates (operator pkg/crdmanager). These keys are a contract with customers, so
// the suite pins them.
const (
	operatorFieldManager           = "amazon-cloudwatch-agent-operator"
	crdManagedByLabel              = "cloudwatch.aws.amazon.com/crd-managed-by"
	crdAppliedVersionAnnotation    = "cloudwatch.aws.amazon.com/crd-applied-version"
	crdAppliedGenerationAnnotation = "cloudwatch.aws.amazon.com/crd-applied-generation"
	upstreamVersionAnnotation      = "operator.prometheus.io/version"

	operatorDeploymentName  = "amazon-cloudwatch-observability-controller-manager"
	managePrometheusCRDsArg = "--manage-prometheus-crds"
)

// TestServiceMonitorPodMonitorCRDsCreatedByOperator verifies zero-step install
// (G1): after a plain chart install with SM/PM scraping enabled, the community
// ServiceMonitor and PodMonitor CRDs exist and were created by the operator, not
// by Helm or the EKS add-on. The harness never installs these CRDs, so absence
// means the operator did not create them.
//
// It checks the full ownership metadata rather than mere presence, so a
// pre-existing prometheus-operator install cannot make the test pass, and it
// checks that Helm does not own the CRDs, so uninstalling the chart or the add-on
// leaves them and every ServiceMonitor/PodMonitor in place.
func TestServiceMonitorPodMonitorCRDsCreatedByOperator(t *testing.T) {
	dyn := dynamicClient(t)

	for _, tc := range []struct {
		name string
		gvr  schema.GroupVersionResource
	}{
		{"ServiceMonitor", gvrServiceMonitor},
		{"PodMonitor", gvrPodMonitor},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			name := tc.name
			crd := getCRD(t, dyn, tc.gvr)
			require.NotNilf(t, crd, "%s CRD absent: the operator did not create it (the harness never installs it)", name)

			labels, ann := crd.GetLabels(), crd.GetAnnotations()

			// Created by the operator.
			assert.Equalf(t, operatorFieldManager, labels[crdManagedByLabel],
				"%s CRD label %s=%q, want %q (not created by the operator)", name, crdManagedByLabel, labels[crdManagedByLabel], operatorFieldManager)
			require.NotEmptyf(t, ann[upstreamVersionAnnotation], "%s CRD has no %s annotation", name, upstreamVersionAnnotation)
			assert.Equalf(t, ann[upstreamVersionAnnotation], ann[crdAppliedVersionAnnotation],
				"%s CRD %s=%q does not match %s=%q", name, upstreamVersionAnnotation, ann[upstreamVersionAnnotation], crdAppliedVersionAnnotation, ann[crdAppliedVersionAnnotation])
			assert.Equalf(t, strconv.FormatInt(crd.GetGeneration(), 10), ann[crdAppliedGenerationAnnotation],
				"%s CRD metadata.generation=%d does not match %s=%q", name, crd.GetGeneration(), crdAppliedGenerationAnnotation, ann[crdAppliedGenerationAnnotation])

			// The operator is the only writer of spec.
			assert.Equalf(t, []string{operatorFieldManager}, specManagers(crd.GetManagedFields()),
				"%s CRD spec field managers", name)

			// No other install tool's markers (the operator's ownership check steps
			// back on any of them), and no parent object, so uninstall leaves the CRD.
			assert.Emptyf(t, labels["app.kubernetes.io/managed-by"], "%s CRD carries app.kubernetes.io/managed-by", name)
			assert.Emptyf(t, ann["meta.helm.sh/release-name"], "%s CRD belongs to a Helm release", name)
			assert.Emptyf(t, ann["kubectl.kubernetes.io/last-applied-configuration"], "%s CRD was client-side applied", name)
			assert.Emptyf(t, crd.GetOwnerReferences(), "%s CRD has owner references", name)
		})
	}
}

// TestOperatorManagesPrometheusCRDs verifies the chart wiring: with SM/PM scraping
// enabled, the chart starts the operator with --manage-prometheus-crds.
func TestOperatorManagesPrometheusCRDs(t *testing.T) {
	clientset := k8sClientset(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dep, err := clientset.AppsV1().Deployments(agentNamespace).Get(ctx, operatorDeploymentName, metav1.GetOptions{})
	require.NoErrorf(t, err, "getting operator Deployment %s/%s", agentNamespace, operatorDeploymentName)

	var args []string
	for _, c := range dep.Spec.Template.Spec.Containers {
		if c.Name == "manager" {
			args = c.Args
		}
	}
	require.NotEmpty(t, args, "operator manager container not found or has no args")
	assert.Contains(t, args, managePrometheusCRDsArg)
}

// TestOperatorReportsPrometheusCRDsAsManaged verifies that on a fresh install the
// operator treats the CRDs it created as its own: it emitted a
// PrometheusCRDCreated Event for each and no PrometheusCRDNotManaged Event.
// Events for cluster-scoped objects are recorded in the default namespace.
// Events expire (one hour by default), so this runs right after the harness install.
func TestOperatorReportsPrometheusCRDsAsManaged(t *testing.T) {
	clientset := k8sClientset(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	events, err := clientset.CoreV1().Events("default").List(ctx, metav1.ListOptions{
		FieldSelector: "involvedObject.kind=CustomResourceDefinition",
	})
	require.NoError(t, err, "listing CRD events")

	created := map[string]bool{}
	for _, e := range events.Items {
		name := e.InvolvedObject.Name
		if name != crdName(gvrServiceMonitor) && name != crdName(gvrPodMonitor) {
			continue
		}
		switch e.Reason {
		case "PrometheusCRDCreated":
			created[name] = true
		case "PrometheusCRDNotManaged":
			t.Errorf("operator reported %s as not managed: %s", name, e.Message)
		}
	}
	for _, gvr := range []schema.GroupVersionResource{gvrServiceMonitor, gvrPodMonitor} {
		assert.Truef(t, created[crdName(gvr)], "no PrometheusCRDCreated Event for %s", crdName(gvr))
	}
}

func crdName(gvr schema.GroupVersionResource) string { return gvr.Resource + "." + gvr.Group }

// specManagers returns the field managers that own any field under spec, in order.
func specManagers(mfs []metav1.ManagedFieldsEntry) []string {
	var out []string
	for _, mf := range mfs {
		if mf.FieldsV1 == nil {
			continue
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(mf.FieldsV1.Raw, &fields); err != nil {
			out = append(out, mf.Manager)
			continue
		}
		if _, ok := fields["f:spec"]; ok {
			out = append(out, mf.Manager)
		}
	}
	return out
}
