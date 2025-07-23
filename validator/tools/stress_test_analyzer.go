// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

// StressTestResult represents a single metric validation result
type StressTestResult struct {
	Timestamp    time.Time
	MetricName   string
	Namespace    string
	Value        float64
	UpperBound   float64
	SampleCount  int
	ExpectedMin  int
	ExpectedMax  int
	IsSuccessful bool
	ErrorMessage string
}

// TestSummary represents the overall test summary
type TestSummary struct {
	TestCase     string
	ValidateType string
	StartTime    time.Time
	EndTime      time.Time
	Results      []StressTestResult
	Successful   bool
	ErrorMessage string
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run stress_test_analyzer.go <log_file_path>")
		os.Exit(1)
	}

	logFilePath := os.Args[1]
	summary, err := analyzeLogFile(logFilePath)
	if err != nil {
		fmt.Printf("Error analyzing log file: %v\n", err)
		os.Exit(1)
	}

	printSummary(summary)
}

func analyzeLogFile(filePath string) (*TestSummary, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	summary := &TestSummary{
		Results:    make([]StressTestResult, 0),
		Successful: true,
	}

	scanner := bufio.NewScanner(file)
	
	// Regular expressions for parsing log lines
	testCaseRegex := regexp.MustCompile(`test case: (.*), validate type: (.*)`)
	metricNameRegex := regexp.MustCompile(`VALIDATING METRIC: (.*)`)
	namespaceRegex := regexp.MustCompile(`Namespace: (.*) \| Start: (.*) \| End: (.*)`)
	metricValueRegex := regexp.MustCompile(`METRIC VALUE: (.*) = (.*) \| Upper Bound: (.*)`)
	sampleCountRegex := regexp.MustCompile(`SAMPLE COUNT: Expected range \[(.*), (.*)\] for metric '(.*)'`)
	validationFailedRegex := regexp.MustCompile(`validation failed: (.*)`)
	
	var currentResult *StressTestResult

	for scanner.Scan() {
		line := scanner.Text()
		
		// Extract test case and validation type
		if matches := testCaseRegex.FindStringSubmatch(line); len(matches) > 2 {
			summary.TestCase = matches[1]
			summary.ValidateType = matches[2]
			continue
		}
		
		// Start of a new metric validation
		if matches := metricNameRegex.FindStringSubmatch(line); len(matches) > 1 {
			// Save previous result if exists
			if currentResult != nil && currentResult.MetricName != "" {
				summary.Results = append(summary.Results, *currentResult)
			}
			
			currentResult = &StressTestResult{
				MetricName:   matches[1],
				Timestamp:    time.Now(),
				IsSuccessful: true,
			}
			continue
		}
		
		// Extract namespace and time range
		if currentResult != nil && matches := namespaceRegex.FindStringSubmatch(line); len(matches) > 3 {
			currentResult.Namespace = matches[1]
			startTime, _ := time.Parse("2006-01-02 15:04:05 -0700 MST", matches[2])
			endTime, _ := time.Parse("2006-01-02 15:04:05 -0700 MST", matches[3])
			
			if summary.StartTime.IsZero() || startTime.Before(summary.StartTime) {
				summary.StartTime = startTime
			}
			if summary.EndTime.IsZero() || endTime.After(summary.EndTime) {
				summary.EndTime = endTime
			}
			continue
		}
		
		// Extract metric value and upper bound
		if currentResult != nil && matches := metricValueRegex.FindStringSubmatch(line); len(matches) > 3 {
			fmt.Sscanf(matches[2], "%f", &currentResult.Value)
			fmt.Sscanf(matches[3], "%f", &currentResult.UpperBound)
			continue
		}
		
		// Extract sample count expectations
		if currentResult != nil && matches := sampleCountRegex.FindStringSubmatch(line); len(matches) > 3 {
			fmt.Sscanf(matches[1], "%d", &currentResult.ExpectedMin)
			fmt.Sscanf(matches[2], "%d", &currentResult.ExpectedMax)
			continue
		}
		
		// Check for validation failures
		if currentResult != nil && matches := validationFailedRegex.FindStringSubmatch(line); len(matches) > 1 {
			currentResult.IsSuccessful = false
			currentResult.ErrorMessage = matches[1]
			summary.Successful = false
			if summary.ErrorMessage == "" {
				summary.ErrorMessage = matches[1]
			} else {
				summary.ErrorMessage += "; " + matches[1]
			}
			continue
		}
		
		// Check for validation success
		if currentResult != nil && strings.Contains(line, "VALIDATION PASSED") {
			currentResult.IsSuccessful = true
			continue
		}
	}
	
	// Add the last result if exists
	if currentResult != nil && currentResult.MetricName != "" {
		summary.Results = append(summary.Results, *currentResult)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	return summary, nil
}

func printSummary(summary *TestSummary) {
	fmt.Println("=================================================")
	fmt.Println("           STRESS TEST ANALYSIS REPORT           ")
	fmt.Println("=================================================")
	fmt.Printf("Test Case:      %s\n", summary.TestCase)
	fmt.Printf("Validate Type:  %s\n", summary.ValidateType)
	fmt.Printf("Time Range:     %v to %v\n", summary.StartTime, summary.EndTime)
	fmt.Printf("Overall Status: %s\n", getStatusString(summary.Successful))
	
	if !summary.Successful {
		fmt.Printf("Error Message:  %s\n", summary.ErrorMessage)
	}
	
	fmt.Println("\nMETRIC VALIDATION RESULTS:")
	fmt.Println("-------------------------------------------------")
	fmt.Printf("%-25s %-15s %-15s %-15s %s\n", "Metric", "Value", "Upper Bound", "Status", "Details")
	fmt.Println("-------------------------------------------------")
	
	for _, result := range summary.Results {
		status := getStatusString(result.IsSuccessful)
		details := ""
		if !result.IsSuccessful {
			details = result.ErrorMessage
		}
		
		fmt.Printf("%-25s %-15.2f %-15.2f %-15s %s\n", 
			result.MetricName, 
			result.Value, 
			result.UpperBound,
			status,
			details)
	}
	
	fmt.Println("=================================================")
	
	// Print recommendations if test failed
	if !summary.Successful {
		printRecommendations(summary)
	}
}

func getStatusString(isSuccessful bool) string {
	if isSuccessful {
		return "✓ PASSED"
	}
	return "✗ FAILED"
}

func printRecommendations(summary *TestSummary) {
	fmt.Println("\nRECOMMENDATIONS:")
	
	// Check for memory-related issues
	memoryIssues := false
	for _, result := range summary.Results {
		if !result.IsSuccessful && strings.Contains(result.MetricName, "memory") {
			memoryIssues = true
			break
		}
	}
	
	if memoryIssues {
		fmt.Println("1. Memory usage exceeds thresholds. Consider:")
		fmt.Println("   - Increasing the memory threshold values in the validator")
		fmt.Println("   - Optimizing the CloudWatch Agent configuration to reduce memory usage")
		fmt.Println("   - Checking for memory leaks in the agent")
	}
	
	// Check for sample count issues
	sampleCountIssues := false
	for _, result := range summary.Results {
		if !result.IsSuccessful && strings.Contains(result.ErrorMessage, "sample count") {
			sampleCountIssues = true
			break
		}
	}
	
	if sampleCountIssues {
		fmt.Println("2. Sample count issues detected. Consider:")
		fmt.Println("   - Increasing the sample count deviation allowance")
		fmt.Println("   - Checking for network connectivity issues")
		fmt.Println("   - Verifying CloudWatch API throttling is not occurring")
	}
	
	fmt.Println("\nGeneral recommendations:")
	fmt.Println("- Review the CloudWatch Agent logs for errors or warnings")
	fmt.Println("- Ensure the test environment has consistent resources")
	fmt.Println("- Consider running the test with a longer warm-up period")
}
