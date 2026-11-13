//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package java_profiling

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/profilesquery"
)

const (
	namespace       = "java-profiling"
	profiledApp     = "java-profiling"
	optOutApp       = "java-profiling-opt-out"
	derivedApp      = "java-profiling-derived"
	javaContainer   = "app"
	sidecar         = "sidecar"
	workloadFrame   = "JavaProfilingWorkload"
	profilerVolume  = "opentelemetry-profiler-data-java"
	profilerMount   = "/otel-profiler-data-java"
	profilerPrefix  = "OTEL_AWS_PROFILER_"
	envMemory       = "OTEL_AWS_PROFILER_MEMORY_ENABLED"
	profilerVolSize = "160Mi" // per profiled container; each pod has one

	// The node agent's Service, which the chart's OTLP endpoint and the
	// operator's Application Signals variables both point at.
	agentService = "cloudwatch-agent.amazon-cloudwatch"
	// The Application Signals port the operator injects for the SDK's traces
	// and Application Signals metrics, and the X-Ray sampling port.
	appSignalsPort = 4316
	samplingPort   = 2000
	// The chart's otlp.httpMaxRequestBodyBytes, which it passes as the
	// profiler's payload cap.
	maxPayloadBytes = "20971520"
	// The pod annotation that would set service.name before the Deployment
	// name in the chart's service identity rules.
	serviceNameAnnotation = "resource.opentelemetry.io/service.name"

	wallType         = "wall:wall:nanoseconds:wall:nanoseconds"
	allocSpaceType   = "memory:alloc_space:bytes:alloc_space:bytes"
	allocObjectsType = "memory:alloc_objects:count:alloc_objects:count"
	threadStateLabel = "@sample.thread.state"

	// The profiler uploads every 60 s and a profile is queryable 1-2 minutes
	// later; allow for JVM start and the operator's first injection as well.
	firstProfileTimeout = 15 * time.Minute
	queryTimeout        = 5 * time.Minute
	pollInterval        = 30 * time.Second
)

// sdkRoutingPrefixes select the variables that route the SDK's own telemetry:
// its OTLP exporter and Application Signals settings. Profiling must leave
// every one of them as the operator injects it without profiling.
var sdkRoutingPrefixes = []string{
	"OTEL_EXPORTER_OTLP_",
	"OTEL_AWS_APPLICATION_SIGNALS_",
	"OTEL_AWS_APP_SIGNALS_",
	"OTEL_AWS_OTLP_",
	"OTEL_TRACES_",
	"OTEL_METRICS_EXPORTER",
	"OTEL_LOGS_EXPORTER",
}

// resourceLabelKeys are the Kubernetes resource attributes the agent's
// profiles pipeline sets, which every profile must carry as labels.
var resourceLabelKeys = []string{"k8s.cluster.name", "k8s.namespace.name", "k8s.workload.name"}

func TestJavaProfiling(t *testing.T) {
	ctx := context.Background()
	profiled := runningPod(ctx, t, profiledApp)
	optOut := runningPod(ctx, t, optOutApp)
	derived := runningPod(ctx, t, derivedApp)
	derivedDeployment := deploymentOf(ctx, t, derived)

	// The chart passes its OTLP HTTP endpoint, on otlp.http.port, to the operator.
	wantEndpoint := fmt.Sprintf("http://%s:%d/v1development/profiles", agentService, otlpHTTPPort)

	t.Run("InjectedEnv", func(t *testing.T) {
		for _, pod := range []*corev1.Pod{profiled, derived} {
			env := literalEnv(container(t, pod, javaContainer))
			assertJavaAgent(t, env)
			assert.Equal(t, "true", env["OTEL_AWS_PROFILER_ENABLED"], pod.Name)
			assert.Equal(t, wantEndpoint, env["OTEL_AWS_PROFILER_ENDPOINT"], pod.Name)
			assert.Equal(t, maxPayloadBytes, env["OTEL_AWS_PROFILER_MAX_PAYLOAD_BYTES"], pod.Name)
			assert.Equal(t, profilerMount+"/"+javaContainer, env["OTEL_AWS_PROFILER_DATA_DIR"], pod.Name)
			// "true" is injected by the operator; "false" is the workload's own
			// value, which the operator must not override.
			assert.Equal(t, string(expectMemory), env[envMemory], "%s: %s", pod.Name, envMemory)
		}
	})

	t.Run("SDKRoutingUnchanged", func(t *testing.T) {
		// The opted-out container is instrumented by the same operator and
		// Instrumentation, without the profiler: its variables are what the
		// operator injects without profiling.
		ref := sdkRoutingEnv(container(t, optOut, javaContainer))
		// Both containers are also pinned to the operator's Java variables, so a
		// change that hit both the same way would still fail. The OTLP scheme is
		// https when the agent's Application Signals config has TLS; the runtime
		// metrics switch follows the chart's java.runtime_metrics.
		scheme, _, ok := strings.Cut(ref["OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"], "://")
		require.True(t, ok && (scheme == "http" || scheme == "https"),
			"opted-out container: OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=%q", ref["OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"])
		runtime := ref["OTEL_AWS_APPLICATION_SIGNALS_RUNTIME_ENABLED"]
		assert.Contains(t, []string{"true", "false"}, runtime, "opted-out container: OTEL_AWS_APPLICATION_SIGNALS_RUNTIME_ENABLED")
		otlp := fmt.Sprintf("%s://%s:%d", scheme, agentService, appSignalsPort)
		want := map[string]string{
			"OTEL_EXPORTER_OTLP_PROTOCOL":                    "http/protobuf",
			"OTEL_METRICS_EXPORTER":                          "none",
			"OTEL_LOGS_EXPORTER":                             "none",
			"OTEL_AWS_APP_SIGNALS_ENABLED":                   "true",
			"OTEL_AWS_APPLICATION_SIGNALS_ENABLED":           "true",
			"OTEL_TRACES_SAMPLER":                            "xray",
			"OTEL_TRACES_SAMPLER_ARG":                        fmt.Sprintf("endpoint=http://%s:%d", agentService, samplingPort),
			"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT":             otlp + "/v1/traces",
			"OTEL_AWS_APP_SIGNALS_EXPORTER_ENDPOINT":         otlp + "/v1/metrics",
			"OTEL_AWS_APPLICATION_SIGNALS_EXPORTER_ENDPOINT": otlp + "/v1/metrics",
			"OTEL_AWS_APPLICATION_SIGNALS_RUNTIME_ENABLED":   runtime,
			"OTEL_AWS_OTLP_LOGS_ENDPOINT":                    otlp + "/v1/logs",
			"OTEL_AWS_OTLP_METRICS_ENDPOINT":                 otlp + "/v1/metrics",
		}
		assert.Equal(t, want, ref, "opted-out container: SDK routing variables")
		for _, pod := range []*corev1.Pod{profiled, derived} {
			got := sdkRoutingEnv(container(t, pod, javaContainer))
			assert.Equal(t, ref, got, "%s: SDK routing variables differ from the unprofiled container's", pod.Name)
			// No catch-all or profiles endpoint for the SDK: the profiler has its own.
			assert.NotContains(t, got, "OTEL_EXPORTER_OTLP_ENDPOINT", pod.Name)
			assert.NotContains(t, got, "OTEL_EXPORTER_OTLP_PROFILES_ENDPOINT", pod.Name)
		}
		t.Logf("SDK routing variables: %v", ref)
	})

	t.Run("Volume", func(t *testing.T) {
		var vol *corev1.Volume
		for i := range profiled.Spec.Volumes {
			if profiled.Spec.Volumes[i].Name == profilerVolume {
				vol = &profiled.Spec.Volumes[i]
			}
		}
		require.NotNil(t, vol, "pod has no %s volume", profilerVolume)
		require.NotNil(t, vol.EmptyDir, "%s is not an emptyDir", profilerVolume)
		require.NotNil(t, vol.EmptyDir.SizeLimit, "%s has no sizeLimit", profilerVolume)
		assert.Zero(t, vol.EmptyDir.SizeLimit.Cmp(resource.MustParse(profilerVolSize)),
			"%s sizeLimit is %s, want %s", profilerVolume, vol.EmptyDir.SizeLimit, profilerVolSize)
		assert.Equal(t, profilerMount, mountPath(container(t, profiled, javaContainer), profilerVolume))
	})

	t.Run("NotInstrumentedContainerGetsNothing", func(t *testing.T) {
		c := container(t, profiled, sidecar)
		assert.Empty(t, profilerEnv(c))
		assert.Empty(t, mountPath(c, profilerVolume))
	})

	t.Run("OptedOutContainerGetsNothing", func(t *testing.T) {
		app := container(t, optOut, javaContainer)
		assertJavaAgent(t, literalEnv(app))
		assert.Equal(t, map[string]string{"OTEL_AWS_PROFILER_ENABLED": "false"}, profilerEnv(app))
		assert.Empty(t, mountPath(app, profilerVolume))
		for _, v := range optOut.Spec.Volumes {
			assert.NotEqual(t, profilerVolume, v.Name, "opted-out pod has the profiler volume")
		}
	})

	t.Run("DerivedServiceName", func(t *testing.T) {
		// No OTEL_SERVICE_NAME in the manifest and no service.name annotation,
		// so the service identity rules give the Deployment name, and the
		// operator injects that name for the SDK (and so for Application Signals).
		_, annotated := derived.Annotations[serviceNameAnnotation]
		require.False(t, annotated, "the derived workload must not have the %s annotation", serviceNameAnnotation)
		assert.Equal(t, derivedDeployment, literalEnv(container(t, derived, javaContainer))["OTEL_SERVICE_NAME"],
			"operator-injected OTEL_SERVICE_NAME")
	})

	if t.Failed() {
		return
	}

	service := literalEnv(container(t, profiled, javaContainer))["OTEL_SERVICE_NAME"]
	optOutService := literalEnv(container(t, optOut, javaContainer))["OTEL_SERVICE_NAME"]
	require.NotEmpty(t, service)
	require.NotEmpty(t, optOutService)
	require.NotEqual(t, service, optOutService)
	require.NotEqual(t, service, derivedDeployment)
	selector := serviceSelector(service)
	derivedSelector := serviceSelector(derivedDeployment)
	// Look from just before the pods started, so profiles and traces of an
	// earlier run of the same service name cannot satisfy the checks.
	start := profiled.CreationTimestamp.Add(-time.Minute)
	derivedStart := derived.CreationTimestamp.Add(-time.Minute)
	t.Logf("service=%s opt-out service=%s derived service=%s window start=%s",
		service, optOutService, derivedDeployment, start.UTC().Format(time.RFC3339))

	t.Run("WallProfileType", func(t *testing.T) {
		var types []string
		err := poll(ctx, firstProfileTimeout, func() error {
			var err error
			types, err = profiles.LabelValues(ctx, profilesquery.ProfileTypeLabel, []string{selector}, start, time.Now())
			if err != nil {
				return err
			}
			if !slices.Contains(types, wallType) {
				return fmt.Errorf("profile types for %s: %v, want %s", service, types, wallType)
			}
			return nil
		})
		require.NoError(t, err)
		t.Logf("profile types: %v", types)
	})
	if t.Failed() {
		return
	}

	t.Run("WallStacks", func(t *testing.T) {
		nonEmptyFlamegraph(ctx, t, wallType, selector, start)
	})

	t.Run("ThreadStateLabel", func(t *testing.T) {
		err := poll(ctx, queryTimeout, func() error {
			names, err := profiles.LabelNames(ctx, []string{selector}, start, time.Now())
			if err != nil {
				return err
			}
			if !slices.Contains(names, threadStateLabel) {
				return fmt.Errorf("label names for %s: %v, want %s", service, names, threadStateLabel)
			}
			values, err := profiles.LabelValues(ctx, threadStateLabel, []string{selector}, start, time.Now())
			if err != nil {
				return err
			}
			if !slices.Contains(values, "RUNNABLE") {
				return fmt.Errorf("%s values: %v, want RUNNABLE", threadStateLabel, values)
			}
			t.Logf("%s values: %v", threadStateLabel, values)
			return nil
		})
		require.NoError(t, err)
	})

	t.Run("RunnableFilteredStacks", func(t *testing.T) {
		runnable := fmt.Sprintf(`{service_name=%q,%s="RUNNABLE"}`, service, threadStateLabel)
		fg := nonEmptyFlamegraph(ctx, t, wallType, runnable, start)
		assert.True(t, slices.ContainsFunc(fg.Names, func(n string) bool { return strings.Contains(n, workloadFrame) }),
			"RUNNABLE stacks have no %s frame: %v", workloadFrame, fg.Names)
		// The filter has to narrow the result: the workload sleeps 80% of the time.
		// Compare both over one fixed window, and query the unfiltered total
		// first: data for the window that becomes queryable between the two
		// calls then lands in the filtered total, which can only make an ignored
		// filter fail, never pass.
		end := time.Now()
		unfiltered, err := profiles.SelectMergeStacktraces(ctx, wallType, selector, start, end)
		require.NoError(t, err)
		filtered, err := profiles.SelectMergeStacktraces(ctx, wallType, runnable, start, end)
		require.NoError(t, err)
		assert.Positive(t, int64(filtered.Total), "RUNNABLE total over [start, end]")
		assert.Less(t, int64(filtered.Total), int64(unfiltered.Total), "RUNNABLE total is not below the unfiltered total")
		t.Logf("wall total=%d RUNNABLE total=%d", unfiltered.Total, filtered.Total)
	})

	t.Run("AllocationProfiles", func(t *testing.T) {
		if expectMemory == memoryOff {
			// Read the types again: the first read was as soon as the wall type
			// arrived. The derived workload turns memory off as well.
			for _, sel := range []string{selector, derivedSelector} {
				got, err := profiles.LabelValues(ctx, profilesquery.ProfileTypeLabel, []string{sel}, start, time.Now())
				require.NoError(t, err)
				assert.NotContains(t, got, allocSpaceType, sel)
				assert.NotContains(t, got, allocObjectsType, sel)
			}
			return
		}
		for _, pt := range []string{allocSpaceType, allocObjectsType} {
			err := poll(ctx, queryTimeout, func() error {
				got, err := profiles.LabelValues(ctx, profilesquery.ProfileTypeLabel, []string{selector}, start, time.Now())
				if err != nil {
					return err
				}
				if !slices.Contains(got, pt) {
					return fmt.Errorf("profile types for %s: %v, want %s", service, got, pt)
				}
				return nil
			})
			require.NoError(t, err)
			nonEmptyFlamegraph(ctx, t, pt, selector, start)
		}
	})

	t.Run("DerivedServiceProfiles", func(t *testing.T) {
		// The profile's service_name is the name the service identity rules give
		// the workload, the Deployment name. Wait for the series first, as for
		// the profiled service, before querying its stacks.
		err := poll(ctx, queryTimeout, func() error {
			got, err := profiles.LabelValues(ctx, profilesquery.ProfileTypeLabel, []string{derivedSelector}, derivedStart, time.Now())
			if err != nil {
				return err
			}
			if !slices.Contains(got, wallType) {
				return fmt.Errorf("profile types for %s: %v, want %s", derivedDeployment, got, wallType)
			}
			return nil
		})
		require.NoError(t, err)
		nonEmptyFlamegraph(ctx, t, wallType, derivedSelector, derivedStart)
	})

	t.Run("ResourceLabels", func(t *testing.T) {
		for _, c := range []struct {
			selector, workload string
			start              time.Time
		}{
			{selector, profiledApp, start},
			{derivedSelector, derivedDeployment, derivedStart},
		} {
			want := map[string]string{
				"k8s.cluster.name":   clusterName,
				"k8s.namespace.name": namespace,
				"k8s.workload.name":  c.workload,
			}
			err := poll(ctx, queryTimeout, func() error {
				names, err := profiles.LabelNames(ctx, []string{c.selector}, c.start, time.Now())
				if err != nil {
					return err
				}
				for _, key := range resourceLabelKeys {
					label, ok := resourceLabel(names, key)
					if !ok {
						return fmt.Errorf("label names for %s: %v, want %s", c.selector, names, key)
					}
					values, err := profiles.LabelValues(ctx, label, []string{c.selector}, c.start, time.Now())
					if err != nil {
						return err
					}
					if !slices.Equal(values, []string{want[key]}) {
						return fmt.Errorf("%s values for %s: %v, want [%s]", label, c.selector, values, want[key])
					}
				}
				t.Logf("label names for %s: %v", c.selector, names)
				return nil
			})
			assert.NoError(t, err)
		}
	})

	t.Run("TracesReachCloudWatch", func(t *testing.T) {
		// The SDK's traces still reach CloudWatch through Application Signals,
		// under the same service names as the profiles.
		for _, c := range []struct {
			service string
			start   time.Time
		}{
			{service, start},
			{derivedDeployment, derivedStart},
		} {
			filter := awsservice.FilterExpression(map[string]interface{}{"aws_local_service": c.service})
			err := poll(ctx, queryTimeout, func() error {
				ids, err := awsservice.GetTraceIDs(c.start, time.Now(), filter)
				if err != nil {
					return err
				}
				if len(ids) == 0 {
					return fmt.Errorf("no traces for %s", filter)
				}
				t.Logf("%s: %d traces", filter, len(ids))
				return nil
			})
			assert.NoError(t, err)
		}
	})

	t.Run("OptedOutServiceHasNoProfiles", func(t *testing.T) {
		// By now the profiled service's uploads have arrived, so the opted-out
		// service, started at the same time, would have some too.
		got, err := profiles.LabelValues(ctx, profilesquery.ProfileTypeLabel,
			[]string{serviceSelector(optOutService)}, optOut.CreationTimestamp.Add(-time.Minute), time.Now())
		require.NoError(t, err)
		assert.Empty(t, got, "opted-out service %s has profiles", optOutService)
	})
}

func serviceSelector(service string) string {
	return fmt.Sprintf(`{service_name=%q}`, service)
}

// resourceLabel returns the label name that carries resource attribute key in
// names: the attribute name itself, with an @resource. prefix like the @sample.
// labels of sample attributes, or with underscores like service_name.
func resourceLabel(names []string, key string) (string, bool) {
	for _, name := range []string{key, "@resource." + key, strings.ReplaceAll(key, ".", "_")} {
		if slices.Contains(names, name) {
			return name, true
		}
	}
	return "", false
}

// nonEmptyFlamegraph polls until the merged flame graph of profileType for
// selector has samples and frames below the root.
func nonEmptyFlamegraph(ctx context.Context, t *testing.T, profileType, selector string, start time.Time) *profilesquery.Flamegraph {
	t.Helper()
	var fg *profilesquery.Flamegraph
	err := poll(ctx, queryTimeout, func() error {
		var err error
		fg, err = profiles.SelectMergeStacktraces(ctx, profileType, selector, start, time.Now())
		if err != nil {
			return err
		}
		if fg.Total <= 0 || fg.Frames() == 0 {
			return fmt.Errorf("%s %s: total=%d frames=%d", profileType, selector, fg.Total, fg.Frames())
		}
		return nil
	})
	require.NoError(t, err)
	t.Logf("%s %s: total=%d frames=%d", profileType, selector, fg.Total, fg.Frames())
	return fg
}

// poll calls check every pollInterval until it returns nil or timeout passes,
// and returns its last error. A request error that retrying cannot change
// (a 4xx other than 429) ends it at once.
func poll(ctx context.Context, timeout time.Duration, check func() error) error {
	deadline := time.Now().Add(timeout)
	for {
		err := check()
		if err == nil {
			return nil
		}
		var reqErr *profilesquery.RequestError
		if errors.As(err, &reqErr) {
			return err
		}
		if time.Now().Add(pollInterval).After(deadline) {
			return fmt.Errorf("after %s: %w", timeout, err)
		}
		fmt.Printf("waiting: %v\n", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
}

func runningPod(ctx context.Context, t *testing.T, app string) *corev1.Pod {
	t.Helper()
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=" + app,
		FieldSelector: "status.phase=Running",
	})
	require.NoError(t, err)
	// A pod a rollout restart replaced can still be Running while it terminates.
	var live []*corev1.Pod
	for i := range pods.Items {
		if pods.Items[i].DeletionTimestamp == nil {
			live = append(live, &pods.Items[i])
		}
	}
	require.Len(t, live, 1, "want one running %s pod", app)
	return live[0]
}

// deploymentOf returns the name of the Deployment that owns pod, through its
// ReplicaSet.
func deploymentOf(ctx context.Context, t *testing.T, pod *corev1.Pod) string {
	t.Helper()
	for _, owner := range pod.OwnerReferences {
		if owner.Kind != "ReplicaSet" {
			continue
		}
		rs, err := clientset.AppsV1().ReplicaSets(pod.Namespace).Get(ctx, owner.Name, metav1.GetOptions{})
		require.NoError(t, err)
		for _, rsOwner := range rs.OwnerReferences {
			if rsOwner.Kind == "Deployment" {
				return rsOwner.Name
			}
		}
	}
	require.FailNow(t, "no Deployment owner", "pod %s is not owned by a Deployment", pod.Name)
	return ""
}

func container(t *testing.T, pod *corev1.Pod, name string) *corev1.Container {
	t.Helper()
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == name {
			return &pod.Spec.Containers[i]
		}
	}
	require.FailNow(t, "container not found", "pod %s has no container %s", pod.Name, name)
	return nil
}

// literalEnv is the container's env with literal values. The operator injects
// literal values only.
func literalEnv(c *corev1.Container) map[string]string {
	env := map[string]string{}
	for _, e := range c.Env {
		if e.ValueFrom == nil {
			env[e.Name] = e.Value
		}
	}
	return env
}

// sdkRoutingEnv is every variable of the container that sdkRoutingPrefixes
// select, a valueFrom one included (as "valueFrom").
func sdkRoutingEnv(c *corev1.Container) map[string]string {
	env := map[string]string{}
	for _, e := range c.Env {
		if !slices.ContainsFunc(sdkRoutingPrefixes, func(p string) bool { return strings.HasPrefix(e.Name, p) }) {
			continue
		}
		if e.ValueFrom != nil {
			env[e.Name] = "valueFrom"
		} else {
			env[e.Name] = e.Value
		}
	}
	return env
}

// profilerEnv is every OTEL_AWS_PROFILER_* variable the container sets, a
// valueFrom one included (with an empty value).
func profilerEnv(c *corev1.Container) map[string]string {
	env := map[string]string{}
	for _, e := range c.Env {
		if strings.HasPrefix(e.Name, profilerPrefix) {
			env[e.Name] = e.Value
		}
	}
	return env
}

func mountPath(c *corev1.Container, volume string) string {
	for _, m := range c.VolumeMounts {
		if m.Name == volume {
			return m.MountPath
		}
	}
	return ""
}

// assertJavaAgent checks that the operator auto-instrumented the container,
// so a missing profiler setting cannot come from a pod it skipped.
func assertJavaAgent(t *testing.T, env map[string]string) {
	t.Helper()
	assert.Contains(t, env["JAVA_TOOL_OPTIONS"], "-javaagent:", "container is not Java auto-instrumented")
}
