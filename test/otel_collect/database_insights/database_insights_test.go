// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package database_insights

import (
	"fmt"
	"log"
	"os/exec"
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
	workloadDur     = 5 * time.Minute
	serverLogsGroup = "/aws/self-managed-database-insights/postgresql/server-logs"
	rawEventsGroup  = "/aws/self-managed-database-insights/postgresql/raw-events"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

type DbiTestRunner struct {
	test_runner.BaseTestRunner
	env *environment.MetaData
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
	out, err := exec.Command("bash", "resources/database_insights_setup.sh").CombinedOutput()
	log.Printf("setup.sh output:\n%s", string(out))
	if err != nil {
		return fmt.Errorf("setup.sh failed: %w", err)
	}
	return t.BaseTestRunner.SetupBeforeAgentRun()
}

func (t *DbiTestRunner) SetupAfterAgentRun() error {
	if err := initWorkload(); err != nil {
		return err
	}
	go runWorkload(workloadDur)
	return nil
}

func (t *DbiTestRunner) Validate() status.TestGroupResult {
	var results []status.TestResult

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
		"@resource.process.executable.name": "postgres",
		"@resource.host.id":                 t.env.InstanceId,
	})
	results = append(results, processResult.TestResults...)

	return status.TestGroupResult{Name: t.GetTestName(), TestResults: results}
}

func TestDbi(t *testing.T) {
	env := environment.GetEnvironmentMetaData()

	testRunner := &DbiTestRunner{
		BaseTestRunner: test_runner.BaseTestRunner{},
		env:            env,
	}
	runner := &test_runner.TestRunner{TestRunner: testRunner}
	result := runner.Run()

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
