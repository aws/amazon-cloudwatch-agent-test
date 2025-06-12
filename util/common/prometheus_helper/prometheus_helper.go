// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package prometheus_helper

import (
	"fmt"
	"log"
	"os"
	exec2 "os/exec"
	"regexp"
	"strconv"
	"strings"
)

func GetAvalancheParams(metricPerInterval int) (counter, gauge, summary, series, label int) {
	switch metricPerInterval {
	case 1000:
		return 50, 50, 20, 10, 0

	case 5000:
		return 50, 50, 20, 20, 0

	case 10000:
		return 100, 100, 20, 50, 10

	case 50000:
		return 100, 100, 20, 100, 10

	default:
		return 10, 10, 5, 20, 10
	}
}

func CreatePrometheusConfig(prometheusTemplate string, scrapeInterval int) error {
	cfg := prometheusTemplate
	cfg = strings.ReplaceAll(cfg, "$SCRAPE_INTERVAL", fmt.Sprintf("%ds", scrapeInterval))
	cfg = strings.ReplaceAll(cfg, "$PORT", fmt.Sprintf("%d", 8101))

	err := os.WriteFile("/tmp/prometheus.yaml", []byte(cfg), os.ModePerm)
	if err != nil {
		log.Printf("[Prometheus] Failed to write config: %v", err)
		return err
	}

	return nil
}

func CleanupPortPrometheus(port int) {
	killCmd := exec2.Command("sudo", "fuser", "-k", fmt.Sprintf("%d/tcp", port))
	if err := killCmd.Run(); err != nil {
		log.Printf("[Prometheus] Failed to kill port %d: %v", port, err)
	}
}

/*
Behavior:
This method updates the Prometheus-EMF agent JSON to add the instance ID and index to the namespace.
Which allows us to isolate tests from each other by ensuring each test has a unique namespace.
In the case of retries, we want to continue generating a unique namespace.
We start with CloudWatchAgentStress/Prometheus in agent.json config file,
and the following happens on each run:

subsequent runs afterwards:
1. On the first run:
  - Replaces "CloudWatchAgentStress/Prometheus".
  - Replaces it with "CloudWatchAgentStress/Prometheus/{instanceID}/1".

2. On subsequent runs (if stress test retried):
  - Detects an existing namespace in the form "CloudWatchAgentStress/Prometheus/{instanceID}/{index}".
  - Increments the {index} by 1 and replaces it with the new value.

The function writes the updated config back to the file and returns the new namespace used.
*/
func UpdateNamespace(configPath string, instanceID string) string {
	data, err := os.ReadFile(configPath)
	if err != nil {
		fmt.Printf("failed to read agent config: %v\n", err)
		os.Exit(1)
	}

	cfg := string(data)
	baseNamespace := "CloudWatchAgentStress/Prometheus"
	fullPattern := fmt.Sprintf(`%s/%s/(\d+)`, baseNamespace, regexp.QuoteMeta(instanceID))
	fullRegex := regexp.MustCompile(fullPattern)

	matches := fullRegex.FindStringSubmatch(cfg)
	newNamespace := fmt.Sprintf("%s/%s/1", baseNamespace, instanceID)
	searchPattern := baseNamespace

	if len(matches) == 2 {
		oldIndex, _ := strconv.Atoi(matches[1])
		newNamespace = fmt.Sprintf("%s/%s/%d", baseNamespace, instanceID, oldIndex+1)
		searchPattern = fullPattern
	}

	if err := os.WriteFile(configPath, []byte(regexp.MustCompile(searchPattern).ReplaceAllString(cfg, newNamespace)), os.ModePerm); err != nil {
		fmt.Printf("failed to write modified config: %v\n", err)
		os.Exit(1)
	}

	return newNamespace
}
