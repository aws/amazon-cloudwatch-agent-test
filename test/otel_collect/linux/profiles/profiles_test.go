// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package profiles

// The *_unit_test.go files and the helper Test functions in this package are plain unit tests:
// they use no AWS and no running agent, and they guard the check logic the integration tests
// depend on. Run them locally with:
//
//	go test -run Test -skip 'TestProfiles$|TestProfilesAttributeCap$' ./test/otel_collect/linux/profiles/
//
// The two skipped tests are the integration entry points, which need the agent and an EC2 host.

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

const (
	profilesRuntime    = 3 * time.Minute
	otlpHTTPAddr       = "127.0.0.1:4318"
	profilesService    = "cwagent-integ-test-profiles"
	translatedYamlPath = "/opt/aws/amazon-cloudwatch-agent/etc/amazon-cloudwatch-agent.yaml"

	// exportSettleTime lets the last pushes leave the exporter queue before self telemetry is read.
	exportSettleTime = 15 * time.Second

	// pushWindowTail is how long before the agent stops the regular pushes end. It leaves room for
	// the baseline counter read, the payload without service.name, and the final counter read.
	pushWindowTail = 50 * time.Second

	// otherSignalsStartTick is the push round before which the fail-open check reads its starting
	// counters. Earlier rounds give the metrics batch processor (10s timeout) time to export once.
	otherSignalsStartTick = 3
)

type ProfilesTestRunner struct {
	test_runner.BaseTestRunner
	env *environment.MetaData

	mu                sync.Mutex
	pushAttempts      int
	pushAccepted      int
	pushFailures      []string
	transportAccepted map[string]int

	metricsExporter  string
	metricsConfigErr error
	metricsAccepted  int
	metricsFailures  []string
	otherStart       otherSignalsSnapshot
	otherEnd         otherSignalsSnapshot

	// serviceName is unique per run, so each run's pushes carry a distinct service.name and do
	// not collide with a previous run's data.
	serviceName string
	pushStart   time.Time

	exporterCounts exporterCounts
	exporterErr    error
	exporterRead   bool

	// Counters read just before and after the single payload that has no service.name. The
	// difference isolates that payload from the regular pushes.
	defaultNameBefore exporterCounts
	defaultNameAfter  exporterCounts
	defaultNameErr    error
	defaultNameRead   bool
}

var _ test_runner.ITestRunner = (*ProfilesTestRunner)(nil)

func (t *ProfilesTestRunner) GetTestName() string                { return "OtelCollectProfiles" }
func (t *ProfilesTestRunner) GetAgentRunDuration() time.Duration { return profilesRuntime }
func (t *ProfilesTestRunner) GetAgentConfigFileName() string     { return "profiles_config.json" }
func (t *ProfilesTestRunner) GetMeasuredMetrics() []string       { return nil }

func (t *ProfilesTestRunner) SetupBeforeAgentRun() error {
	resetAgentLog()
	return t.SetUpConfig()
}

func (t *ProfilesTestRunner) SetupAfterAgentRun() error {
	go t.pushProfiles()
	return nil
}

func (t *ProfilesTestRunner) pushProfiles() {
	// The deadline is fixed before waiting for the port, so a slow agent start shortens the push
	// window instead of pushing the final counter reads past the agent's stop.
	deadline := time.Now().Add(profilesRuntime - pushWindowTail)
	for _, addr := range []string{otlpHTTPAddr, otlpGRPCAddr} {
		if err := common.WaitForTCPPort(addr, 2*time.Minute); err != nil {
			t.recordFailure(fmt.Sprintf("OTLP port %s never became ready: %v", addr, err))
			return
		}
	}
	t.findMetricsExporter()

	t.mu.Lock()
	t.pushStart = time.Now()
	t.mu.Unlock()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	timeout := time.After(time.Until(deadline))

	for tick := 1; ; tick++ {
		select {
		case <-timeout:
			t.readExporterCounts()
			t.pushWithoutServiceName()
			return
		case <-ticker.C:
			if tick == otherSignalsStartTick {
				t.mu.Lock()
				metricsExporter := t.metricsExporter
				t.mu.Unlock()
				start := readOtherSignals(metricsExporter)
				t.mu.Lock()
				t.otherStart = start
				t.mu.Unlock()
			}
			t.pushOnce()
		}
	}
}

func (t *ProfilesTestRunner) findMetricsExporter() {
	yamlContent, err := common.RunCommand("sudo cat " + translatedYamlPath)
	var id string
	if err == nil {
		id, err = metricsExporterID([]byte(yamlContent))
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.metricsExporter, t.metricsConfigErr = id, err
}

// Scrape and systemctl read run with no lock held; the caller reads t.metricsExporter under t.mu,
// calls this, then stores the result under t.mu.
func readOtherSignals(metricsExporter string) otherSignalsSnapshot {
	if metricsExporter == "" {
		return otherSignalsSnapshot{}
	}
	counts, err := scrapeExporterCounts(metricsExporter, sentMetricPointsMetric, sendFailedMetricPointsMetric)
	return otherSignalsSnapshot{counts: counts, err: err, pid: agentPID(), read: true}
}

// readExporterCounts snapshots the exporter's delivery counters while the agent is still
// running, because the runner stops the agent before Validate and self telemetry goes with it.
// It scrapes first and takes t.mu only to read the metrics exporter id and to store the results,
// so no HTTP scrape runs while the lock is held.
func (t *ProfilesTestRunner) readExporterCounts() {
	time.Sleep(exportSettleTime)
	counts, err := scrapeProfilesExporterCounts()

	t.mu.Lock()
	metricsExporter := t.metricsExporter
	t.mu.Unlock()

	other := readOtherSignals(metricsExporter)

	t.mu.Lock()
	defer t.mu.Unlock()
	t.exporterCounts, t.exporterErr, t.exporterRead = counts, err, true
	t.otherEnd = other
}

// pushWithoutServiceName sends one payload with no service.name. The backend rejects a resource
// without one, so this payload is only delivered if the agent's resource/profiles processor
// inserted the default (unknown_service). The counters read before and after it isolate its
// outcome from the regular pushes, which all carry a service name.
func (t *ProfilesTestRunner) pushWithoutServiceName() {
	t.mu.Lock()
	before, beforeErr := t.exporterCounts, t.exporterErr
	t.mu.Unlock()

	err := beforeErr
	var after exporterCounts
	if err == nil {
		err = postProfiles(buildProfilesPayload("", t.env.InstanceId))
	}
	if err == nil {
		time.Sleep(exportSettleTime)
		after, err = scrapeProfilesExporterCounts()
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	t.defaultNameBefore, t.defaultNameAfter, t.defaultNameErr, t.defaultNameRead = before, after, err, true
}

func (t *ProfilesTestRunner) pushOnce() {
	for _, tr := range profilesTransports {
		t.mu.Lock()
		t.pushAttempts++
		t.mu.Unlock()

		if err := tr.send(t.serviceName+tr.serviceSuffix, t.env.InstanceId); err != nil {
			t.recordFailure(fmt.Sprintf("%s: %v", tr.name, err))
			continue
		}

		t.mu.Lock()
		t.pushAccepted++
		if t.transportAccepted == nil {
			t.transportAccepted = map[string]int{}
		}
		t.transportAccepted[tr.name]++
		t.mu.Unlock()
	}

	err := postMetrics(buildMetricsPayload(t.serviceName))
	t.mu.Lock()
	defer t.mu.Unlock()
	if err != nil {
		t.metricsFailures = append(t.metricsFailures, err.Error())
		return
	}
	t.metricsAccepted++
}

func (t *ProfilesTestRunner) recordFailure(msg string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pushFailures = append(t.pushFailures, msg)
}

// Runs every check and logs each outcome so one failure never hides the others.
func (t *ProfilesTestRunner) Validate() status.TestGroupResult {
	results := []status.TestResult{
		t.validateTranslatedConfig(),
		t.validatePipelineInvariants(),
		t.validateReceiverAcceptance(),
		t.validateExporterHealth(),
		t.validateExporterDelivery(),
		t.validateDefaultServiceName(),
		t.validateOtherSignalsUnaffected(),
	}
	for _, r := range results {
		log.Printf("[Profiles] check %s: %s %v", r.Name, r.Status, reasonText(r))
	}
	return status.TestGroupResult{Name: t.GetTestName(), TestResults: results}
}

func reasonText(r status.TestResult) string {
	if r.Reason == nil {
		return ""
	}
	return "- " + r.Reason.Error()
}

func (t *ProfilesTestRunner) validateTranslatedConfig() status.TestResult {
	result := status.TestResult{Name: "Profiles_TranslatedConfig", Status: status.FAILED}

	yamlContent, err := common.RunCommand("sudo cat " + translatedYamlPath)
	if err != nil {
		result.Reason = fmt.Errorf("failed to read translated agent config %s: %w", translatedYamlPath, err)
		return result
	}

	for _, want := range []string{profilesExporterID, profilesURLPath} {
		if !strings.Contains(yamlContent, want) {
			result.Reason = fmt.Errorf("translated config %s does not contain %q: the agent build does not generate a profiles pipeline from the opentelemetry otlp section", translatedYamlPath, want)
			return result
		}
	}

	result.Status = status.SUCCESSFUL
	return result
}

// See checkProfilesPipelineInvariants for the invariants and why they matter.
func (t *ProfilesTestRunner) validatePipelineInvariants() status.TestResult {
	result := status.TestResult{Name: "Profiles_PipelineInvariants", Status: status.FAILED}

	yamlContent, err := common.RunCommand("sudo cat " + translatedYamlPath)
	if err != nil {
		result.Reason = fmt.Errorf("failed to read translated agent config %s: %w", translatedYamlPath, err)
		return result
	}
	p, err := parseProfilesPipeline([]byte(yamlContent))
	if err == nil {
		err = checkProfilesPipelineInvariants(p)
	}
	if err != nil {
		result.Reason = err
		return result
	}
	log.Printf("[Profiles] %s processors: %v", profilesPipelineID, p.Processors)
	result.Status = status.SUCCESSFUL
	return result
}

func (t *ProfilesTestRunner) validateReceiverAcceptance() status.TestResult {
	t.mu.Lock()
	defer t.mu.Unlock()

	result := status.TestResult{Name: "Profiles_ReceiverAcceptance", Status: status.FAILED}
	log.Printf("[Profiles] push attempts: %d, accepted: %d (%v), failures: %d", t.pushAttempts, t.pushAccepted, t.transportAccepted, len(t.pushFailures))

	if len(t.pushFailures) > 0 {
		result.Reason = fmt.Errorf("profile pushes failed: %s", strings.Join(t.pushFailures, "; "))
		return result
	}
	if err := missingTransports(t.transportAccepted); err != nil {
		result.Reason = err
		return result
	}

	result.Status = status.SUCCESSFUL
	return result
}

func (t *ProfilesTestRunner) validateExporterHealth() status.TestResult {
	result := status.TestResult{Name: "Profiles_ExporterHealth", Status: status.FAILED}

	logContent, err := common.RunCommand("sudo cat " + common.AgentLogFile)
	if err != nil {
		result.Reason = fmt.Errorf("failed to read agent log %s: %w", common.AgentLogFile, err)
		return result
	}
	if len(logContent) == 0 {
		result.Reason = fmt.Errorf("agent log %s is empty", common.AgentLogFile)
		return result
	}

	var offending []string
	collectorReady := false
	for _, line := range strings.Split(logContent, "\n") {
		if strings.Contains(line, "Everything is ready. Begin running and processing data.") {
			collectorReady = true
		}
		if profilesExporterLogProblem(line) {
			offending = append(offending, line)
		}
	}

	if !collectorReady {
		result.Reason = fmt.Errorf("agent log never reported the collector as ready")
		return result
	}
	if len(offending) > 0 {
		result.Reason = fmt.Errorf("agent log shows profiles export problems:\n%s", strings.Join(offending, "\n"))
		return result
	}

	result.Status = status.SUCCESSFUL
	return result
}

// The receiver's 200 cannot show delivery, because the exporter queue decouples the receiver
// response from the export outcome; these counters show whether the backend accepted samples.
func (t *ProfilesTestRunner) validateExporterDelivery() status.TestResult {
	t.mu.Lock()
	defer t.mu.Unlock()

	result := status.TestResult{Name: "Profiles_ExporterDelivery", Status: status.FAILED}
	switch {
	case !t.exporterRead:
		result.Reason = fmt.Errorf("exporter self telemetry was never read")
	case t.exporterErr != nil:
		result.Reason = fmt.Errorf("reading exporter self telemetry: %w", t.exporterErr)
	case !t.exporterCounts.found:
		result.Reason = fmt.Errorf("self telemetry has no %s series for %s", sentProfileSamplesMetric, profilesExporterID)
	default:
		// t.exporterCounts is read before the no-service-name probe (see readExporterCounts), so
		// its counters cover the regular pushes only. The probe is bracketed by
		// defaultNameBefore/defaultNameAfter and judged by Profiles_DefaultServiceName, so a
		// probe-only failure is never reported here as the regular pushes failing.
		if err := exporterDeliveryError(t.exporterCounts, t.pushAccepted); err != nil {
			result.Reason = err
		} else {
			log.Printf("[Profiles] exporter sent %.0f samples, failed %.0f, receiver accepted %d payloads",
				t.exporterCounts.sent, t.exporterCounts.sendFailed, t.pushAccepted)
			result.Status = status.SUCCESSFUL
		}
	}
	return result
}

// counts must be the snapshot read before the no-service-name probe, so that probe's outcome is
// excluded from this regular-push judgement.
func exporterDeliveryError(counts exporterCounts, pushAccepted int) error {
	switch {
	case counts.sendFailed > 0:
		return fmt.Errorf("%s reported %.0f profile samples from the regular pushes that failed to send", profilesExporterID, counts.sendFailed)
	case counts.sent == 0:
		return fmt.Errorf("%s sent no profile samples (receiver accepted %d payloads)", profilesExporterID, pushAccepted)
	case counts.sent < float64(pushAccepted):
		// Every payload carries one sample record (value 100). The counter counts records, not
		// values, so it has to cover every accepted push.
		return fmt.Errorf("%s sent %.0f profile samples, fewer than the %d payloads the receiver accepted over all transports", profilesExporterID, counts.sent, pushAccepted)
	}
	return nil
}

// Delivery of the payload with no service.name means resource/profiles inserted the default,
// because the backend rejects a resource without one.
func (t *ProfilesTestRunner) validateDefaultServiceName() status.TestResult {
	t.mu.Lock()
	read, readErr := t.defaultNameRead, t.defaultNameErr
	before, after := t.defaultNameBefore, t.defaultNameAfter
	t.mu.Unlock()

	result := status.TestResult{Name: "Profiles_DefaultServiceName", Status: status.FAILED}
	switch {
	case !read:
		result.Reason = fmt.Errorf("payload without service.name was never sent")
	case readErr != nil:
		result.Reason = fmt.Errorf("payload without service.name: %w", readErr)
	default:
		// The log is only read when the counters show a failure, to name the export error.
		var exportFailure string
		if after.sendFailed > before.sendFailed {
			lines, err := profilesLogLines()
			if err == nil {
				exportFailure = lastExportFailure(lines)
			}
		}
		result.Reason = defaultServiceNameError(before, after, exportFailure)
		if result.Reason == nil {
			result.Status = status.SUCCESSFUL
		}
	}
	return result
}

// defaultServiceNameError judges the payload without service.name from the counters read just
// before it (which cover every regular push) and just after it. A send failure only points at
// resource/profiles when the regular pushes, which all carry a service name, were delivered
// cleanly; otherwise every export is failing and the reason is the export error itself.
func defaultServiceNameError(before, after exporterCounts, exportFailure string) error {
	sentDelta := after.sent - before.sent
	failedDelta := after.sendFailed - before.sendFailed
	if failedDelta <= 0 {
		if sentDelta <= 0 {
			return fmt.Errorf("payload without service.name: no samples sent within %s", exportSettleTime)
		}
		return nil
	}

	cause := "no failed export line in the agent log"
	if exportFailure != "" {
		cause = "last export error: " + exportFailure
	}
	if before.sent > 0 && before.sendFailed == 0 {
		return fmt.Errorf("payload without service.name: %.0f samples failed to send while the regular pushes were delivered, so resource/profiles may not have set a default service name; %s", failedDelta, cause)
	}
	return fmt.Errorf("payload without service.name: %.0f samples failed to send, as did the regular pushes (sent %.0f, failed %.0f before it), so the export itself is failing; %s", failedDelta, before.sent, before.sendFailed, cause)
}

func missingTransports(accepted map[string]int) error {
	var missing []string
	for _, tr := range profilesTransports {
		if accepted[tr.name] == 0 {
			missing = append(missing, tr.name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("no profile payloads were accepted over %s", strings.Join(missing, ", "))
	}
	return nil
}

// validateOtherSignalsUnaffected is the fail-open check: exporting profiles, successfully or
// not, must not stop the agent's other pipelines. The test pushes OTLP metrics next to the
// profiles; otherSignalsOutcome decides from the metrics exporter's counters and export errors.
// It is skipped when the translated config has no OTLP metrics exporter to watch.
func (t *ProfilesTestRunner) validateOtherSignalsUnaffected() status.TestResult {
	t.mu.Lock()
	defer t.mu.Unlock()

	result := status.TestResult{Name: "Profiles_OtherSignalsUnaffected", Status: status.FAILED}
	switch {
	case t.metricsConfigErr != nil:
		result.Reason = fmt.Errorf("reading the metrics exporter from %s: %w", translatedYamlPath, t.metricsConfigErr)
		return result
	case t.metricsExporter == "" && t.pushStart.IsZero():
		result.Reason = fmt.Errorf("the push window never started")
		return result
	case t.metricsExporter == "":
		result.Status = status.SKIPPED
		result.Reason = fmt.Errorf("skipped: the translated config has no OTLP HTTP exporter in a metrics pipeline")
		log.Printf("[Profiles] fail-open branch: skipped, no OTLP metrics exporter")
		return result
	}
	if len(t.metricsFailures) > 0 {
		result.Reason = fmt.Errorf("metrics pushes failed: %s", strings.Join(t.metricsFailures, "; "))
		return result
	}
	exportErrors, err := exporterFailureLines(t.metricsExporter)
	if err != nil {
		result.Reason = err
		return result
	}
	outcome, err := otherSignalsOutcome(t.metricsExporter, t.otherStart, t.otherEnd, t.metricsAccepted, exportErrors)
	if err != nil {
		result.Reason = err
		return result
	}
	log.Printf("[Profiles] fail-open branch: %s (%s sent %.0f, send-failed %.0f metric points during the run, pid %s)",
		outcome, t.metricsExporter, t.otherEnd.counts.sent-t.otherStart.counts.sent, t.otherEnd.counts.sendFailed-t.otherStart.counts.sendFailed, t.otherEnd.pid)
	result.Status = status.SUCCESSFUL
	if outcome != metricsDelivered {
		result.Reason = fmt.Errorf("%s", outcome)
	}
	return result
}

func TestProfiles(t *testing.T) {
	env := environment.GetEnvironmentMetaData()

	testRunner := &ProfilesTestRunner{
		BaseTestRunner: test_runner.BaseTestRunner{},
		env:            env,
		serviceName:    fmt.Sprintf("%s-%d", profilesService, time.Now().Unix()),
	}
	runner := &test_runner.TestRunner{TestRunner: testRunner}
	result := runner.Run()

	// Collect every failure before asserting, so the output names all failing checks.
	var failed []string
	for _, r := range result.TestResults {
		if r.Status == status.FAILED {
			failed = append(failed, fmt.Sprintf("%s: %v", r.Name, r.Reason))
		}
	}
	require.Empty(t, failed, "profiles checks failed:\n%s", strings.Join(failed, "\n"))
}
