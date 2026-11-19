// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

// Package database_insights_plans checks that the self-managed PostgreSQL Database
// Insights receiver starts collecting query plans after pg_store_plans is set up
// while the agent is already running, without an agent restart.
//
// Timeline, measured from the agent start:
//  1. pg_store_plans is installed but not preloaded and not created (setup script).
//     The agent finds it missing, warns once per receiver and skips plan lookups.
//     Top query events are still reported, with an empty plan.
//  2. The test preloads pg_store_plans and restarts PostgreSQL while the agent runs,
//     which is how a customer adds the extension to a monitored server. The agent
//     must keep running and keep reporting top queries across the restart.
//  3. The warning count stays the same while the extension is still missing, also
//     across the PostgreSQL restart.
//  4. The test runs CREATE EXTENSION in testdb.
//  5. The receiver re-checks, finds the extension and logs that it resumed.
//  6. Top query events that carry a plan appear in the raw events log group, and
//     none carried one before the extension was created.
//
// The workload runs as a non-superuser because the receiver does not look up plans
// for statements owned by a superuser. It runs in short pgbench runs so it survives
// the PostgreSQL restart.
package database_insights_plans

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

const (
	// setupTimeout bounds the base PostgreSQL setup plus the pg_store_plans build.
	setupTimeout = 30 * time.Minute
	// workloadDur is the agent run time. A passing run needs about 8 minutes; see the
	// step durations below.
	workloadDur = 10 * time.Minute

	instanceName   = "dbi-plans-integ-test"
	rawEventsGroup = "/aws/self-managed-database-insights/postgresql/raw-events"
	agentLogPath   = "/opt/aws/amazon-cloudwatch-agent/logs/amazon-cloudwatch-agent.log"

	workloadUser     = "dbi_app"
	workloadPassword = "dbi_app_password"

	// Log messages of the receiver (postgresqlreceiver, queryPlanStoreReady).
	missingMsg = "pg_store_plans extension not installed"
	resumedMsg = "pg_store_plans extension is now installed"

	// firstWarnTimeout is how long to wait for the first plan lookup to find the
	// extension missing. The first top query collection runs when the agent starts.
	firstWarnTimeout = 3 * time.Minute
	// settleAfterWarn lets the other receiver instance do its first collection, so
	// that every instance has warned before the warnings are counted.
	settleAfterWarn = 70 * time.Second
	// restartTimeout bounds the preload script, which restarts PostgreSQL and waits
	// until it accepts connections again.
	restartTimeout = 3 * time.Minute
	// stillMissingWindow is how long the extension stays missing after the PostgreSQL
	// restart. The receiver re-checks on every top query collection (once a minute by
	// default), so this covers at least one re-check, which must not warn again.
	stillMissingWindow = 130 * time.Second
	// workloadRunSeconds is the length of one pgbench run. The workload is a series
	// of runs because a single run does not survive the PostgreSQL restart.
	workloadRunSeconds = 20

	// agentStartMsg is logged each time the agent starts. It must appear once.
	agentStartMsg = "Starting AmazonCloudWatchAgent"
	// resumeTimeout is how long to wait for the receiver to notice the extension.
	// A re-check runs on every top query collection, so the receiver resumes on the
	// first collection after the extension is created.
	resumeTimeout = 4 * time.Minute
	// exportWait gives the receiver time to emit and export events with plans after it resumed.
	exportWait = 90 * time.Second

	logPollInterval = 10 * time.Second
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

// lifecycle is what the background goroutine observed.
type lifecycle struct {
	// warnedBefore and warnedStill are the missing-extension warning counts taken
	// before and after stillMissingWindow, while the extension was still missing.
	warnedBefore int
	warnedStill  int
	// restartedAt is when PostgreSQL was back after the preload restart.
	restartedAt time.Time
	// agentStartsBefore and agentStartsAfter count the agent start messages before the
	// PostgreSQL restart and at the end of the lifecycle.
	agentStartsBefore int
	agentStartsAfter  int
	createdAt         time.Time
	// err is the first step that failed. Later steps did not run.
	err error
}

type DbiPlansTestRunner struct {
	test_runner.BaseTestRunner
	env *environment.MetaData

	done chan struct{}
	mu   sync.Mutex
	life lifecycle
}

var _ test_runner.ITestRunner = (*DbiPlansTestRunner)(nil)

func (t *DbiPlansTestRunner) GetTestName() string { return "DBIPlans" }
func (t *DbiPlansTestRunner) GetAgentConfigFileName() string {
	return "database_insights_plans_config.json"
}
func (t *DbiPlansTestRunner) GetAgentRunDuration() time.Duration { return workloadDur }
func (t *DbiPlansTestRunner) GetMeasuredMetrics() []string       { return []string{} }

func (t *DbiPlansTestRunner) SetupBeforeAgentRun() error {
	ctx, cancel := context.WithTimeout(context.Background(), setupTimeout)
	defer cancel()

	// The base script installs PostgreSQL with pg_stat_statements and creates testdb,
	// cw_monitor and the pgpass file. The second script builds pg_store_plans without
	// preloading or creating it, and creates the workload role.
	for _, script := range []string{
		"../database_insights/resources/database_insights_setup.sh",
		"resources/database_insights_plans_setup.sh",
	} {
		log.Printf("=== Running %s ===", script)
		cmd := exec.CommandContext(ctx, "bash", script)
		// Return shortly after the kill even if orphaned sudo children keep the pipe open.
		cmd.WaitDelay = 10 * time.Second
		out, err := cmd.CombinedOutput()
		log.Printf("%s output:\n%s", script, string(out))
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("%s timed out after %v; output above shows the last step reached", script, setupTimeout)
		}
		if err != nil {
			return fmt.Errorf("%s failed: %w", script, err)
		}
	}
	return t.BaseTestRunner.SetupBeforeAgentRun()
}

func (t *DbiPlansTestRunner) SetupAfterAgentRun() error {
	if err := initWorkload(); err != nil {
		return err
	}
	go runWorkload(workloadDur)
	go t.driveExtensionLifecycle()
	return nil
}

// driveExtensionLifecycle waits until the receiver reports the extension missing,
// checks that the warning is not repeated, creates the extension, and waits until
// the receiver reports that it resumed.
func (t *DbiPlansTestRunner) driveExtensionLifecycle() {
	defer close(t.done)
	var life lifecycle
	defer func() {
		t.mu.Lock()
		t.life = life
		t.mu.Unlock()
	}()

	if err := waitForAgentLog(missingMsg, firstWarnTimeout); err != nil {
		life.err = fmt.Errorf("the agent never reported pg_store_plans as missing; the agent build may not contain the pg_store_plans plan lookup, or the workload role was skipped: %w", err)
		return
	}
	log.Println("=== The agent reported pg_store_plans as missing ===")

	time.Sleep(settleAfterWarn)
	var err error
	if life.warnedBefore, err = countAgentLog(missingMsg); err != nil {
		life.err = err
		return
	}
	if life.agentStartsBefore, err = countAgentLog(agentStartMsg); err != nil {
		life.err = err
		return
	}

	// Preload the library and restart PostgreSQL while the agent is running.
	if err = preloadAndRestartPostgres(); err != nil {
		life.err = err
		return
	}
	life.restartedAt = time.Now()
	log.Println("=== PostgreSQL restarted with pg_store_plans preloaded; the agent keeps running ===")

	time.Sleep(stillMissingWindow)
	if life.warnedStill, err = countAgentLog(missingMsg); err != nil {
		life.err = err
		return
	}

	if err = createExtension(); err != nil {
		life.err = err
		return
	}
	life.createdAt = time.Now()

	if err = waitForAgentLog(resumedMsg, resumeTimeout); err != nil {
		life.err = fmt.Errorf("the agent did not resume plan collection after the extension was created; it needs a restart to notice it: %w", err)
		return
	}
	log.Println("=== The agent resumed plan collection ===")

	time.Sleep(exportWait)
	if life.agentStartsAfter, err = countAgentLog(agentStartMsg); err != nil {
		life.err = err
	}
}

func (t *DbiPlansTestRunner) Validate() status.TestGroupResult {
	var results []status.TestResult

	select {
	case <-t.done:
	case <-time.After(2 * time.Minute):
		results = append(results, failed("Extension lifecycle completed",
			errors.New("the extension lifecycle goroutine was still running when the workload ended")))
		return status.TestGroupResult{Name: t.GetTestName(), TestResults: results}
	}
	t.mu.Lock()
	life := t.life
	t.mu.Unlock()

	const lifecycleName = "Plan lookups skipped while pg_store_plans missing, resumed after it was created"
	if life.err != nil {
		results = append(results, failed(lifecycleName, life.err))
		return status.TestGroupResult{Name: t.GetTestName(), TestResults: results}
	}
	results = append(results, passed(lifecycleName))

	// The agent was not restarted at any point, including the PostgreSQL restart.
	if life.agentStartsBefore < 1 || life.agentStartsAfter != life.agentStartsBefore {
		results = append(results, failed("Agent kept running through the PostgreSQL restart",
			fmt.Errorf("%q appears %d times before the PostgreSQL restart and %d times at the end; want at least 1 and no change",
				agentStartMsg, life.agentStartsBefore, life.agentStartsAfter)))
	} else {
		results = append(results, passed("Agent kept running through the PostgreSQL restart"))
	}

	// Each receiver instance warns when a database becomes missing, and not again on
	// the re-checks that still find it missing.
	if life.warnedBefore < 1 || life.warnedStill != life.warnedBefore {
		results = append(results, failed("Missing-extension warning is not repeated",
			fmt.Errorf("warnings before the still-missing window: %d, after: %d; want at least 1 and no change", life.warnedBefore, life.warnedStill)))
	} else {
		results = append(results, passed("Missing-extension warning is not repeated"))
	}

	// Every receiver instance that warned also reports that it resumed.
	warned, werr := countAgentLog(missingMsg)
	resumed, rerr := countAgentLog(resumedMsg)
	switch {
	case werr != nil || rerr != nil:
		results = append(results, failed("Every warned receiver resumed", errors.Join(werr, rerr)))
	case warned != resumed:
		results = append(results, failed("Every warned receiver resumed",
			fmt.Errorf("%d missing-extension warnings but %d resume messages", warned, resumed)))
	default:
		results = append(results, passed("Every warned receiver resumed"))
	}

	results = append(results, t.validatePlanEvents(life.restartedAt, life.createdAt)...)
	return status.TestGroupResult{Name: t.GetTestName(), TestResults: results}
}

// validatePlanEvents checks the raw events stream: top query events were reported
// without plans while the extension was missing, top queries kept being reported
// after the PostgreSQL restart, none carried a plan before the extension was
// created, and at least one did afterwards.
func (t *DbiPlansTestRunner) validatePlanEvents(restartedAt, createdAt time.Time) []status.TestResult {
	const (
		maxRetries    = 5
		retryInterval = 30 * time.Second
	)
	stream := fmt.Sprintf("%s/%s", t.env.InstanceId, instanceName)

	var (
		emptyBefore, emptyAfterRestart, plansBefore, plansAfter int
		lastErr                                                 error
	)
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(retryInterval)
		}
		events, err := awsservice.GetLogsSince(rawEventsGroup, stream, nil, nil)
		if err != nil {
			lastErr = err
			log.Printf("[plan events] attempt %d/%d: %v", attempt+1, maxRetries, err)
			continue
		}
		lastErr = nil
		emptyBefore, emptyAfterRestart, plansBefore, plansAfter = 0, 0, 0, 0
		for _, e := range events {
			if e.Message == nil || e.Timestamp == nil {
				continue
			}
			plan, isTopQuery := topQueryPlan(*e.Message)
			if !isTopQuery {
				continue
			}
			at := time.UnixMilli(*e.Timestamp)
			before := at.Before(createdAt)
			switch {
			case before && plan == "":
				emptyBefore++
				if at.After(restartedAt) {
					emptyAfterRestart++
				}
			case before:
				plansBefore++
			case plan != "":
				plansAfter++
			}
		}
		log.Printf("[plan events] attempt %d/%d: before %s: %d without a plan, %d with a plan; after: %d with a plan",
			attempt+1, maxRetries, createdAt.Format(time.RFC3339), emptyBefore, plansBefore, plansAfter)
		if plansAfter > 0 {
			break
		}
	}

	var results []status.TestResult
	if lastErr != nil {
		return append(results, failed("Top query events reported", lastErr))
	}
	if emptyBefore == 0 {
		results = append(results, failed("Top queries reported without plans while the extension was missing",
			fmt.Errorf("no top query event with an empty plan in %s/%s before %s", rawEventsGroup, stream, createdAt.Format(time.RFC3339))))
	} else {
		results = append(results, passed("Top queries reported without plans while the extension was missing"))
	}
	if emptyAfterRestart == 0 {
		results = append(results, failed("Top queries still reported after the PostgreSQL restart",
			fmt.Errorf("no top query event between the PostgreSQL restart (%s) and CREATE EXTENSION (%s)",
				restartedAt.Format(time.RFC3339), createdAt.Format(time.RFC3339))))
	} else {
		results = append(results, passed("Top queries still reported after the PostgreSQL restart"))
	}
	if plansBefore > 0 {
		results = append(results, failed("No plans before the extension was created",
			fmt.Errorf("%d events carried a plan before pg_store_plans was created", plansBefore)))
	} else {
		results = append(results, passed("No plans before the extension was created"))
	}
	if plansAfter == 0 {
		results = append(results, failed("Plans reported after the extension was created",
			fmt.Errorf("no event carried a plan after %s", createdAt.Format(time.RFC3339))))
	} else {
		results = append(results, passed("Plans reported after the extension was created"))
	}
	return results
}

func TestDbiPlans(t *testing.T) {
	env := environment.GetEnvironmentMetaData()

	testRunner := &DbiPlansTestRunner{
		BaseTestRunner: test_runner.BaseTestRunner{},
		env:            env,
		done:           make(chan struct{}),
	}
	runner := &test_runner.TestRunner{TestRunner: testRunner}
	result := runner.Run()

	// Print every result first so one failure does not hide the others.
	result.Print()
	for _, r := range result.TestResults {
		require.Equal(t, status.SUCCESSFUL, r.Status, "%s failed: %v", r.Name, r.Reason)
	}
}

// topQueryPlan returns the postgresql.query_plan attribute of a raw events record.
// Records are single-line OTLP JSON with flat dotted attribute keys, and the plan
// is a string that is empty when no plan was collected. isTopQuery is false for
// records that do not carry the attribute, such as query samples.
func topQueryPlan(message string) (plan string, isTopQuery bool) {
	var record struct {
		Attributes map[string]any `json:"attributes"`
	}
	if err := json.Unmarshal([]byte(message), &record); err != nil {
		return "", false
	}
	value, ok := record.Attributes["postgresql.query_plan"]
	if !ok {
		return "", false
	}
	plan, _ = value.(string)
	return plan, true
}

// preloadAndRestartPostgres preloads pg_store_plans and restarts PostgreSQL. It runs
// while the agent is running.
func preloadAndRestartPostgres() error {
	ctx, cancel := context.WithTimeout(context.Background(), restartTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "resources/database_insights_plans_preload.sh")
	cmd.WaitDelay = 10 * time.Second
	out, err := cmd.CombinedOutput()
	log.Printf("preload and restart output:\n%s", string(out))
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("preloading pg_store_plans and restarting PostgreSQL timed out after %v", restartTimeout)
	}
	if err != nil {
		return fmt.Errorf("preloading pg_store_plans and restarting PostgreSQL failed: %w", err)
	}
	return nil
}

func createExtension() error {
	out, err := exec.Command("sudo", "-u", "postgres", "psql", "-d", "testdb",
		"-v", "ON_ERROR_STOP=1", "-c", "CREATE EXTENSION pg_store_plans").CombinedOutput()
	log.Printf("CREATE EXTENSION output:\n%s", string(out))
	if err != nil {
		return fmt.Errorf("CREATE EXTENSION pg_store_plans failed: %w", err)
	}
	return nil
}

// countAgentLog returns how many lines of the agent log contain message.
func countAgentLog(message string) (int, error) {
	out, err := exec.Command("sudo", "grep", "-F", "-c", message, agentLogPath).Output()
	if err != nil {
		// grep exits 1 when nothing matched; any other failure is a real error.
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			return 0, fmt.Errorf("grep %s: %w", agentLogPath, err)
		}
	}
	n, convErr := strconv.Atoi(strings.TrimSpace(string(out)))
	if convErr != nil {
		return 0, fmt.Errorf("parse grep count %q: %w", strings.TrimSpace(string(out)), convErr)
	}
	return n, nil
}

// waitForAgentLog polls the agent log until it contains message.
func waitForAgentLog(message string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		n, err := countAgentLog(message)
		if err != nil {
			log.Printf("waiting for %q: %v", message, err)
		} else if n > 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%q not found in %s after %v", message, agentLogPath, timeout)
		}
		time.Sleep(logPollInterval)
	}
}

func passed(name string) status.TestResult {
	return status.TestResult{Name: name, Status: status.SUCCESSFUL}
}

func failed(name string, reason error) status.TestResult {
	return status.TestResult{Name: name, Status: status.FAILED, Reason: reason}
}

// pgbench runs pgbench against testdb over TCP as the non-superuser workload role.
func pgbench(args ...string) *exec.Cmd {
	cmd := exec.Command("pgbench", append([]string{"-h", "127.0.0.1", "-U", workloadUser}, args...)...)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+workloadPassword)
	return cmd
}

func initWorkload() error {
	log.Println("=== Initializing pgbench tables ===")
	out, err := pgbench("-i", "testdb").CombinedOutput()
	log.Printf("pgbench init output:\n%s", string(out))
	if err != nil {
		return fmt.Errorf("pgbench init failed: %w", err)
	}
	return nil
}

// runWorkload runs pgbench in short runs until the duration has passed. A run that
// fails, which happens while PostgreSQL restarts, is logged and followed by the next.
func runWorkload(duration time.Duration) {
	end := time.Now().Add(duration)
	log.Printf("=== Running pgbench in %d second runs for %s ===", workloadRunSeconds, duration)
	for time.Now().Before(end) {
		out, err := pgbench("-c", "10", "-j", "2", "-T", strconv.Itoa(workloadRunSeconds), "testdb").CombinedOutput()
		if err != nil {
			log.Printf("pgbench run failed, retrying in 5 seconds: %v, output: %s", err, string(out))
			time.Sleep(5 * time.Second)
		}
	}
	log.Println("=== The workload finished ===")
}
