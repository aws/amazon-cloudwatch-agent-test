// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

// Package database_insights_plans checks that the self-managed PostgreSQL Database
// Insights receiver starts collecting query plans after pg_store_plans is set up
// while the agent is already running, without an agent restart. It covers both
// orders a customer can follow, one database each:
//
//   - testdb: preload the library and restart PostgreSQL first, then CREATE EXTENSION.
//     The monitoring user cannot read the plan store at first, and is granted SELECT later.
//   - testdb2: CREATE EXTENSION first, then preload the library and restart PostgreSQL.
//     Until PostgreSQL restarts, the extension exists but every plan read fails.
//
// Timeline, measured from the agent start:
//  1. pg_store_plans is installed but not preloaded and not created in either database
//     (setup script). The agent finds it missing, warns once per receiver and database,
//     and skips plan lookups. Top query events are still reported, with an empty plan.
//  2. The test runs CREATE EXTENSION in testdb2 while the library is not preloaded. The
//     receiver must warn once that the extension is created but not loaded, must not log
//     that it resumed, and must keep reporting top queries without plans.
//  3. The test preloads pg_store_plans and restarts PostgreSQL while the agent runs. The
//     agent must keep running and keep reporting top queries across the restart.
//  4. The receiver resumes testdb2 on its own, and top queries seen before the restart
//     now carry plans. Nothing was cached as an empty plan while the library was missing.
//  5. testdb still lacks the extension. Its warning count stays the same across the
//     re-checks and the PostgreSQL restart.
//  6. The test runs CREATE EXTENSION in testdb and, in the same transaction, revokes
//     SELECT on the plan store from PUBLIC and the monitoring user. The extension and the
//     library are fine, so only the permission probe can notice. The receiver must warn
//     once that the monitoring user cannot read it, naming the grant, must not warn again
//     on later collections, and must not log that it resumed. testdb2 is untouched and
//     stays quiet. The revoke is part of the creation because the receiver probes the plan
//     store only for queries whose plan is not cached, and it caches plans for an hour:
//     once plans are cached, a later revoke is not noticed.
//  7. The test grants SELECT back, and the receiver resumes testdb without an agent
//     restart. Top queries carry plans from then on.
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
	// workloadDur is the agent run time. A passing run needs about 15 minutes; see the
	// step durations below.
	workloadDur = 18 * time.Minute

	instanceName   = "dbi-plans-integ-test"
	rawEventsGroup = "/aws/self-managed-database-insights/postgresql/raw-events"
	agentLogPath   = "/opt/aws/amazon-cloudwatch-agent/logs/amazon-cloudwatch-agent.log"

	workloadUser     = "dbi_app"
	workloadPassword = "dbi_app_password"

	// dbLate gets the extension after the library is preloaded. dbEarly gets it before.
	dbLate  = "testdb"
	dbEarly = "testdb2"

	// Log messages of the receiver (postgresqlreceiver, queryPlanStoreReady).
	missingMsg   = "pg_store_plans extension not installed"
	notLoadedMsg = "pg_store_plans extension is created but not loaded"
	noAccessMsg  = "pg_store_plans extension is created but the monitoring user cannot read it"
	resumedMsg   = "pg_store_plans is now readable"

	// monitorUser is the role the agent connects as (database_insights_setup.sh).
	monitorUser = "cw_monitor"
	// revokePlanStoreSQL takes SELECT on the two views the receiver reads away from everyone, and
	// grantPlanStoreSQL gives it back the way CREATE EXTENSION leaves it. psql runs the statements
	// of one -c string in a single transaction, so CREATE EXTENSION followed by the revoke never
	// shows the receiver a readable plan store.
	revokePlanStoreSQL = "REVOKE SELECT ON pg_store_plans_info, pg_store_plans FROM PUBLIC, " + monitorUser
	grantPlanStoreSQL  = "GRANT SELECT ON pg_store_plans_info, pg_store_plans TO PUBLIC"

	// firstWarnTimeout is how long to wait for the first plan lookup to find the
	// extension missing. The first top query collection runs when the agent starts.
	firstWarnTimeout = 3 * time.Minute
	// notLoadedTimeout is how long to wait for the receiver to notice that the extension
	// exists but its library is not loaded. A check runs on every top query collection.
	notLoadedTimeout = 3 * time.Minute
	// settleAfterWarn lets the other receiver instance do its first collection, so
	// that every instance has warned before the warnings are counted.
	settleAfterWarn = 70 * time.Second
	// restartTimeout bounds the preload script, which restarts PostgreSQL and waits
	// until it accepts connections again.
	restartTimeout = 3 * time.Minute
	// noAccessTimeout is how long to wait for the receiver to notice that the monitoring user cannot
	// read the plan store. A check runs on every top query collection.
	noAccessTimeout = 3 * time.Minute
	// noAccessStillWindow is how long testdb stays without access after the warning was counted. The
	// receiver re-checks on every top query collection (once a minute by default), so this covers at
	// least one re-check, which must not warn again.
	noAccessStillWindow = 70 * time.Second
	// stillMissingWindow is how long testdb stays without the extension after the
	// PostgreSQL restart. The receiver re-checks on every top query collection (once a
	// minute by default), so this covers at least one re-check, which must not warn again.
	stillMissingWindow = 130 * time.Second
	// workloadRunSeconds is the length of one pgbench run. The workload is a series
	// of runs because a single run does not survive the PostgreSQL restart.
	workloadRunSeconds = 20

	// workloadTable appears in the text of every statement pgbench runs. The checks that need events
	// to exist count only these events, so the receiver's own statements in these databases cannot
	// satisfy them when the workload's queries get no plan.
	workloadTable = "pgbench_"
	// minPlannedQueries is how many distinct workload queries must carry a plan once plans can be
	// read. The pgbench script runs five statements that get plans (its BEGIN and END are utility
	// statements and never do). Requiring several instead of one fails when plans stay empty for
	// most of the workload, for example when empty plans were cached for the queries.
	minPlannedQueries = 3

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
	// missingBefore and missingStill are the missing-extension warning counts per
	// database taken before and after stillMissingWindow. testdb is still missing
	// throughout, and testdb2 is not missing after its extension was created.
	missingBefore map[string]int
	missingStill  map[string]int
	// notLoadedBefore and notLoadedStill are the created-but-not-loaded warning counts
	// for testdb2 before the PostgreSQL restart and after stillMissingWindow.
	notLoadedBefore int
	notLoadedStill  int
	// resumedEarly counts resume messages for testdb2 while its library was not loaded.
	// It must stay 0, because nothing can be read in that state.
	resumedEarly int
	// restartedAt is when PostgreSQL was back after the preload restart.
	restartedAt time.Time
	// agentStartsBefore and agentStartsAfter count the agent start messages before the
	// PostgreSQL restart and at the end of the lifecycle.
	agentStartsBefore int
	agentStartsAfter  int
	// grantedAt is when SELECT on the plan store was granted back to the monitoring user in testdb,
	// stamped before the GRANT. Plans cannot be read before it, so it is the cut-off for testdb.
	grantedAt time.Time
	// noAccessBefore and noAccessStill are the no-access warning counts for testdb before and after
	// noAccessStillWindow. noAccessOther is the count for testdb2, which keeps its access.
	noAccessBefore, noAccessStill, noAccessOther int
	// resumedNoAccess counts resume messages for testdb while the monitoring user could not read the
	// plan store. It must stay 0, because nothing can be read in that state.
	resumedNoAccess int
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
	// preloading or creating it, and creates the workload role. Both share one deadline.
	for _, script := range []string{
		"../database_insights/resources/database_insights_setup.sh",
		"resources/database_insights_plans_setup.sh",
	} {
		log.Printf("=== Running %s ===", script)
		if err := runScript(ctx, script, setupTimeout); err != nil {
			return err
		}
	}
	return t.BaseTestRunner.SetupBeforeAgentRun()
}

func (t *DbiPlansTestRunner) SetupAfterAgentRun() error {
	for _, db := range []string{dbLate, dbEarly} {
		if err := initWorkload(db); err != nil {
			return err
		}
	}
	for _, db := range []string{dbLate, dbEarly} {
		go runWorkload(db, workloadDur)
	}
	go t.driveExtensionLifecycle()
	return nil
}

// driveExtensionLifecycle runs the steps in the package comment and records what the
// agent log showed at each of them.
func (t *DbiPlansTestRunner) driveExtensionLifecycle() {
	defer close(t.done)
	life := lifecycle{missingBefore: map[string]int{}, missingStill: map[string]int{}}
	defer func() {
		t.mu.Lock()
		t.life = life
		t.mu.Unlock()
	}()

	// 1. Both databases are reported as missing.
	for _, db := range []string{dbLate, dbEarly} {
		if err := waitForAgentLog(missingMsg, db, firstWarnTimeout); err != nil {
			life.err = fmt.Errorf("the agent never reported pg_store_plans as missing in %s; the agent build may not contain the pg_store_plans plan lookup, or the workload role was skipped: %w", db, err)
			return
		}
	}
	log.Println("=== The agent reported pg_store_plans as missing in both databases ===")

	// Let the other receiver instance do its first collection, so that every instance
	// has warned before the warnings are counted.
	time.Sleep(settleAfterWarn)
	var err error
	for _, db := range []string{dbLate, dbEarly} {
		if life.missingBefore[db], err = countAgentLog(missingMsg, db); err != nil {
			life.err = err
			return
		}
	}

	// 2. Create the extension in testdb2 while the library is not preloaded.
	if err = createExtension(dbEarly); err != nil {
		life.err = err
		return
	}
	if err = waitForAgentLog(notLoadedMsg, dbEarly, notLoadedTimeout); err != nil {
		life.err = fmt.Errorf("the agent did not report that pg_store_plans is created but not loaded in %s: %w", dbEarly, err)
		return
	}
	log.Println("=== The agent reported pg_store_plans as created but not loaded ===")
	time.Sleep(settleAfterWarn)
	if life.notLoadedBefore, err = countAgentLog(notLoadedMsg, dbEarly); err != nil {
		life.err = err
		return
	}
	if life.resumedEarly, err = countAgentLog(resumedMsg, dbEarly); err != nil {
		life.err = err
		return
	}
	if life.agentStartsBefore, err = countAgentLog(agentStartMsg, ""); err != nil {
		life.err = err
		return
	}

	// 3. Preload the library and restart PostgreSQL while the agent is running. Stamp the
	// time before the restart, so a top query collection that lands while PostgreSQL is
	// coming back is counted as after the restart, never before it.
	life.restartedAt = time.Now()
	if err = preloadAndRestartPostgres(); err != nil {
		life.err = err
		return
	}
	log.Println("=== PostgreSQL restarted with pg_store_plans preloaded; the agent keeps running ===")

	// 4. testdb2 resumes without any further action.
	if err = waitForAgentLog(resumedMsg, dbEarly, resumeTimeout); err != nil {
		life.err = fmt.Errorf("the agent did not resume plan collection in %s after PostgreSQL restarted with the library: %w", dbEarly, err)
		return
	}
	log.Println("=== The agent resumed plan collection in the database whose extension was created first ===")

	// 5. testdb is still missing the extension, and nothing warns again.
	time.Sleep(stillMissingWindow)
	for _, db := range []string{dbLate, dbEarly} {
		if life.missingStill[db], err = countAgentLog(missingMsg, db); err != nil {
			life.err = err
			return
		}
	}
	if life.notLoadedStill, err = countAgentLog(notLoadedMsg, dbEarly); err != nil {
		life.err = err
		return
	}

	// 6. Create the extension in testdb and revoke SELECT on the plan store from PUBLIC and the
	// monitoring user in the same transaction. The receiver has no cached plan for testdb, so its
	// next collection probes the plan store and finds that it cannot read it. The extension makes
	// both views readable by everyone, so the revoke covers PUBLIC as well as the role.
	if err = runPsql(dbLate, "CREATE EXTENSION pg_store_plans; "+revokePlanStoreSQL); err != nil {
		life.err = err
		return
	}
	if err = waitForAgentLog(noAccessMsg, dbLate, noAccessTimeout); err != nil {
		life.err = fmt.Errorf("the agent did not report that the monitoring user cannot read pg_store_plans in %s: %w", dbLate, err)
		return
	}
	log.Println("=== The agent reported that the monitoring user cannot read pg_store_plans ===")
	time.Sleep(settleAfterWarn)
	if life.noAccessBefore, err = countAgentLog(noAccessMsg, dbLate); err != nil {
		life.err = err
		return
	}
	time.Sleep(noAccessStillWindow)
	if life.noAccessStill, err = countAgentLog(noAccessMsg, dbLate); err != nil {
		life.err = err
		return
	}
	if life.noAccessOther, err = countAgentLog(noAccessMsg, dbEarly); err != nil {
		life.err = err
		return
	}
	if life.resumedNoAccess, err = countAgentLog(resumedMsg, dbLate); err != nil {
		life.err = err
		return
	}

	// 7. Grant SELECT back. The receiver resumes testdb on its own. Stamp the time before the
	// GRANT, so an event from a collection straddling it counts as after, never before it.
	life.grantedAt = time.Now()
	if err = runPsql(dbLate, grantPlanStoreSQL); err != nil {
		life.err = err
		return
	}
	if err = waitForAgentLog(resumedMsg, dbLate, resumeTimeout); err != nil {
		life.err = fmt.Errorf("the agent did not resume plan collection in %s after SELECT was granted back; it needs a restart to notice it: %w", dbLate, err)
		return
	}
	log.Println("=== The agent resumed plan collection in both databases ===")

	time.Sleep(exportWait)
	if life.agentStartsAfter, err = countAgentLog(agentStartMsg, ""); err != nil {
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

	const lifecycleName = "Plan lookups skipped while pg_store_plans unavailable, resumed once it can be read"
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
	// the re-checks that still find it missing. testdb2 has the extension by then, so
	// its count must not grow either.
	for _, db := range []string{dbLate, dbEarly} {
		name := fmt.Sprintf("Missing-extension warning is not repeated in %s", db)
		if life.missingBefore[db] < 1 || life.missingStill[db] != life.missingBefore[db] {
			results = append(results, failed(name,
				fmt.Errorf("warnings before the still-missing window: %d, after: %d; want at least 1 and no change", life.missingBefore[db], life.missingStill[db])))
		} else {
			results = append(results, passed(name))
		}
	}

	// The extension exists but the library is not loaded: warn once per receiver
	// instance, and do not claim that plan collection resumed.
	const notLoadedName = "Created-but-not-loaded warning is reported once"
	if life.notLoadedBefore < 1 || life.notLoadedStill != life.notLoadedBefore {
		results = append(results, failed(notLoadedName,
			fmt.Errorf("warnings before the PostgreSQL restart: %d, after the still-missing window: %d; want at least 1 and no change", life.notLoadedBefore, life.notLoadedStill)))
	} else {
		results = append(results, passed(notLoadedName))
	}
	const noFalseResumeName = "No resume reported while the library was not loaded"
	if life.resumedEarly != 0 {
		results = append(results, failed(noFalseResumeName,
			fmt.Errorf("%d resume messages for %s before the library was preloaded; want 0", life.resumedEarly, dbEarly)))
	} else {
		results = append(results, passed(noFalseResumeName))
	}

	// Every receiver instance that warned about a database also reports that it resumed. In testdb the
	// receiver goes from missing to no access, and resumes once, after SELECT was granted back.
	for _, db := range []string{dbLate, dbEarly} {
		name := fmt.Sprintf("Every warned receiver resumed in %s", db)
		warned, werr := countAgentLog(missingMsg, db)
		resumed, rerr := countAgentLog(resumedMsg, db)
		switch {
		case werr != nil || rerr != nil:
			results = append(results, failed(name, errors.Join(werr, rerr)))
		case warned != resumed:
			results = append(results, failed(name,
				fmt.Errorf("%d missing-extension warnings but %d resume messages", warned, resumed)))
		default:
			results = append(results, passed(name))
		}
	}

	// The monitoring user cannot read the plan store: warn once per receiver instance, naming the
	// grant, only for the database where that is so, and do not claim that plan collection resumed.
	const noAccessName = "No-access warning is reported once"
	if life.noAccessBefore < 1 || life.noAccessStill != life.noAccessBefore {
		results = append(results, failed(noAccessName,
			fmt.Errorf("warnings for %s before the still-without-access window: %d, after: %d; want at least 1 and no change", dbLate, life.noAccessBefore, life.noAccessStill)))
	} else {
		results = append(results, passed(noAccessName))
	}
	const noAccessScopeName = "No-access warning is limited to the database where the user cannot read the plan store"
	if life.noAccessOther != 0 {
		results = append(results, failed(noAccessScopeName,
			fmt.Errorf("%d no-access warnings for %s, where the user can read the plan store; want 0", life.noAccessOther, dbEarly)))
	} else {
		results = append(results, passed(noAccessScopeName))
	}
	const noAccessResumeName = "No resume reported while the monitoring user could not read the plan store"
	if life.resumedNoAccess != 0 {
		results = append(results, failed(noAccessResumeName,
			fmt.Errorf("%d resume messages for %s before SELECT was granted back; want 0", life.resumedNoAccess, dbLate)))
	} else {
		results = append(results, passed(noAccessResumeName))
	}

	results = append(results, t.validatePlanEvents(life)...)
	return status.TestGroupResult{Name: t.GetTestName(), TestResults: results}
}

// planCounts classifies the top query events of one database.
type planCounts struct {
	// emptyBefore counts workload events before the cut-off without a plan. plansBefore counts
	// events of any query before the cut-off with a plan, which must not exist.
	emptyBefore, plansBefore int
	// emptyAfterRestart counts workload events without a plan between the PostgreSQL restart and the cut-off.
	emptyAfterRestart int
	// plansAfter counts workload events with a plan at or after the cut-off.
	plansAfter int
	// plannedQueries holds the distinct query IDs of those events.
	plannedQueries map[string]struct{}
}

// validatePlanEvents checks the raw events stream. In testdb, top query events were
// reported without plans while the extension was missing and while the monitoring user
// could not read it, top queries kept being reported after the PostgreSQL restart, none
// carried a plan before SELECT was granted, and several distinct workload queries did
// afterwards. In testdb2, no event
// carried a plan while the library was not loaded, and several distinct workload queries
// carried plans after PostgreSQL restarted with it.
func (t *DbiPlansTestRunner) validatePlanEvents(life lifecycle) []status.TestResult {
	const (
		maxRetries    = 5
		retryInterval = 30 * time.Second
	)
	stream := fmt.Sprintf("%s/%s", t.env.InstanceId, instanceName)

	var (
		late, early planCounts
		lastErr     error
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
		late, early = planCounts{}, planCounts{}
		for _, e := range events {
			if e.Message == nil || e.Timestamp == nil {
				continue
			}
			q, isTopQuery := parseTopQuery(*e.Message)
			if !isTopQuery {
				continue
			}
			at := time.UnixMilli(*e.Timestamp)
			switch q.database {
			case dbLate:
				// The cut-off is the grant: plans cannot be read before the monitoring user may read the plan store.
				late.add(at, q, life.restartedAt, life.grantedAt)
			case dbEarly:
				// The cut-off is the PostgreSQL restart: plans cannot be read before it.
				early.add(at, q, life.restartedAt, life.restartedAt)
			}
		}
		log.Printf("[plan events] attempt %d/%d: %s %+v, %s %+v", attempt+1, maxRetries, dbLate, late, dbEarly, early)
		if len(late.plannedQueries) >= minPlannedQueries && len(early.plannedQueries) >= minPlannedQueries {
			break
		}
	}

	var results []status.TestResult
	if lastErr != nil {
		return append(results, failed("Top query events reported", lastErr))
	}
	if late.emptyBefore == 0 {
		results = append(results, failed("Top queries reported without plans while plans could not be read",
			fmt.Errorf("no %s top query event with an empty plan in %s/%s before SELECT was granted (%s)", dbLate, rawEventsGroup, stream, life.grantedAt.Format(time.RFC3339))))
	} else {
		results = append(results, passed("Top queries reported without plans while plans could not be read"))
	}
	if late.emptyAfterRestart == 0 {
		results = append(results, failed("Top queries still reported after the PostgreSQL restart",
			fmt.Errorf("no %s top query event between the PostgreSQL restart (%s) and SELECT being granted (%s)",
				dbLate, life.restartedAt.Format(time.RFC3339), life.grantedAt.Format(time.RFC3339))))
	} else {
		results = append(results, passed("Top queries still reported after the PostgreSQL restart"))
	}
	if late.plansBefore > 0 {
		results = append(results, failed("No plans before the monitoring user could read the plan store",
			fmt.Errorf("%d %s events carried a plan before SELECT was granted (%s)", late.plansBefore, dbLate, life.grantedAt.Format(time.RFC3339))))
	} else {
		results = append(results, passed("No plans before the monitoring user could read the plan store"))
	}
	if len(late.plannedQueries) < minPlannedQueries {
		results = append(results, failed("Plans reported after SELECT was granted back",
			fmt.Errorf("%d distinct %s workload queries carried a plan after SELECT was granted (%s, %d events); want at least %d",
				len(late.plannedQueries), dbLate, life.grantedAt.Format(time.RFC3339), late.plansAfter, minPlannedQueries)))
	} else {
		results = append(results, passed("Plans reported after SELECT was granted back"))
	}

	if early.emptyBefore == 0 || early.plansBefore > 0 {
		results = append(results, failed("No plans while the extension existed without its library",
			fmt.Errorf("%s events before the PostgreSQL restart (%s): %d without a plan, %d with a plan; want some without and none with",
				dbEarly, life.restartedAt.Format(time.RFC3339), early.emptyBefore, early.plansBefore)))
	} else {
		results = append(results, passed("No plans while the extension existed without its library"))
	}
	if len(early.plannedQueries) < minPlannedQueries {
		results = append(results, failed("Plans reported after PostgreSQL restarted with the library",
			fmt.Errorf("%d distinct %s workload queries carried a plan after the PostgreSQL restart (%s, %d events); want at least %d. Queries seen before the restart may have been cached without a plan",
				len(early.plannedQueries), dbEarly, life.restartedAt.Format(time.RFC3339), early.plansAfter, minPlannedQueries)))
	} else {
		results = append(results, passed("Plans reported after PostgreSQL restarted with the library"))
	}
	return results
}

// add classifies one top query event. Events before cutoff are counted as before, the
// rest as after. Events without a plan between restartedAt and cutoff are also counted.
// Only workload events (see isWorkloadQuery) count towards the checks that need events to
// exist, so the receiver's own statements cannot satisfy them. Any event with a plan
// before cutoff is counted, because no query may have one then.
func (c *planCounts) add(at time.Time, q topQuery, restartedAt, cutoff time.Time) {
	before := at.Before(cutoff)
	switch {
	case before && q.plan != "":
		c.plansBefore++
	case !isWorkloadQuery(q.text):
	case before:
		c.emptyBefore++
		if at.After(restartedAt) {
			c.emptyAfterRestart++
		}
	case q.plan != "":
		c.plansAfter++
		if c.plannedQueries == nil {
			c.plannedQueries = map[string]struct{}{}
		}
		c.plannedQueries[q.queryID] = struct{}{}
	}
}

// isWorkloadQuery reports whether a statement was run by the pgbench workload.
func isWorkloadQuery(text string) bool { return strings.Contains(text, workloadTable) }

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

// topQuery is the part of a db.server.top_query raw event that the checks use.
type topQuery struct {
	// database is db.namespace, text is db.query.text and queryID is postgresql.queryid.
	database, text, queryID string
	// plan is postgresql.query_plan, which is empty when no plan was collected.
	plan string
}

// parseTopQuery returns the top query a raw events record describes. Records are
// single-line OTLP JSON with flat dotted attribute keys. isTopQuery is false for records
// that do not carry the plan attribute, such as query samples.
func parseTopQuery(message string) (q topQuery, isTopQuery bool) {
	var record struct {
		Attributes map[string]any `json:"attributes"`
	}
	if err := json.Unmarshal([]byte(message), &record); err != nil {
		return topQuery{}, false
	}
	value, ok := record.Attributes["postgresql.query_plan"]
	if !ok {
		return topQuery{}, false
	}
	q.plan, _ = value.(string)
	q.database, _ = record.Attributes["db.namespace"].(string)
	q.text, _ = record.Attributes["db.query.text"].(string)
	if id, ok := record.Attributes["postgresql.queryid"]; ok {
		q.queryID = fmt.Sprint(id)
	}
	return q, true
}

// preloadAndRestartPostgres preloads pg_store_plans and restarts PostgreSQL. It runs
// while the agent is running.
func preloadAndRestartPostgres() error {
	ctx, cancel := context.WithTimeout(context.Background(), restartTimeout)
	defer cancel()
	return runScript(ctx, "resources/database_insights_plans_preload.sh", restartTimeout)
}

// runScript runs a bash script, logging its combined output, and returns a wrapped
// error. ctx owns the deadline; a deadline hit is reported with the timeout it was
// given. WaitDelay lets Wait return shortly after a kill even if orphaned sudo
// children keep the output pipe open.
func runScript(ctx context.Context, script string, timeout time.Duration) error {
	cmd := exec.CommandContext(ctx, "bash", script)
	cmd.WaitDelay = 10 * time.Second
	out, err := cmd.CombinedOutput()
	log.Printf("%s output:\n%s", script, string(out))
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("%s timed out after %v; output above shows the last step reached", script, timeout)
	}
	if err != nil {
		return fmt.Errorf("%s failed: %w", script, err)
	}
	return nil
}

func createExtension(database string) error {
	return runPsql(database, "CREATE EXTENSION pg_store_plans")
}

// runPsql runs one SQL statement in database as the postgres superuser, logging its output.
func runPsql(database, statement string) error {
	out, err := exec.Command("sudo", "-u", "postgres", "psql", "-d", database,
		"-v", "ON_ERROR_STOP=1", "-c", statement).CombinedOutput()
	log.Printf("%q in %s output:\n%s", statement, database, string(out))
	if err != nil {
		return fmt.Errorf("%q in %s failed: %w", statement, database, err)
	}
	return nil
}

// countAgentLog returns how many lines of the agent log contain message. When database
// is not empty, it counts only the lines that carry that database as a receiver log
// field, which the agent writes as compact JSON, for example "database":"testdb".
func countAgentLog(message, database string) (int, error) {
	out, err := exec.Command("sudo", "grep", "-F", message, agentLogPath).Output()
	if err != nil {
		// grep exits 1 when nothing matched; any other failure is a real error.
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			return 0, fmt.Errorf("grep %s: %w", agentLogPath, err)
		}
		return 0, nil
	}
	return countLines(string(out), database), nil
}

// countLines counts the non-empty lines of out, or only the ones that carry database
// as a log field when database is not empty. The closing quote keeps testdb from
// matching testdb2.
func countLines(out, database string) int {
	needle := fmt.Sprintf("%q:%q", "database", database)
	n := 0
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line != "" && (database == "" || strings.Contains(line, needle)) {
			n++
		}
	}
	return n
}

// waitForAgentLog polls the agent log until it contains message, for database when given.
func waitForAgentLog(message, database string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		n, err := countAgentLog(message, database)
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

// pgbench runs pgbench over TCP as the non-superuser workload role.
func pgbench(args ...string) *exec.Cmd {
	cmd := exec.Command("pgbench", append([]string{"-h", "127.0.0.1", "-U", workloadUser}, args...)...)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+workloadPassword)
	return cmd
}

func initWorkload(database string) error {
	log.Printf("=== Initializing pgbench tables in %s ===", database)
	out, err := pgbench("-i", database).CombinedOutput()
	log.Printf("pgbench init output:\n%s", string(out))
	if err != nil {
		return fmt.Errorf("pgbench init in %s failed: %w", database, err)
	}
	return nil
}

// runWorkload runs pgbench in short runs until the duration has passed. A run that
// fails, which happens while PostgreSQL restarts, is logged and followed by the next.
func runWorkload(database string, duration time.Duration) {
	end := time.Now().Add(duration)
	log.Printf("=== Running pgbench in %s in %d second runs for %s ===", database, workloadRunSeconds, duration)
	for time.Now().Before(end) {
		out, err := pgbench("-c", "10", "-j", "2", "-T", strconv.Itoa(workloadRunSeconds), database).CombinedOutput()
		if err != nil {
			log.Printf("pgbench run failed, retrying in 5 seconds: %v, output: %s", err, string(out))
			time.Sleep(5 * time.Second)
		}
	}
	log.Printf("=== The %s workload finished ===", database)
}
