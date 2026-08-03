// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build windows

package database_insights_sqlserver

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
	instanceName    = "dbi-sqlserver-integ-test"
	saPassword      = "IntegTest#P@ss1"
	workloadDur     = 5 * time.Minute
	serverLogsGroup = "/aws/self-managed-database-insights/sqlserver/server-logs"
	rawEventsGroup  = "/aws/self-managed-database-insights/sqlserver/raw-events"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

type DbiSqlServerTestRunner struct {
	test_runner.BaseTestRunner
	env *environment.MetaData
}

var _ test_runner.ITestRunner = (*DbiSqlServerTestRunner)(nil)

func (t *DbiSqlServerTestRunner) GetTestName() string { return "DBI_SQLServer" }
func (t *DbiSqlServerTestRunner) GetAgentConfigFileName() string {
	return "database_insights_sqlserver_config.json"
}
func (t *DbiSqlServerTestRunner) GetAgentRunDuration() time.Duration { return workloadDur }
func (t *DbiSqlServerTestRunner) GetMeasuredMetrics() []string {
	return append(append(counterMetrics(), dbLoadMetrics()...), topSQLMetrics()...)
}

func (t *DbiSqlServerTestRunner) SetupBeforeAgentRun() error {
	log.Println("=== Running SQL Server setup ===")
	out, err := exec.Command("powershell", "-ExecutionPolicy", "Bypass", "-File", "resources\\database_insights_sqlserver_setup.ps1").CombinedOutput()
	log.Printf("setup.ps1 output:\n%s", string(out))
	if err != nil {
		return fmt.Errorf("setup.ps1 failed: %w", err)
	}
	return t.BaseTestRunner.SetupBeforeAgentRun()
}

func (t *DbiSqlServerTestRunner) SetupAfterAgentRun() error {
	go runWorkload(workloadDur)
	return nil
}

func (t *DbiSqlServerTestRunner) Validate() status.TestGroupResult {
	var results []status.TestResult

	metricsResult := otlpvalidation.ValidateOtlpMetricsWithLabels(t.GetTestName()+" Metrics", t.env.Region, t.GetMeasuredMetrics(), map[string]string{
		"@resource.db.system.name":   "sqlserver",
		"@resource.db.instance.name": instanceName,
		"@resource.host.id":          t.env.InstanceId,
	})
	results = append(results, metricsResult.TestResults...)

	logStream := fmt.Sprintf("%s/%s", t.env.InstanceId, instanceName)
	results = append(results, validateLogStream(serverLogsGroup, logStream, "Server Logs"))
	results = append(results, validateLogStream(rawEventsGroup, logStream, "Raw Events"))

	processResult := otlpvalidation.ValidateOtlpMetricsWithLabels(t.GetTestName()+" Process Metrics", t.env.Region, processMetrics(), map[string]string{
		"@resource.process.executable.name": "sqlservr.*",
		"@resource.host.id":                 t.env.InstanceId,
	})
	results = append(results, processResult.TestResults...)

	return status.TestGroupResult{Name: t.GetTestName(), TestResults: results}
}

func TestDbiSqlServer(t *testing.T) {
	env := environment.GetEnvironmentMetaData()
	testRunner := &DbiSqlServerTestRunner{BaseTestRunner: test_runner.BaseTestRunner{}, env: env}
	runner := &test_runner.TestRunner{TestRunner: testRunner}
	result := runner.Run()
	for _, r := range result.TestResults {
		require.Equal(t, status.SUCCESSFUL, r.Status, "%s failed: %v", r.Name, r.Reason)
	}
}

// counterMetrics returns all SQL Server receiver metrics enabled by the DBI translator.
// Includes both DMV-based metrics (work on Linux/Windows) and Windows Performance Monitor
// counters (Windows-only: transaction.rate, page.split.rate, transaction_log.*, lock.wait_time.avg).
func counterMetrics() []string {
	return []string{
		"sqlserver.batch.request.rate",
		"sqlserver.batch.sql_compilation.rate",
		"sqlserver.batch.sql_recompilation.rate",
		"sqlserver.deadlock.rate",
		"sqlserver.page.buffer_cache.hit_ratio",
		"sqlserver.page.lookup.rate",
		"sqlserver.page.buffer_cache.free_list.stalls.rate",
		"sqlserver.page.checkpoint.flush.rate",
		"sqlserver.page.lazy_write.rate",
		"sqlserver.page.split.rate",
		"sqlserver.forwarded_records.rate",
		"sqlserver.latch.wait.rate",
		"sqlserver.lock.wait.rate",
		"sqlserver.lock.timeout.rate",
		"sqlserver.lock.wait.count",
		"sqlserver.lock.wait_time.avg",
		"sqlserver.user.connection.count",
		"sqlserver.processes.blocked",
		"sqlserver.memory.grants.pending.count",
		"sqlserver.memory.usage",
		"sqlserver.login.rate",
		"sqlserver.logout.rate",
		"sqlserver.session.count",
		"sqlserver.index.search.rate",
		"sqlserver.database.full_scan.rate",
		"sqlserver.database.execution.errors",
		"sqlserver.database.tempdb.version_store.size",
		"sqlserver.resource_pool.disk.throttled.write.rate",
		"sqlserver.transaction.rate",
		"sqlserver.transaction.write.rate",
		"sqlserver.transaction.mirror_write.rate",
		"sqlserver.transaction_log.flush.rate",
		"sqlserver.transaction_log.flush.data.rate",
		"sqlserver.transaction_log.flush.wait.rate",
		"sqlserver.transaction_log.growth.count",
		"sqlserver.transaction_log.shrink.count",
		"sqlserver.transaction_log.usage",
	}
}

// dbLoadMetrics returns the 8 DB Load metrics produced by count/dbi_dbload_sqlserver.
func dbLoadMetrics() []string {
	return []string{
		"sqlserver.active_sessions.by_app",
		"sqlserver.active_sessions.by_wait",
		"sqlserver.active_sessions.by_user",
		"sqlserver.active_sessions.by_db",
		"sqlserver.active_sessions.by_sql",
		"sqlserver.active_sessions.by_sql_wait",
		"sqlserver.active_sessions.by_host",
		"sqlserver.active_sessions.count",
	}
}

// topSQLMetrics returns Top SQL metrics produced by signaltometrics/dbi_topsql_sqlserver.
func topSQLMetrics() []string {
	return []string{
		"sqlserver.execution_count",
		"sqlserver.total_elapsed_time",
		"sqlserver.total_worker_time",
		"sqlserver.total_logical_reads",
		"sqlserver.total_logical_writes",
		"sqlserver.total_physical_reads",
		"sqlserver.total_rows",
		"sqlserver.total_grant_kb",
	}
}

func processMetrics() []string {
	return []string{
		"process.cpu.utilization",
		"process.memory.utilization",
		"process.threads",
	}
}

// runWorkload drives a mixed read/write workload so query_sample captures active
// sessions and top_query accumulates plan stats.
func runWorkload(duration time.Duration) {
	sqlcmd := `C:\Program Files\Microsoft SQL Server\Client SDK\ODBC\170\Tools\Binn\SQLCMD.EXE`
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		out, err := exec.Command(sqlcmd,
			"-S", "localhost", "-U", "sa", "-P", saPassword, "-C", "-N",
			"-d", "testdb", "-Q", `
SET NOCOUNT ON;
INSERT INTO test_orders (customer_name, amount) VALUES (CONCAT('Customer_', CAST(RAND()*1000 AS INT)), RAND()*1000);
SELECT TOP 100 * FROM test_orders ORDER BY created_at DESC;
SELECT customer_name, SUM(amount) FROM test_orders GROUP BY customer_name;
SELECT COUNT_BIG(*) FROM sys.all_objects a CROSS JOIN sys.all_objects b OPTION (MAXDOP 1);
`).CombinedOutput()
		if err != nil {
			log.Printf("workload query failed: %v, output: %s", err, string(out))
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func validateLogStream(logGroup, streamName, testName string) status.TestResult {
	const maxRetries = 3
	const retryInterval = 30 * time.Second
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(retryInterval)
		}
		events, err := awsservice.GetLogsSince(logGroup, streamName, nil, nil)
		if err != nil {
			log.Printf("[%s] attempt %d/%d: error getting events from %s/%s: %v", testName, attempt+1, maxRetries, logGroup, streamName, err)
			continue
		}
		if len(events) > 0 {
			log.Printf("[%s] found %d events in %s/%s", testName, len(events), logGroup, streamName)
			return status.TestResult{Name: testName, Status: status.SUCCESSFUL}
		}
		log.Printf("[%s] attempt %d/%d: no events yet in %s/%s", testName, attempt+1, maxRetries, logGroup, streamName)
	}
	return status.TestResult{Name: testName, Status: status.FAILED, Reason: fmt.Errorf("no log events in %s/%s after %d retries", logGroup, streamName, maxRetries)}
}
