// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package database_insights_mysql

import (
	"encoding/json"
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
	instanceName    = "dbi-mysql-integ-test"
	workloadDur     = 5 * time.Minute
	serverLogsGroup = "/aws/self-managed-database-insights/mysql/server-logs"
	rawEventsGroup  = "/aws/self-managed-database-insights/mysql/raw-events"
)

func init() {
	environment.RegisterEnvironmentMetaDataFlags()
}

type DbiMysqlTestRunner struct {
	test_runner.BaseTestRunner
	env *environment.MetaData
}

var _ test_runner.ITestRunner = (*DbiMysqlTestRunner)(nil)

func (t *DbiMysqlTestRunner) GetTestName() string { return "DBI_MySQL" }
func (t *DbiMysqlTestRunner) GetAgentConfigFileName() string {
	return "database_insights_mysql_config.json"
}
func (t *DbiMysqlTestRunner) GetAgentRunDuration() time.Duration { return workloadDur }
func (t *DbiMysqlTestRunner) GetMeasuredMetrics() []string {
	return append(append(counterMetrics(), dbLoadMetrics()...), topSQLMetrics()...)
}

func (t *DbiMysqlTestRunner) SetupBeforeAgentRun() error {
	log.Println("=== Running MySQL setup ===")
	out, err := exec.Command("bash", "resources/database_insights_mysql_setup.sh").CombinedOutput()
	log.Printf("setup.sh output:\n%s", string(out))
	if err != nil {
		return fmt.Errorf("setup.sh failed: %w", err)
	}
	return t.BaseTestRunner.SetupBeforeAgentRun()
}

func (t *DbiMysqlTestRunner) SetupAfterAgentRun() error {
	if err := initWorkload(); err != nil {
		return err
	}
	go runWorkload(workloadDur)
	return nil
}

func (t *DbiMysqlTestRunner) Validate() status.TestGroupResult {
	var results []status.TestResult

	// Validate metrics via PromQL with resource attribute labels.
	metricsResult := otlpvalidation.ValidateOtlpMetricsWithLabels(t.GetTestName()+" Metrics", t.env.Region, t.GetMeasuredMetrics(), map[string]string{
		"@resource.db.system.name":   "mysql",
		"@resource.db.instance.name": instanceName,
		"@resource.host.id":          t.env.InstanceId,
	})
	results = append(results, metricsResult.TestResults...)

	// Validate server logs (MySQL error log) in the expected log stream.
	logStream := fmt.Sprintf("%s/%s", t.env.InstanceId, instanceName)
	serverLogsResult := validateLogStream(serverLogsGroup, logStream, "Server Logs")
	results = append(results, serverLogsResult)

	// Validate raw events (query samples / top queries) in the expected log stream.
	rawEventsResult := validateLogStream(rawEventsGroup, logStream, "Raw Events")
	results = append(results, rawEventsResult)

	// Validate mysqld process metrics from the host metrics process scraper.
	processResult := otlpvalidation.ValidateOtlpMetricsWithLabels(t.GetTestName()+" Process Metrics", t.env.Region, processMetrics(), map[string]string{
		"@resource.process.executable.name": "mysqld",
		"@resource.host.id":                 t.env.InstanceId,
	})
	results = append(results, processResult.TestResults...)

	// Validate top query attributes present in the raw-events stream.
	topQueryResult := validateTopQueryAttributes(logStream)

	// Validate query_sample events (DB Load source) present in the raw-events stream.
	querySampleResult := validateQuerySampleEvents(logStream)
	results = append(results, querySampleResult)
	results = append(results, topQueryResult)

	return status.TestGroupResult{Name: t.GetTestName(), TestResults: results}
}

func TestDbiMysql(t *testing.T) {
	env := environment.GetEnvironmentMetaData()

	testRunner := &DbiMysqlTestRunner{
		BaseTestRunner: test_runner.BaseTestRunner{},
		env:            env,
	}
	runner := &test_runner.TestRunner{TestRunner: testRunner}
	result := runner.Run()

	for _, r := range result.TestResults {
		require.Equal(t, status.SUCCESSFUL, r.Status, "%s failed: %v", r.Name, r.Reason)
	}
}

// counterMetrics returns MySQL receiver metrics that are enabled by default in
// the DBI golden YAML and reliably appear on a single-node localhost instance
// with performance_schema enabled. Metrics that are disabled by default
// (e.g. mysql.joins, mysql.sessions, mysql.connection.count,
// mysql.client.network.io) or that require replication
// (e.g. mysql.replica.*) are intentionally excluded.
func counterMetrics() []string {
	return []string{
		"mysql.buffer_pool.data_pages",
		"mysql.buffer_pool.limit",
		"mysql.buffer_pool.operations",
		"mysql.buffer_pool.page_flushes",
		"mysql.buffer_pool.pages",
		"mysql.buffer_pool.usage",
		"mysql.double_writes",
		"mysql.handlers",
		"mysql.index.io.wait.count",
		"mysql.index.io.wait.time",
		"mysql.locks",
		"mysql.log_operations",
		"mysql.mysqlx_connections",
		"mysql.opened_resources",
		"mysql.operations",
		"mysql.page_operations",
		"mysql.prepared_statements",
		"mysql.row_locks",
		"mysql.row_operations",
		"mysql.sorts",
		"mysql.threads",
		"mysql.table.io.wait.count",
		"mysql.table.io.wait.time",
		"mysql.tmp_resources",
		"mysql.uptime",
	}
}

// dbLoadMetrics returns the 7 DB Load metrics produced by the count/dbi_dbload_mysql
// connector from performance_schema.threads + events_statements_current snapshots.
func dbLoadMetrics() []string {
	return []string{
		"mysql.active_sessions.by_wait",
		"mysql.active_sessions.by_user",
		"mysql.active_sessions.by_db",
		"mysql.active_sessions.by_sql",
		"mysql.active_sessions.by_sql_wait",
		"mysql.active_sessions.by_host",
		"mysql.active_sessions.count",
	}
}

// topSQLMetrics returns Top SQL metrics produced by the
// signaltometrics/dbi_topsql_mysql connector from
// events_statements_summary_by_digest.
func topSQLMetrics() []string {
	return []string{
		"mysql.count_star",
		"mysql.sum_timer_wait",
		"mysql.sum_lock_time",
		"mysql.sum_rows_sent",
		"mysql.sum_rows_examined",
		"mysql.sum_errors",
		"mysql.sum_sort_rows",
		"mysql.sum_created_tmp_tables",
		"mysql.sum_created_tmp_disk_tables",
		"mysql.sum_no_index_used",
		"mysql.sum_select_full_join",
		"mysql.sum_sort_scan",
		"mysql.sum_no_good_index_used",
		"mysql.sum_select_scan",
	}
}

func processMetrics() []string {
	return []string{
		"process.cpu.utilization",
		"process.memory.utilization",
	}
}

func initWorkload() error {
	if _, err := exec.LookPath("sysbench"); err != nil {
		log.Println("sysbench not found, will use mysqlslap fallback (no prepare needed)")
		return nil
	}
	log.Println("=== Initializing sysbench tables ===")
	out, err := exec.Command("sysbench",
		"oltp_read_write",
		"--mysql-host=127.0.0.1",
		"--mysql-port=3306",
		"--mysql-user=sysbench",
		"--mysql-password=sysbench",
		"--mysql-db=testdb",
		"--tables=4",
		"--table-size=10000",
		"prepare",
	).CombinedOutput()
	log.Printf("sysbench prepare output:\n%s", string(out))
	if err != nil {
		return fmt.Errorf("sysbench prepare failed: %w", err)
	}
	return nil
}

func runWorkload(duration time.Duration) {
	seconds := fmt.Sprintf("%d", int(duration.Seconds()))

	if _, err := exec.LookPath("sysbench"); err == nil {
		log.Printf("=== Running sysbench for %s seconds with 10 threads ===", seconds)
		out, err := exec.Command("sysbench",
			"oltp_read_write",
			"--mysql-host=127.0.0.1",
			"--mysql-port=3306",
			"--mysql-user=sysbench",
			"--mysql-password=sysbench",
			"--mysql-db=testdb",
			"--tables=4",
			"--table-size=10000",
			"--threads=10",
			"--time="+seconds,
			"--report-interval=10",
			"run",
		).CombinedOutput()
		if err != nil {
			log.Printf("sysbench failed: %v, output: %s", err, string(out))
			return
		}
		log.Printf("sysbench output:\n%s", string(out))
		return
	}

	// Fallback: use mysqlslap to drive concurrent activity.
	log.Println("=== Running mysqlslap for load generation ===")
	out, err := exec.Command("mysqlslap",
		"--host=127.0.0.1",
		"--user=root",
		"--concurrency=10",
		"--iterations=100",
		"--auto-generate-sql",
		"--auto-generate-sql-load-type=mixed",
	).CombinedOutput()
	if err != nil {
		log.Printf("mysqlslap failed: %v, output: %s", err, string(out))
		return
	}
	log.Printf("mysqlslap output:\n%s", string(out))
}

// validateLogStream checks that a specific log stream in a CloudWatch Logs log
// group contains at least one event.
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

// validateTopQueryAttributes fetches the latest log event from the raw-events
// log group and asserts it is a db.server.top_query event carrying the expected
// events_statements_summary_by_digest attributes.
func validateTopQueryAttributes(streamName string) status.TestResult {
	const testName = "Top Query Attributes"
	const maxRetries = 3
	const retryInterval = 30 * time.Second

	requiredAttrs := []string{
		"db.system.name",
		"db.namespace",
		"db.query.text",
		"mysql.events_statements_summary_by_digest.digest",
		"mysql.events_statements_summary_by_digest.count_star",
		"mysql.events_statements_summary_by_digest.sum_timer_wait",
		"mysql.events_statements_summary_by_digest.sum_lock_time",
		"mysql.events_statements_summary_by_digest.sum_rows_sent",
		"mysql.events_statements_summary_by_digest.sum_rows_examined",
		"mysql.events_statements_summary_by_digest.sum_errors",
		"mysql.events_statements_summary_by_digest.sum_sort_rows",
		"mysql.events_statements_summary_by_digest.sum_created_tmp_tables",
		"mysql.events_statements_summary_by_digest.sum_created_tmp_disk_tables",
		"mysql.events_statements_summary_by_digest.sum_no_index_used",
		"mysql.events_statements_summary_by_digest.sum_select_full_join",
		"mysql.events_statements_summary_by_digest.sum_sort_scan",
		"mysql.events_statements_summary_by_digest.sum_no_good_index_used",
		"mysql.events_statements_summary_by_digest.sum_select_scan",
	}

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(retryInterval)
		}

		events, err := awsservice.GetLogsSince(rawEventsGroup, streamName, nil, nil)
		if err != nil || len(events) == 0 {
			log.Printf("[%s] Attempt %d/%d: no events available", testName, attempt+1, maxRetries)
			continue
		}

		latestEvent := events[len(events)-1]
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(*latestEvent.Message), &parsed); err != nil {
			log.Printf("[%s] Attempt %d/%d: failed to parse JSON: %v", testName, attempt+1, maxRetries, err)
			continue
		}

		eventName, _ := parsed["eventName"].(string)
		if eventName != "db.server.top_query" {
			log.Printf("[%s] Attempt %d/%d: eventName=%q, want db.server.top_query", testName, attempt+1, maxRetries, eventName)
			continue
		}

		attrs, _ := parsed["attributes"].(map[string]interface{})
		if attrs == nil {
			log.Printf("[%s] Attempt %d/%d: no attributes map found", testName, attempt+1, maxRetries)
			continue
		}

		var missing []string
		for _, attr := range requiredAttrs {
			if _, ok := attrs[attr]; !ok {
				missing = append(missing, attr)
			}
		}
		if len(missing) > 0 {
			log.Printf("[%s] Attempt %d/%d: missing attributes: %v", testName, attempt+1, maxRetries, missing)
			continue
		}

		return status.TestResult{Name: testName, Status: status.SUCCESSFUL}
	}

	return status.TestResult{
		Name:   testName,
		Status: status.FAILED,
		Reason: fmt.Errorf("top query attributes validation failed after %d retries", maxRetries),
	}
}

// validateQuerySampleEvents checks that at least one db.server.query_sample
// event exists in the raw-events log stream. These events are the source of
// DB Load data and validate the events_statements_current + threads pipeline.
func validateQuerySampleEvents(streamName string) status.TestResult {
	const testName = "Query Sample Events"
	const maxRetries = 3
	const retryInterval = 30 * time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(retryInterval)
		}

		events, err := awsservice.GetLogsSince(rawEventsGroup, streamName, nil, nil)
		if err != nil || len(events) == 0 {
			log.Printf("[%s] Attempt %d/%d: no events available", testName, attempt+1, maxRetries)
			continue
		}

		for _, event := range events {
			var parsed map[string]interface{}
			if err := json.Unmarshal([]byte(*event.Message), &parsed); err != nil {
				continue
			}
			if eventName, _ := parsed["eventName"].(string); eventName == "db.server.query_sample" {
				log.Printf("[%s] Found query_sample event in %s/%s", testName, rawEventsGroup, streamName)
				return status.TestResult{Name: testName, Status: status.SUCCESSFUL}
			}
		}

		log.Printf("[%s] Attempt %d/%d: no query_sample events found yet", testName, attempt+1, maxRetries)
	}

	return status.TestResult{
		Name:   testName,
		Status: status.FAILED,
		Reason: fmt.Errorf("no db.server.query_sample events found in %s/%s after %d retries", rawEventsGroup, streamName, maxRetries),
	}
}
