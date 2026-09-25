// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package e2e

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/environment/eksinstallationtype"
	"github.com/aws/amazon-cloudwatch-agent-test/test/e2e/utils"
)

//------------------------------------------------------------------------------
// Pod-level values plumbing test
//------------------------------------------------------------------------------
//
// The amazon-cloudwatch-observability chart accepts top-level podLabels,
// podAnnotations, topologySpreadConstraints, priorityClassName, and
// podDisruptionBudget values that flow into every supported workload.
// These constants define the values installed by applyHelmResources and the
// assertions in VerifyPodLevelValues. The intent is NOT to exhaustively test
// Kubernetes semantics of each field — only that the chart plumbs the values
// into the rendered workloads without validation errors.
//
// The values are deliberately inert so they cannot disturb the metric/log
// assertions of the suites they ride along with:
//   - labels/annotations are pure metadata
//   - the topology constraint uses whenUnsatisfiable=ScheduleAnyway
//   - the PDB uses the chart default maxUnavailable=1 on DaemonSets, which
//     never blocks normal operation
//   - priorityClassName system-cluster-critical is valid outside kube-system
//     since Kubernetes 1.17 and the chart already uses system-node-critical
//     defaults for its DaemonSets

const (
	// PodLevelTestLabelKey/Value are attached to every supported workload's pods.
	PodLevelTestLabelKey   = "e2e-pod-level-values"
	PodLevelTestLabelValue = "true"

	// PodLevelTestAnnotationKey/Value are attached to every supported workload's pods.
	PodLevelTestAnnotationKey   = "e2e.amazonaws.com/pod-level-values"
	PodLevelTestAnnotationValue = "plumbing-check"

	// PodLevelTestPriorityClass is applied to workloads without a
	// component-level priorityClassName override (operator; NOT the agent or
	// fluent-bit, which default to system-node-critical).
	PodLevelTestPriorityClass = "system-cluster-critical"

	// operatorPDBName is the PodDisruptionBudget the chart creates when
	// podDisruptionBudget.enabled is set. Only Deployment-backed workloads
	// get chart-owned PDBs; DaemonSet-backed workloads (fluent-bit,
	// node-exporter) deliberately get none — a maxUnavailable PDB targeting
	// pods of a controller without the scale subresource is permanently
	// SyncFailed with disruptionsAllowed=0, blocking eviction-API evictions.
	operatorPDBName = "amazon-cloudwatch-observability-controller-manager-pdb"

	amazonCloudWatchNamespace = "amazon-cloudwatch"
)

// PodLevelValuesHelmValues returns the helm --set values that install the
// pod-level configuration exercised by VerifyPodLevelValues.
func PodLevelValuesHelmValues() map[string]utils.HelmValue {
	return map[string]utils.HelmValue{
		"podLabels": {
			Value: fmt.Sprintf(`{"%s":"%s"}`, PodLevelTestLabelKey, PodLevelTestLabelValue),
			Type:  utils.HelmValueJSON,
		},
		"podAnnotations": {
			Value: fmt.Sprintf(`{"%s":"%s"}`, PodLevelTestAnnotationKey, PodLevelTestAnnotationValue),
			Type:  utils.HelmValueJSON,
		},
		// ScheduleAnyway = advisory only; never blocks scheduling.
		"topologySpreadConstraints": {
			Value: fmt.Sprintf(`[{"maxSkew":1,"topologyKey":"kubernetes.io/hostname","whenUnsatisfiable":"ScheduleAnyway","labelSelector":{"matchLabels":{"%s":"%s"}}}]`,
				PodLevelTestLabelKey, PodLevelTestLabelValue),
			Type: utils.HelmValueJSON,
		},
		"priorityClassName":           utils.NewHelmValue(PodLevelTestPriorityClass),
		"podDisruptionBudget.enabled": utils.NewHelmValue("true"),
	}
}

// VerifyPodLevelValues asserts the pod-level values installed by
// applyHelmResources were plumbed through the chart into the workloads.
// Only the HELM_CHART installation type is verified: the EKS addon schema
// does not expose these fields yet (tracked as a follow-up), so the addon
// path skips.
func VerifyPodLevelValues(t *testing.T, clientset *kubernetes.Clientset, env *environment.MetaData) {
	if env.EKSInstallationType != eksinstallationtype.HELM_CHART {
		t.Skipf("pod-level values are only installed on the HELM_CHART path (installation type: %s)", env.EKSInstallationType)
	}

	ctx := context.TODO()

	// ── Operator (controller-manager) Deployment — receives all five values ──
	operatorPods, err := clientset.CoreV1().Pods(amazonCloudWatchNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "control-plane=controller-manager",
	})
	require.NoError(t, err, "listing operator pods")
	require.NotEmpty(t, operatorPods.Items, "no operator pods found")

	operatorPod := operatorPods.Items[0]
	require.Equal(t, PodLevelTestLabelValue, operatorPod.Labels[PodLevelTestLabelKey],
		"podLabels not plumbed to operator pod")
	require.Equal(t, PodLevelTestAnnotationValue, operatorPod.Annotations[PodLevelTestAnnotationKey],
		"podAnnotations not plumbed to operator pod")
	require.Equal(t, PodLevelTestPriorityClass, operatorPod.Spec.PriorityClassName,
		"priorityClassName not plumbed to operator pod")
	require.NotEmpty(t, operatorPod.Spec.TopologySpreadConstraints,
		"topologySpreadConstraints not plumbed to operator pod")
	require.Equal(t, "kubernetes.io/hostname", operatorPod.Spec.TopologySpreadConstraints[0].TopologyKey,
		"unexpected topologySpreadConstraint on operator pod")

	// ── Fluent Bit DaemonSet — labels/annotations (keeps its own
	//    system-node-critical priorityClassName default) ──
	fluentBitPods, err := clientset.CoreV1().Pods(amazonCloudWatchNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "k8s-app=fluent-bit",
	})
	require.NoError(t, err, "listing fluent-bit pods")
	require.NotEmpty(t, fluentBitPods.Items, "no fluent-bit pods found")

	fluentBitPod := fluentBitPods.Items[0]
	require.Equal(t, PodLevelTestLabelValue, fluentBitPod.Labels[PodLevelTestLabelKey],
		"podLabels not plumbed to fluent-bit pod")
	require.Equal(t, PodLevelTestAnnotationValue, fluentBitPod.Annotations[PodLevelTestAnnotationKey],
		"podAnnotations not plumbed to fluent-bit pod")
	// Component-level default must win over the root value.
	require.Equal(t, "system-node-critical", fluentBitPod.Spec.PriorityClassName,
		"fluent-bit component priorityClassName default should not be overridden by the root value")

	// ── CloudWatch Agent DaemonSet — podAnnotations via the
	//    AmazonCloudWatchAgent CR.
	//
	//    Deliberately NOT asserted here:
	//      - podLabels: requires operator v3.8.0+ (CRD field) plus chart-side
	//        wiring — add once released (follow-up).
	//      - topologySpreadConstraints: schema-accepted today but silently
	//        dropped by the operator for daemonset-mode agents until
	//        operator v3.8.0+ — add once released (follow-up).
	//      - podDisruptionBudget: the operator NEVER emits a PDB for
	//        daemonset-mode agents, by design (its defaulting webhook
	//        populates the field on every CR, so daemonset emission would
	//        create surprise PDBs on upgrade). Do not add a CWA PDB
	//        assertion — it can never pass for the default daemonset agent. ──
	agentPods, err := clientset.CoreV1().Pods(amazonCloudWatchNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/component=amazon-cloudwatch-agent",
	})
	require.NoError(t, err, "listing cloudwatch-agent pods")
	require.NotEmpty(t, agentPods.Items, "no cloudwatch-agent pods found")

	agentPod := agentPods.Items[0]
	require.Equal(t, PodLevelTestAnnotationValue, agentPod.Annotations[PodLevelTestAnnotationKey],
		"podAnnotations not plumbed through the AmazonCloudWatchAgent CR to agent pod")

	// ── PodDisruptionBudget — created by the chart when
	//    podDisruptionBudget.enabled=true (Deployment-backed workloads only;
	//    the chart deliberately emits no PDBs for DaemonSet workloads) ──
	pdb, pdbErr := clientset.PolicyV1().PodDisruptionBudgets(amazonCloudWatchNamespace).Get(ctx, operatorPDBName, metav1.GetOptions{})
	require.NoError(t, pdbErr, "getting PodDisruptionBudget %s", operatorPDBName)
	require.NotNil(t, pdb.Spec.MaxUnavailable, "PodDisruptionBudget %s missing maxUnavailable", operatorPDBName)
	require.Equal(t, 1, pdb.Spec.MaxUnavailable.IntValue(),
		"PodDisruptionBudget %s should carry the chart default maxUnavailable", operatorPDBName)
	// The PDB must actually sync and select pods — a PDB that exists but is
	// SyncFailed (e.g. maxUnavailable against a scale-less controller) has
	// disruptionsAllowed=0 and blocks evictions.
	require.Greater(t, pdb.Status.ExpectedPods, int32(0),
		"PodDisruptionBudget %s did not sync (expectedPods=0)", operatorPDBName)

	// No chart-owned PDBs for DaemonSet workloads.
	for _, name := range []string{"fluent-bit-pdb", "node-exporter-pdb"} {
		_, dsPdbErr := clientset.PolicyV1().PodDisruptionBudgets(amazonCloudWatchNamespace).Get(ctx, name, metav1.GetOptions{})
		require.Error(t, dsPdbErr, "unexpected PodDisruptionBudget %s for a DaemonSet workload", name)
	}
}
