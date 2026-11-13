//go:build integration

// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package java_profiling

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/aws/amazon-cloudwatch-agent-test/util/profilesquery"
)

const (
	namespace       = "java-profiling"
	profiledApp     = "java-profiling"
	optOutApp       = "java-profiling-opt-out"
	javaContainer   = "app"
	sidecar         = "sidecar"
	workloadFrame   = "JavaProfilingWorkload"
	profilerVolume  = "opentelemetry-profiler-data-java"
	profilerMount   = "/otel-profiler-data-java"
	profilerPrefix  = "OTEL_AWS_PROFILER_"
	envMemory       = "OTEL_AWS_PROFILER_MEMORY_ENABLED"
	profilerVolSize = "160Mi" // per profiled container; this pod has one

	// The chart's OTLP HTTP endpoint on the node agent (otlp.http.port).
	wantEndpoint = "http://cloudwatch-agent.amazon-cloudwatch:4318/v1development/profiles"

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

func TestJavaProfiling(t *testing.T) {
	ctx := context.Background()
	profiled := runningPod(ctx, t, profiledApp)
	optOut := runningPod(ctx, t, optOutApp)

	var memoryInjected bool
	t.Run("InjectedEnv", func(t *testing.T) {
		app := container(t, profiled, javaContainer)
		env := literalEnv(app)
		assertJavaAgent(t, env)
		assert.Equal(t, "true", env["OTEL_AWS_PROFILER_ENABLED"])
		assert.Equal(t, wantEndpoint, env["OTEL_AWS_PROFILER_ENDPOINT"])
		maxPayload, err := strconv.Atoi(env["OTEL_AWS_PROFILER_MAX_PAYLOAD_BYTES"])
		if assert.NoError(t, err, "OTEL_AWS_PROFILER_MAX_PAYLOAD_BYTES") {
			assert.Positive(t, maxPayload, "OTEL_AWS_PROFILER_MAX_PAYLOAD_BYTES")
		}
		assert.Equal(t, profilerMount+"/"+javaContainer, env["OTEL_AWS_PROFILER_DATA_DIR"])

		memory, ok := env[envMemory]
		memoryInjected = ok
		switch expectMemory {
		case memoryOn:
			assert.Equal(t, "true", memory, envMemory)
		case memoryOff:
			// The workload sets "false"; the operator must not override it.
			assert.Equal(t, "false", memory, envMemory)
		default:
			if ok {
				assert.Equal(t, "true", memory, envMemory)
			}
		}
		t.Logf("%s injected=%v value=%q", envMemory, ok, memory)
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

	if t.Failed() {
		return
	}

	service := literalEnv(container(t, profiled, javaContainer))["OTEL_SERVICE_NAME"]
	optOutService := literalEnv(container(t, optOut, javaContainer))["OTEL_SERVICE_NAME"]
	require.NotEmpty(t, service)
	require.NotEmpty(t, optOutService)
	require.NotEqual(t, service, optOutService)
	selector := fmt.Sprintf(`{service_name=%q}`, service)
	// Look from just before the pod started, so profiles of an earlier run of
	// the same service name cannot satisfy the checks.
	start := profiled.CreationTimestamp.Add(-time.Minute)
	t.Logf("service=%s opt-out service=%s window start=%s", service, optOutService, start.UTC().Format(time.RFC3339))

	var types []string
	t.Run("WallProfileType", func(t *testing.T) {
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
		switch {
		case expectMemory == memoryOff:
			// Read the types again: the first read was as soon as the wall type arrived.
			got, err := profiles.LabelValues(ctx, profilesquery.ProfileTypeLabel, []string{selector}, start, time.Now())
			require.NoError(t, err)
			assert.NotContains(t, got, allocSpaceType)
			assert.NotContains(t, got, allocObjectsType)
			return
		case expectMemory == memoryAuto && !memoryInjected:
			t.Skipf("%s is not injected by this operator", envMemory)
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

	t.Run("OptedOutServiceHasNoProfiles", func(t *testing.T) {
		// By now the profiled service's uploads have arrived, so the opted-out
		// service, started at the same time, would have some too.
		got, err := profiles.LabelValues(ctx, profilesquery.ProfileTypeLabel,
			[]string{fmt.Sprintf(`{service_name=%q}`, optOutService)}, optOut.CreationTimestamp.Add(-time.Minute), time.Now())
		require.NoError(t, err)
		assert.Empty(t, got, "opted-out service %s has profiles", optOutService)
	})
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
