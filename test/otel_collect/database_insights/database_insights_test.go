// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package database_insights

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/otel_collect/otlpvalidation"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

const (
	setupTimeout    = 20 * time.Minute
	workloadDur     = 5 * time.Minute
	serverLogsGroup = "/aws/self-managed-database-insights/postgresql/server-logs"
	rawEventsGroup  = "/aws/self-managed-database-insights/postgresql/raw-events"
	// droppedDatabase is created, used and dropped before the agent starts, so that
	// pg_stat_statements keeps rows for a database that no longer exists.
	droppedDatabase = "dbi_dropped_db"
	agentUnit       = "amazon-cloudwatch-agent"
	// pidWatchWindow covers the first 4.5 minutes of the workload, ending before the
	// runner stops the agent after workloadDur.
	pidWatchWindow = workloadDur - 30*time.Second
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

type DbiTestRunner struct {
	test_runner.BaseTestRunner
	env *environment.MetaData
	// agentStartedAt is just before the runner's first agent start; the agent's
	// journal is checked from here.
	agentStartedAt time.Time
	// pidWatch delivers whether the agent kept one main process during the workload.
	pidWatch chan pidWatchResult
}

var _ test_runner.ITestRunner = (*DbiTestRunner)(nil)

func (t *DbiTestRunner) GetTestName() string                { return "DBI" }
func (t *DbiTestRunner) GetAgentConfigFileName() string     { return "database_insights_config.json" }
func (t *DbiTestRunner) GetAgentRunDuration() time.Duration { return workloadDur }
func (t *DbiTestRunner) GetMeasuredMetrics() []string {
	return append(append(counterMetrics(), dbLoadMetrics()...), topSQLMetrics()...)
}

func (t *DbiTestRunner) SetupBeforeAgentRun() error {
	log.Println("=== Running PostgreSQL setup ===")
	// Bound the setup script so a hang fails fast with its partial output instead
	// of stalling silently until the job-level 60-minute timeout: CombinedOutput
	// buffers all output, so nothing is visible in CI until the script exits.
	ctx, cancel := context.WithTimeout(context.Background(), setupTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bash", "resources/database_insights_setup.sh")
	// Ensure Wait returns shortly after the kill even if orphaned sudo children
	// keep the output pipe open.
	cmd.WaitDelay = 10 * time.Second
	out, err := cmd.CombinedOutput()
	log.Printf("setup.sh output:\n%s", string(out))
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("setup.sh timed out after %v; output above shows the last step reached", setupTimeout)
	}
	if err != nil {
		return fmt.Errorf("setup.sh failed: %w", err)
	}
	// pg_stat_statements keeps rows for a dropped database, and the top query SQL of
	// an agent without the fix returns them with a NULL datname. That agent panics on
	// them in its first top query collection after a start and restart-loops, so drop
	// a database before the agent's first start.
	if err := dropDatabaseWithStats(droppedDatabase); err != nil {
		return err
	}
	if err := t.BaseTestRunner.SetupBeforeAgentRun(); err != nil {
		return err
	}
	t.agentStartedAt = time.Now()
	return nil
}

func (t *DbiTestRunner) SetupAfterAgentRun() error {
	t.pidWatch = watchAgentPID(pidWatchWindow)
	if err := initWorkload(); err != nil {
		return err
	}
	go runWorkload(workloadDur)
	return nil
}

func (t *DbiTestRunner) Validate() status.TestGroupResult {
	// The crash checks come first: on an agent that restart-loops they name the cause,
	// and the CloudWatch checks below could only fail for lack of data after minutes
	// of retries, so they are skipped.
	results := t.validateAgentSurvivedDroppedDatabase()
	for _, r := range results {
		if r.Status != status.SUCCESSFUL {
			return status.TestGroupResult{Name: t.GetTestName(), TestResults: results}
		}
	}

	metricsResult := otlpvalidation.ValidateOtlpMetricsWithLabels(t.GetTestName()+" Metrics", t.env.Region, t.GetMeasuredMetrics(), map[string]string{
		"@resource.db.system.name":   "postgresql",
		"@resource.db.instance.name": "dbi-integ-test",
		"@resource.host.id":          t.env.InstanceId,
	})
	results = append(results, metricsResult.TestResults...)

	logStream := fmt.Sprintf("%s/dbi-integ-test", t.env.InstanceId)
	serverLogsResult := validateLogStream(serverLogsGroup, logStream, "Server Logs")
	results = append(results, serverLogsResult)

	rawEventsResult := validateLogStream(rawEventsGroup, logStream, "Raw Events")
	results = append(results, rawEventsResult)

	processResult := otlpvalidation.ValidateOtlpMetricsWithLabels(t.GetTestName()+" Process Metrics", t.env.Region, processMetrics(), map[string]string{
		"@resource.process.executable.name": "post(gres|master).*",
		"@resource.host.id":                 t.env.InstanceId,
	})
	results = append(results, processResult.TestResults...)

	return status.TestGroupResult{Name: t.GetTestName(), TestResults: results}
}

// validateAgentSurvivedDroppedDatabase checks that the agent, started with rows of
// a dropped database in pg_stat_statements, kept running: one main process for the
// first 4.5 minutes of the workload, and no panic in its journal since its start.
func (t *DbiTestRunner) validateAgentSurvivedDroppedDatabase() []status.TestResult {
	running := status.TestResult{Name: "Agent keeps running after DROP DATABASE", Status: status.SUCCESSFUL}
	switch w := <-t.pidWatch; {
	case w.err != nil:
		running.Status, running.Reason = status.FAILED, fmt.Errorf("could not read the agent main PID: %w", w.err)
	case w.firstPID == "":
		running.Status, running.Reason = status.FAILED, errors.New("the agent had no main process when the workload started")
	case w.changedTo != w.firstPID:
		running.Status, running.Reason = status.FAILED, fmt.Errorf("the agent main PID changed from %s to %q during the workload", w.firstPID, w.changedTo)
	}

	noPanic := status.TestResult{Name: "No agent panic after DROP DATABASE", Status: status.SUCCESSFUL}
	since := t.agentStartedAt.Format("2006-01-02 15:04:05")
	out, err := exec.Command("sudo", "journalctl", "-u", agentUnit, "--since", since, "--no-pager").CombinedOutput()
	switch {
	case err != nil:
		noPanic.Status, noPanic.Reason = status.FAILED, fmt.Errorf("journalctl -u %s --since %q: %w", agentUnit, since, err)
	case strings.Contains(string(out), "panic:"):
		noPanic.Status, noPanic.Reason = status.FAILED, fmt.Errorf("the agent panicked: %s", firstLineWith(string(out), "panic:"))
	}
	return []status.TestResult{running, noPanic}
}

func TestDbi(t *testing.T) {
	env := environment.GetEnvironmentMetaData()

	testRunner := &DbiTestRunner{
		BaseTestRunner: test_runner.BaseTestRunner{},
		env:            env,
	}
	runner := &test_runner.TestRunner{TestRunner: testRunner}
	result := runner.Run()
	result.Print()

	for _, r := range result.TestResults {
		require.Equal(t, status.SUCCESSFUL, r.Status, "%s failed: %v", r.Name, r.Reason)
	}
}

// counterMetrics returns PostgreSQL receiver metrics (enabled: true in golden YAML)
// that reliably appear on a single-node localhost instance without replication.
func counterMetrics() []string {
	return []string{
		"postgresql.backends",
		"postgresql.bgwriter.buffers.allocated",
		"postgresql.bgwriter.buffers.writes",
		"postgresql.bgwriter.checkpoint.count",
		"postgresql.bgwriter.duration",
		"postgresql.bgwriter.maxwritten",
		"postgresql.blocks_read",
		"postgresql.commits",
		"postgresql.connection.max",
		"postgresql.database.count",
		"postgresql.db_size",
		"postgresql.index.scans",
		"postgresql.index.size",
		"postgresql.operations",
		"postgresql.rollbacks",
		"postgresql.rows",
		"postgresql.table.count",
		"postgresql.table.size",
		"postgresql.table.vacuum.count",
	}
}

// dbLoadMetrics returns all 8 DB Load metrics produced by the count/dbi_dbload
// connector from pg_stat_activity snapshots.
func dbLoadMetrics() []string {
	return []string{
		"postgresql.active_sessions.by_app",
		"postgresql.active_sessions.by_db",
		"postgresql.active_sessions.by_host",
		"postgresql.active_sessions.by_sql",
		"postgresql.active_sessions.by_sql_wait",
		"postgresql.active_sessions.by_user",
		"postgresql.active_sessions.by_wait",
		"postgresql.active_sessions.count",
	}
}

// topSQLMetrics returns Top SQL metrics produced by the
// signaltometrics/dbi_topsql connector from pg_stat_statements.
func topSQLMetrics() []string {
	return []string{
		"postgresql.calls",
		"postgresql.total_exec_time",
		"postgresql.total_plan_time",
		"postgresql.shared_blks_hit",
		"postgresql.shared_blks_read",
	}
}

func processMetrics() []string {
	return []string{
		"process.cpu.utilization",
		"process.memory.utilization",
	}
}

func initWorkload() error {
	log.Println("=== Initializing pgbench tables ===")
	out, err := exec.Command("sudo", "-u", "postgres", "pgbench", "-i", "testdb").CombinedOutput()
	log.Printf("pgbench init output:\n%s", string(out))
	if err != nil {
		return fmt.Errorf("pgbench init failed: %w", err)
	}
	return nil
}

func runWorkload(duration time.Duration) {
	seconds := fmt.Sprintf("%d", int(duration.Seconds()))
	log.Printf("=== Running pgbench for %s seconds with 10 clients ===", seconds)
	out, err := exec.Command("sudo", "-u", "postgres", "pgbench", "-c", "10", "-j", "2", "-T", seconds, "testdb").CombinedOutput()
	if err != nil {
		log.Printf("pgbench failed: %v, output: %s", err, string(out))
		return
	}
	log.Printf("pgbench output:\n%s", string(out))
}

func validateLogStream(logGroup string, streamName string, testName string) status.TestResult {
	const maxRetries = 3
	const retryInterval = 30 * time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(retryInterval)
		}

		events, err := awsservice.GetLogsSince(logGroup, streamName, nil, nil)
		if err != nil {
			log.Printf("[%s] Attempt %d/%d: error getting events from %s/%s: %v", testName, attempt+1, maxRetries, logGroup, streamName, err)
			continue
		}

		if len(events) > 0 {
			log.Printf("[%s] Found %d events in %s/%s", testName, len(events), logGroup, streamName)
			return status.TestResult{Name: testName, Status: status.SUCCESSFUL}
		}

		log.Printf("[%s] Attempt %d/%d: no events yet in %s/%s", testName, attempt+1, maxRetries, logGroup, streamName)
	}

	return status.TestResult{
		Name:   testName,
		Status: status.FAILED,
		Reason: fmt.Errorf("no log events found in %s/%s after %d retries", logGroup, streamName, maxRetries),
	}
}

// dropDatabaseWithStats creates database name, runs statements in it so that
// pg_stat_statements records rows for it, and drops it. It fails if no orphan
// rows remain afterwards, so the scenario cannot pass without being set up. It
// runs before the agent starts, so no other session holds the database and a
// plain DROP DATABASE works on every PostgreSQL version.
func dropDatabaseWithStats(name string) error {
	log.Printf("=== Creating, using and dropping database %s ===", name)
	steps := [][]string{
		{"createdb", name},
		{"psql", "-d", name, "-c", "CREATE TABLE dropped_t (id int); INSERT INTO dropped_t VALUES (1), (2); SELECT count(*) FROM dropped_t;"},
		{"psql", "-d", "postgres", "-c", fmt.Sprintf("DROP DATABASE %s;", name)},
	}
	for _, step := range steps {
		out, err := exec.Command("sudo", append([]string{"-u", "postgres"}, step...)...).CombinedOutput()
		log.Printf("%s output:\n%s", strings.Join(step, " "), string(out))
		if err != nil {
			return fmt.Errorf("%s failed: %w", strings.Join(step, " "), err)
		}
	}
	out, err := exec.Command("sudo", "-u", "postgres", "psql", "-d", "postgres", "-tAc",
		"SELECT count(*) FROM pg_stat_statements s LEFT JOIN pg_database d ON s.dbid = d.oid WHERE d.oid IS NULL").CombinedOutput()
	if err != nil {
		return fmt.Errorf("counting orphan pg_stat_statements rows failed: %w: %s", err, string(out))
	}
	count := strings.TrimSpace(string(out))
	if count == "" || count == "0" {
		return fmt.Errorf("expected pg_stat_statements rows for the dropped database %s, found %q", name, count)
	}
	log.Printf("pg_stat_statements rows of dropped databases: %s", count)
	return nil
}

type pidWatchResult struct {
	firstPID  string
	changedTo string // equals firstPID when the main process never changed
	err       error  // set when the main PID could not be read
}

// watchAgentPID samples the agent unit's main PID every 10 seconds for d. A
// panic exits the agent and systemd restarts it, which shows as a new or empty PID.
func watchAgentPID(d time.Duration) chan pidWatchResult {
	ch := make(chan pidWatchResult, 1)
	go func() {
		first, err := readAgentMainPID()
		res := pidWatchResult{firstPID: first, changedTo: first, err: err}
		for deadline := time.Now().Add(d); res.err == nil && time.Now().Before(deadline); {
			time.Sleep(10 * time.Second)
			pid, err := readAgentMainPID()
			if err != nil {
				res.err = err
				break
			}
			if pid != first {
				res.changedTo = pid
				break
			}
		}
		ch <- res
	}()
	return ch
}

// readAgentMainPID returns the agent unit's main PID, or "" when it has no running
// main process. It retries once, so that one failed systemctl call is not taken
// for a crash.
func readAgentMainPID() (string, error) {
	out, err := exec.Command("systemctl", "show", "-p", "MainPID", agentUnit).Output()
	if err != nil {
		time.Sleep(2 * time.Second)
		if out, err = exec.Command("systemctl", "show", "-p", "MainPID", agentUnit).Output(); err != nil {
			return "", fmt.Errorf("systemctl show -p MainPID %s: %w", agentUnit, err)
		}
	}
	return parseMainPID(string(out)), nil
}

// parseMainPID returns the MainPID value from `systemctl show -p MainPID` output,
// or "" when it is 0 or missing.
func parseMainPID(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if pid, ok := strings.CutPrefix(strings.TrimSpace(line), "MainPID="); ok && pid != "0" {
			return pid
		}
	}
	return ""
}

func firstLineWith(out, substr string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, substr) {
			return strings.TrimSpace(line)
		}
	}
	return ""
}
