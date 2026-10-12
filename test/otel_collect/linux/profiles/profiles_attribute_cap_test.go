// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package profiles

// The attribute-cap test sends a profile with more resource attributes than the backend accepts
// and checks the agent forwards it and the backend drops it. It is its own runner because the
// main profiles test treats a partial-success log line as a failure while this test expects one,
// and each runner starts the agent with a fresh log so that warning never reaches the main test.

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

const (
	capRuntime = 3 * time.Minute

	// capExtraAttributes puts the oversized resource well over the backend's 150 flattened
	// attribute limit, before the agent's own enrichment adds more.
	capExtraAttributes = 160

	// capPushInterval keeps pushes further apart than the collector's 10s log sampling window,
	// so a repeated warning or error line from each export is not dropped from the log.
	capPushInterval = 12 * time.Second

	capWarmupPushes     = 2
	capSubsequentPushes = 3

	agentUnit = "amazon-cloudwatch-agent"
)

// checkLevel says whether a failing check fails the Go test (must) or is only reported (should).
type checkLevel string

const (
	levelMust   checkLevel = "MUST"
	levelShould checkLevel = "SHOULD"
)

type capCheck struct {
	status.TestResult
	level checkLevel
}

type agentProcess struct {
	mainPID     string
	restarts    string
	activeState string
}

type ProfilesAttributeCapTestRunner struct {
	test_runner.BaseTestRunner
	env *environment.MetaData

	mu   sync.Mutex
	done bool

	warmupErrs []string

	mixedErr error

	subsequentErrs     []string
	subsequentAccepted int

	baseline, afterMixed, final exporterCounts
	countersErr                 error

	before, after agentProcess
	processErr    error

	// checks is what Validate reported, kept so the Go test can tell MUST from SHOULD.
	checks []capCheck
}

var _ test_runner.ITestRunner = (*ProfilesAttributeCapTestRunner)(nil)

func (t *ProfilesAttributeCapTestRunner) GetTestName() string {
	return "OtelCollectProfilesAttributeCap"
}
func (t *ProfilesAttributeCapTestRunner) GetAgentRunDuration() time.Duration { return capRuntime }
func (t *ProfilesAttributeCapTestRunner) GetAgentConfigFileName() string {
	return "profiles_config.json"
}
func (t *ProfilesAttributeCapTestRunner) GetMeasuredMetrics() []string { return nil }

func (t *ProfilesAttributeCapTestRunner) SetupBeforeAgentRun() error {
	resetAgentLog()
	return t.SetUpConfig()
}

func (t *ProfilesAttributeCapTestRunner) SetupAfterAgentRun() error {
	go t.run()
	return nil
}

// run is the whole push sequence. It finishes well inside capRuntime, so every counter read
// happens while the agent still runs. Results are stored once, under the lock, at the end.
func (t *ProfilesAttributeCapTestRunner) run() {
	var (
		warmupErrs, subsequentErrs []string
		mixedErr, countersErr      error
		baseline, afterMixed       exporterCounts
		final                      exporterCounts
		before, after              agentProcess
		processErr                 error
		accepted                   int
	)
	defer func() {
		t.mu.Lock()
		defer t.mu.Unlock()
		t.warmupErrs, t.mixedErr = warmupErrs, mixedErr
		t.subsequentErrs, t.subsequentAccepted = subsequentErrs, accepted
		t.baseline, t.afterMixed, t.final, t.countersErr = baseline, afterMixed, final, countersErr
		t.before, t.after, t.processErr = before, after, processErr
		t.done = true
	}()
	keepFirst := func(dst *error, err error) {
		if *dst == nil {
			*dst = err
		}
	}

	if err := common.WaitForTCPPort(otlpHTTPAddr, 90*time.Second); err != nil {
		mixedErr = fmt.Errorf("OTLP HTTP port never became ready: %w", err)
		return
	}

	validName := fmt.Sprintf("%s-cap-valid-%d", profilesService, time.Now().Unix())
	for i := 0; i < capWarmupPushes; i++ {
		if err := postProfiles(buildProfilesPayload(validName, t.env.InstanceId)); err != nil {
			warmupErrs = append(warmupErrs, err.Error())
		}
		time.Sleep(capPushInterval)
	}
	var err error
	baseline, err = t.readCounters()
	keepFirst(&countersErr, err)
	before, err = readAgentProcess()
	keepFirst(&processErr, err)

	mixedErr = postProfiles(buildAttributeCapPayload(validName, t.env.InstanceId, capExtraAttributes))
	afterMixed, err = t.readCounters()
	keepFirst(&countersErr, err)

	for i := 0; i < capSubsequentPushes; i++ {
		time.Sleep(capPushInterval)
		if err := postProfiles(buildProfilesPayload(validName, t.env.InstanceId)); err != nil {
			subsequentErrs = append(subsequentErrs, err.Error())
			continue
		}
		accepted++
	}
	final, err = t.readCounters()
	keepFirst(&countersErr, err)
	after, err = readAgentProcess()
	keepFirst(&processErr, err)
}

func (t *ProfilesAttributeCapTestRunner) readCounters() (exporterCounts, error) {
	time.Sleep(exportSettleTime)
	return scrapeProfilesExporterCounts()
}

// Mixes a valid resource with an oversized one on purpose: the backend answers 200 with a
// partial success for a mixed request, but 400 when every profile is invalid, which would look
// like a plain export failure.
func buildAttributeCapPayload(serviceName, instanceID string, extra int) []byte {
	valid := []resourceAttr{attr(serviceNameKey, serviceName), attr("instance_id", instanceID)}
	oversized := []resourceAttr{attr(serviceNameKey, serviceName+"-oversized"), attr("instance_id", instanceID)}
	for i := 0; i < extra; i++ {
		oversized = append(oversized, attr(fmt.Sprintf("integ.test.attr.%03d", i), "x"))
	}
	return buildResourceProfilesPayload(valid, oversized)
}

func readAgentProcess() (agentProcess, error) {
	out, err := common.RunCommand("systemctl show -p MainPID -p NRestarts -p ActiveState " + agentUnit)
	if err != nil {
		return agentProcess{}, fmt.Errorf("systemctl show %s: %w", agentUnit, err)
	}
	return parseAgentProcess(out), nil
}

func parseAgentProcess(out string) agentProcess {
	var p agentProcess
	for _, line := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch key {
		case "MainPID":
			p.mainPID = value
		case "NRestarts":
			p.restarts = value
		case "ActiveState":
			p.activeState = value
		}
	}
	return p
}

func (t *ProfilesAttributeCapTestRunner) Validate() status.TestGroupResult {
	checks := t.capChecks()
	results := make([]status.TestResult, 0, len(checks))
	for _, c := range checks {
		log.Printf("[ProfilesCap] check %s (%s): %s %v", c.Name, c.level, c.Status, reasonText(c.TestResult))
		results = append(results, c.TestResult)
	}
	t.mu.Lock()
	t.checks = checks
	t.mu.Unlock()
	return status.TestGroupResult{Name: t.GetTestName(), TestResults: results}
}

func (t *ProfilesAttributeCapTestRunner) capChecks() []capCheck {
	t.mu.Lock()
	defer t.mu.Unlock()

	logLines, logErr := profilesLogLines()
	checks := []capCheck{
		{t.checkMixedAccepted(), levelMust},
		{t.checkAgentStaysUp(), levelMust},
		{t.checkSubsequentDelivery(), levelMust},
		{checkPartialSuccessSurfaced(logLines, logErr), levelShould},
	}
	if t.countersErr == nil && t.done {
		log.Printf("[ProfilesCap] counters sent/failed: baseline %.0f/%.0f, after mixed %.0f/%.0f, final %.0f/%.0f",
			t.baseline.sent, t.baseline.sendFailed, t.afterMixed.sent, t.afterMixed.sendFailed, t.final.sent, t.final.sendFailed)
	}
	return checks
}

func (t *ProfilesAttributeCapTestRunner) checkMixedAccepted() status.TestResult {
	r := status.TestResult{Name: "ProfilesCap_MixedBatchAccepted", Status: status.FAILED}
	switch {
	case !t.done:
		r.Reason = fmt.Errorf("push sequence did not finish before the agent stopped")
	case len(t.warmupErrs) > 0:
		r.Reason = fmt.Errorf("warm-up pushes failed: %s", strings.Join(t.warmupErrs, "; "))
	case t.mixedErr != nil:
		r.Reason = fmt.Errorf("receiver did not accept the mixed request: %w", t.mixedErr)
	default:
		r.Status = status.SUCCESSFUL
	}
	return r
}

func (t *ProfilesAttributeCapTestRunner) checkAgentStaysUp() status.TestResult {
	r := status.TestResult{Name: "ProfilesCap_AgentStaysUp", Status: status.FAILED}
	switch {
	case !t.done:
		r.Reason = fmt.Errorf("push sequence did not finish before the agent stopped")
	case t.processErr != nil:
		r.Reason = t.processErr
	case t.after.activeState != "active":
		r.Reason = fmt.Errorf("agent unit is %q after the oversized push", t.after.activeState)
	case t.before.mainPID == "" || t.before.mainPID == "0" || t.before.mainPID != t.after.mainPID:
		r.Reason = fmt.Errorf("agent main PID changed from %q to %q: it crashed or restarted", t.before.mainPID, t.after.mainPID)
	case t.before.restarts != t.after.restarts:
		r.Reason = fmt.Errorf("agent restart count changed from %s to %s", t.before.restarts, t.after.restarts)
	default:
		r.Status = status.SUCCESSFUL
	}
	return r
}

func (t *ProfilesAttributeCapTestRunner) checkSubsequentDelivery() status.TestResult {
	r := status.TestResult{Name: "ProfilesCap_SubsequentDelivery", Status: status.FAILED}
	switch {
	case !t.done:
		r.Reason = fmt.Errorf("push sequence did not finish before the agent stopped")
	case len(t.subsequentErrs) > 0:
		r.Reason = fmt.Errorf("receiver rejected pushes after the mixed request: %s", strings.Join(t.subsequentErrs, "; "))
	case t.countersErr != nil:
		r.Reason = fmt.Errorf("reading exporter self telemetry: %w", t.countersErr)
	case !t.final.found:
		r.Reason = fmt.Errorf("self telemetry has no %s series for %s", sentProfileSamplesMetric, profilesExporterID)
	case t.final.sendFailed > 0:
		r.Reason = fmt.Errorf("%s reported %.0f profile samples that failed to send (sent %.0f)", profilesExporterID, t.final.sendFailed, t.final.sent)
	case t.final.sent <= t.afterMixed.sent:
		r.Reason = fmt.Errorf("sent counter did not rise after the mixed request (%.0f -> %.0f, %d pushes accepted)", t.afterMixed.sent, t.final.sent, t.subsequentAccepted)
	default:
		r.Status = status.SUCCESSFUL
	}
	return r
}

// checkPartialSuccessSurfaced looks for the exporter's partial success warning, which is the
// only place the backend's rejected count shows up: the exporter returns no error for a 2xx
// partial success, so its self telemetry counts the rejected profile as sent.
func checkPartialSuccessSurfaced(lines []string, logErr error) status.TestResult {
	r := status.TestResult{Name: "ProfilesCap_PartialSuccessSurfaced", Status: status.FAILED}
	if logErr != nil {
		r.Reason = logErr
		return r
	}
	var exportFailures []string
	for _, line := range lines {
		if strings.Contains(line, partialSuccessLogText) {
			log.Printf("[ProfilesCap] partial success line: %s", line)
			r.Status = status.SUCCESSFUL
			return r
		}
		if strings.Contains(strings.ToLower(line), exportFailedLogText) {
			exportFailures = append(exportFailures, line)
		}
	}
	if len(exportFailures) > 0 {
		r.Reason = fmt.Errorf("no %q line; the export failed before the backend could apply the attribute limit:\n%s",
			partialSuccessLogText, strings.Join(exportFailures, "\n"))
		return r
	}
	r.Reason = fmt.Errorf("no %q line and no export failure: the backend's rejection of the oversized profile is not visible in the agent log", partialSuccessLogText)
	return r
}

func TestProfilesAttributeCap(t *testing.T) {
	env := environment.GetEnvironmentMetaData()
	testRunner := &ProfilesAttributeCapTestRunner{BaseTestRunner: test_runner.BaseTestRunner{}, env: env}
	runner := &test_runner.TestRunner{TestRunner: testRunner}
	runner.Run()

	testRunner.mu.Lock()
	checks := testRunner.checks
	testRunner.mu.Unlock()
	if len(checks) == 0 {
		t.Fatal("the attribute cap runner never validated; see the runner log above")
	}

	// Validate already logged every check. Only MUST checks fail the test; a failed SHOULD check is
	// reported so a silent drop is visible without making the suite red.
	var failed []string
	for _, c := range checks {
		if c.Status == status.SUCCESSFUL {
			continue
		}
		if c.level == levelShould {
			t.Logf("SHOULD check %s failed: %v", c.Name, c.Reason)
			continue
		}
		failed = append(failed, fmt.Sprintf("%s: %v", c.Name, c.Reason))
	}
	require.Empty(t, failed, "profiles attribute cap checks failed:\n%s", strings.Join(failed, "\n"))
}
