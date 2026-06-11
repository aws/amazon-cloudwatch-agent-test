// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package dbi

import (
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
	"github.com/aws/amazon-cloudwatch-agent-test/test/otlp_export/otlpvalidation"
	"github.com/aws/amazon-cloudwatch-agent-test/test/status"
	"github.com/aws/amazon-cloudwatch-agent-test/test/test_runner"
	"github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
	"github.com/aws/amazon-cloudwatch-agent-test/util/common"
)

const (
	configPath      = "/tmp/dbi_config.json"
	startCommand    = "sudo /opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl -a fetch-config -m ec2 -s -c "
	workloadDur     = 5 * time.Minute
	instanceName    = "dbi-integ-test"
	serverLogsGroup = "/aws/self-managed-database-insights/postgresql/server-logs"
	rawEventsGroup  = "/aws/self-managed-database-insights/postgresql/raw-events"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

type DbiTestSuite struct {
	suite.Suite
	test_runner.TestSuite
}

func (s *DbiTestSuite) SetupSuite() {
	log.Println(">>>> Starting DbiTestSuite")
}

func (s *DbiTestSuite) TearDownSuite() {
	s.Result.Print()
	log.Println(">>>> Finished DbiTestSuite")
}

func (s *DbiTestSuite) TestAllInSuite() {
	runner := &DbiTestRunner{}
	s.AddToSuiteResult(runner.run())
	s.Assert().Equal(status.SUCCESSFUL, s.Result.GetStatus(), "DBI Test Suite Failed")
}

func TestDbiSuite(t *testing.T) {
	suite.Run(t, new(DbiTestSuite))
}

type DbiTestRunner struct {
	test_runner.BaseTestRunner
}

var _ test_runner.ITestRunner = (*DbiTestRunner)(nil)

func (t *DbiTestRunner) run() status.TestGroupResult {
	// Step 1: Run PostgreSQL setup script
	log.Println("=== Running PostgreSQL setup ===")
	setupOut, err := exec.Command("bash", "setup.sh").CombinedOutput()
	if err != nil {
		log.Printf("setup.sh output:\n%s", string(setupOut))
		return status.TestGroupResult{
			Name:        t.GetTestName(),
			TestResults: []status.TestResult{{Name: "PostgreSQL Setup", Status: status.FAILED, Reason: fmt.Errorf("setup.sh failed: %w", err)}},
		}
	}
	log.Printf("setup.sh output:\n%s", string(setupOut))

	// Step 2: Remove any previous agent config and copy our config
	log.Println("=== Configuring agent ===")
	if err := exec.Command("sudo", "/opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl", "-a", "remove-config").Run(); err != nil {
		log.Printf("remove-config failed (non-fatal): %v", err)
	}
	common.CopyFile(filepath.Join("agent_configs", "dbi_localhost.json"), configPath)

	// Step 3: Start the agent
	log.Println("=== Starting agent ===")
	if err := common.StartAgentWithCommand(configPath, false, false, startCommand); err != nil {
		return status.TestGroupResult{
			Name:        t.GetTestName(),
			TestResults: []status.TestResult{{Name: "Starting Agent", Status: status.FAILED, Reason: err}},
		}
	}

	// Step 4: Generate SQL workload for 5 minutes
	log.Println("=== Generating SQL workload for 5 minutes ===")
	generateWorkload(workloadDur)

	// Step 5: Stop the agent
	log.Println("=== Stopping agent ===")
	common.StopAgent()

	// Step 6: Validate metrics and logs
	log.Println("=== Validating results ===")
	return t.Validate()
}

func (t *DbiTestRunner) GetTestName() string            { return "DBI" }
func (t *DbiTestRunner) GetAgentConfigFileName() string { return "dbi_localhost.json" }
func (t *DbiTestRunner) GetAgentRunDuration() time.Duration {
	return workloadDur
}
func (t *DbiTestRunner) GetMeasuredMetrics() []string {
	return append(append(counterMetrics(), dbLoadMetrics()...), topSQLMetrics()...)
}

func (t *DbiTestRunner) Validate() status.TestGroupResult {
	var results []status.TestResult

	// Validate metrics via PromQL
	metricsResult := otlpvalidation.ValidateOtlpMetrics(t.GetTestName()+" Metrics", "us-west-2", t.GetMeasuredMetrics())
	results = append(results, metricsResult.TestResults...)

	// Validate server logs exist
	serverLogsResult := validateLogGroupHasEvents(serverLogsGroup, "Server Logs")
	results = append(results, serverLogsResult)

	// Validate raw events (query samples/top queries) exist
	rawEventsResult := validateLogGroupHasEvents(rawEventsGroup, "Raw Events")
	results = append(results, rawEventsResult)

	return status.TestGroupResult{Name: t.GetTestName(), TestResults: results}
}

// counterMetrics returns PostgreSQL receiver metrics (enabled: true in golden YAML)
// that reliably appear on a single-node localhost instance without replication.
// Excluded: postgresql.replication.data_delay, postgresql.wal.lag (require replica),
// postgresql.wal.age (requires WAL archiving configured).
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
		"postgresql.rows",
		"postgresql.shared_blks_hit",
		"postgresql.shared_blks_read",
	}
}

// generateWorkload runs SQL queries against the test database for the specified
// duration. This populates pg_stat_activity (DB Load), pg_stat_statements
// (Top SQL), and generates server log entries.
func generateWorkload(duration time.Duration) {
	end := time.Now().Add(duration)
	iteration := 0
	for time.Now().Before(end) {
		iteration++
		queries := []string{
			"SELECT count(*) FROM pg_catalog.pg_class;",
			"SELECT relname, relkind FROM pg_catalog.pg_class LIMIT 10;",
			"SELECT pg_sleep(0.1);",
			fmt.Sprintf("SELECT 'workload_iteration_%d';", iteration),
		}
		for _, q := range queries {
			out, err := exec.Command("sudo", "-u", "postgres", "psql", "-d", "testdb", "-c", q).CombinedOutput()
			if err != nil {
				log.Printf("workload query failed (query=%s): %v, output: %s", q, err, string(out))
			}
		}
		// Pace the workload: ~1 second between iterations
		time.Sleep(1 * time.Second)
	}
	log.Printf("Workload complete: ran %d iterations over %v", iteration, duration)
}

// validateLogGroupHasEvents checks that a CloudWatch Logs log group exists and
// contains at least one log stream with events.
func validateLogGroupHasEvents(logGroup string, testName string) status.TestResult {
	const maxRetries = 10
	const retryInterval = 30 * time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(retryInterval)
		}

		streams := awsservice.GetLogStreams(logGroup)
		if len(streams) == 0 {
			log.Printf("[%s] Attempt %d/%d: no log streams found in %s", testName, attempt+1, maxRetries, logGroup)
			continue
		}

		// Check the most recent stream for events
		streamName := *streams[0].LogStreamName
		events, err := awsservice.GetLogsSince(logGroup, streamName, nil, nil)
		if err != nil {
			log.Printf("[%s] Attempt %d/%d: error getting events from %s/%s: %v", testName, attempt+1, maxRetries, logGroup, streamName, err)
			continue
		}

		if len(events) > 0 {
			log.Printf("[%s] Found %d events in %s/%s", testName, len(events), logGroup, streamName)
			return status.TestResult{Name: testName, Status: status.SUCCESSFUL}
		}

		log.Printf("[%s] Attempt %d/%d: stream exists but no events yet in %s/%s", testName, attempt+1, maxRetries, logGroup, streamName)
	}

	return status.TestResult{
		Name:   testName,
		Status: status.FAILED,
		Reason: fmt.Errorf("no log events found in %s after %d retries", logGroup, maxRetries),
	}
}
