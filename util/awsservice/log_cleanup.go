// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
)

// Common log group patterns for EMF and Container Insights
var (
	// Container Insights patterns
	ContainerInsightsPatterns = []string{
		"/aws/ecs/containerinsights/.*/performance",
		"/aws/ecs/containerinsights/.*/application",
		"/aws/eks/containerinsights/.*/performance",
		"/aws/eks/containerinsights/.*/application",
		"/aws/containerinsights/.*",
	}

	// EMF patterns - these are more flexible since EMF logs can be in various namespaces
	EMFPatterns = []string{
		".*EMF.*", // Common EMF pattern
		".*emf.*", // Lowercase variant
		"/aws/lambda/.*", // Lambda EMF logs
		"EMFECSNameSpace", // From the test configuration
		"EMFEKSNameSpace", // From the test configuration
	}

	// ECS Task patterns
	ECSTaskPatterns = []string{
		"/ecs/.*",
		"/aws/ecs/.*",
	}

	// Test-specific patterns that might be created during integration tests
	TestPatterns = []string{
		".*-test-.*",
		".*test.*",
		"cwagent-.*",
		"cloudwatch-agent-.*",
	}
)

// LogGroupCleanupConfig defines the configuration for log group cleanup
type LogGroupCleanupConfig struct {
	IncludePatterns []string // Patterns to include for cleanup
	ExcludePatterns []string // Patterns to exclude from cleanup (safety)
	DryRun          bool     // If true, only list what would be deleted
	MaxAge          *time.Duration // Only delete log groups older than this
	BatchSize       int            // Number of log groups to process in each batch
}

// LogGroupCleanupResult contains the results of a cleanup operation
type LogGroupCleanupResult struct {
	DeletedLogGroups []string
	SkippedLogGroups []string
	Errors           []error
	TotalProcessed   int
}

// CleanupEMFAndContainerInsightsLogs performs cleanup of EMF and Container Insights log groups
// This is the main entry point for cleaning up logs after test execution
func CleanupEMFAndContainerInsightsLogs(config LogGroupCleanupConfig) (*LogGroupCleanupResult, error) {
	log.Printf("Starting cleanup of EMF and Container Insights log groups (DryRun: %v)", config.DryRun)

	// Combine all common patterns if no specific patterns provided
	if len(config.IncludePatterns) == 0 {
		config.IncludePatterns = append(config.IncludePatterns, ContainerInsightsPatterns...)
		config.IncludePatterns = append(config.IncludePatterns, EMFPatterns...)
		config.IncludePatterns = append(config.IncludePatterns, ECSTaskPatterns...)
		config.IncludePatterns = append(config.IncludePatterns, TestPatterns...)
	}

	// Set default batch size
	if config.BatchSize <= 0 {
		config.BatchSize = 50
	}

	return CleanupLogGroupsByPattern(config)
}

// CleanupLogGroupsByPattern cleans up log groups matching specific patterns
func CleanupLogGroupsByPattern(config LogGroupCleanupConfig) (*LogGroupCleanupResult, error) {
	result := &LogGroupCleanupResult{
		DeletedLogGroups: make([]string, 0),
		SkippedLogGroups: make([]string, 0),
		Errors:           make([]error, 0),
	}

	// Get all log groups
	logGroups, err := listAllLogGroups()
	if err != nil {
		return result, fmt.Errorf("failed to list log groups: %w", err)
	}

	log.Printf("Found %d total log groups to evaluate", len(logGroups))

	// Compile regex patterns for efficiency
	includeRegexes, err := compilePatterns(config.IncludePatterns)
	if err != nil {
		return result, fmt.Errorf("failed to compile include patterns: %w", err)
	}

	excludeRegexes, err := compilePatterns(config.ExcludePatterns)
	if err != nil {
		return result, fmt.Errorf("failed to compile exclude patterns: %w", err)
	}

	// Process log groups
	for _, logGroup := range logGroups {
		logGroupName := *logGroup.LogGroupName
		result.TotalProcessed++

		// Check if log group matches any include pattern
		if !matchesAnyPattern(logGroupName, includeRegexes) {
			continue
		}

		// Check if log group matches any exclude pattern
		if matchesAnyPattern(logGroupName, excludeRegexes) {
			log.Printf("Skipping log group %s (matches exclude pattern)", logGroupName)
			result.SkippedLogGroups = append(result.SkippedLogGroups, logGroupName)
			continue
		}

		// Check age constraint if specified
		if config.MaxAge != nil {
			if logGroup.CreationTime == nil {
				continue
			}
			creationTime := time.UnixMilli(*logGroup.CreationTime)
			if time.Since(creationTime) < *config.MaxAge {
				log.Printf("Skipping log group %s (too recent: %v)", logGroupName, creationTime)
				result.SkippedLogGroups = append(result.SkippedLogGroups, logGroupName)
				continue
			}
		}

		if config.DryRun {
			log.Printf("[DRY RUN] Would delete log group: %s", logGroupName)
			result.DeletedLogGroups = append(result.DeletedLogGroups, logGroupName)
		} else {
			log.Printf("Deleting log group: %s", logGroupName)
			err := deleteLogGroupWithRetry(logGroupName)
			if err != nil {
				log.Printf("Failed to delete log group %s: %v", logGroupName, err)
				result.Errors = append(result.Errors, fmt.Errorf("failed to delete %s: %w", logGroupName, err))
			} else {
				result.DeletedLogGroups = append(result.DeletedLogGroups, logGroupName)
			}
		}
	}

	log.Printf("Cleanup completed. Deleted: %d, Skipped: %d, Errors: %d", 
		len(result.DeletedLogGroups), len(result.SkippedLogGroups), len(result.Errors))

	return result, nil
}

// CleanupTestLogGroups is a convenience function for cleaning up logs after tests
// It uses safe defaults and common test patterns
func CleanupTestLogGroups(dryRun bool) error {
	maxAge := 1 * time.Hour // Only clean up logs older than 1 hour for safety
	config := LogGroupCleanupConfig{
		DryRun: dryRun,
		MaxAge: &maxAge,
		// Add some exclude patterns for safety
		ExcludePatterns: []string{
			"/aws/lambda/.*", // Don't delete Lambda logs unless specifically targeted
			".*production.*", // Never delete production logs
			".*prod.*",       // Never delete prod logs
		},
	}

	result, err := CleanupEMFAndContainerInsightsLogs(config)
	if err != nil {
		return err
	}

	if len(result.Errors) > 0 {
		return fmt.Errorf("cleanup completed with %d errors: %v", len(result.Errors), result.Errors[0])
	}

	return nil
}

// ListEMFAndContainerInsightsLogGroups lists log groups that match EMF and Container Insights patterns
func ListEMFAndContainerInsightsLogGroups() ([]string, error) {
	config := LogGroupCleanupConfig{
		DryRun: true,
	}

	result, err := CleanupEMFAndContainerInsightsLogs(config)
	if err != nil {
		return nil, err
	}

	return result.DeletedLogGroups, nil // In dry run mode, these are the groups that would be deleted
}

// Helper functions

func listAllLogGroups() ([]types.LogGroup, error) {
	var allLogGroups []types.LogGroup
	var nextToken *string

	for {
		input := &cloudwatchlogs.DescribeLogGroupsInput{
			NextToken: nextToken,
			Limit:     aws.Int32(50), // AWS limit
		}

		output, err := CwlClient.DescribeLogGroups(context.TODO(), input)
		if err != nil {
			return nil, err
		}

		allLogGroups = append(allLogGroups, output.LogGroups...)

		if output.NextToken == nil {
			break
		}
		nextToken = output.NextToken
	}

	return allLogGroups, nil
}

func compilePatterns(patterns []string) ([]*regexp.Regexp, error) {
	var regexes []*regexp.Regexp
	for _, pattern := range patterns {
		regex, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("invalid pattern '%s': %w", pattern, err)
		}
		regexes = append(regexes, regex)
	}
	return regexes, nil
}

func matchesAnyPattern(text string, patterns []*regexp.Regexp) bool {
	for _, pattern := range patterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

func deleteLogGroupWithRetry(logGroupName string) error {
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		err := deleteLogGroupSafe(logGroupName)
		if err == nil {
			return nil
		}

		// Check if it's a retryable error
		if strings.Contains(err.Error(), "ResourceNotFoundException") {
			// Log group already deleted
			return nil
		}

		if i < maxRetries-1 {
			log.Printf("Retry %d/%d for deleting log group %s: %v", i+1, maxRetries, logGroupName, err)
			time.Sleep(time.Duration(i+1) * time.Second)
		}
	}

	return fmt.Errorf("failed to delete log group %s after %d retries", logGroupName, maxRetries)
}

func deleteLogGroupSafe(logGroupName string) error {
	_, err := CwlClient.DeleteLogGroup(context.TODO(), &cloudwatchlogs.DeleteLogGroupInput{
		LogGroupName: aws.String(logGroupName),
	})
	return err
}

// GetLogGroupsByPrefix returns log groups matching a specific prefix
func GetLogGroupsByPrefix(prefix string) ([]string, error) {
	input := &cloudwatchlogs.DescribeLogGroupsInput{
		LogGroupNamePrefix: aws.String(prefix),
	}

	var logGroupNames []string
	var nextToken *string

	for {
		input.NextToken = nextToken
		output, err := CwlClient.DescribeLogGroups(context.TODO(), input)
		if err != nil {
			return nil, err
		}

		for _, logGroup := range output.LogGroups {
			logGroupNames = append(logGroupNames, *logGroup.LogGroupName)
		}

		if output.NextToken == nil {
			break
		}
		nextToken = output.NextToken
	}

	return logGroupNames, nil
}

// CleanupLogGroupsByPrefix deletes all log groups with a specific prefix
func CleanupLogGroupsByPrefix(prefix string, dryRun bool) error {
	logGroups, err := GetLogGroupsByPrefix(prefix)
	if err != nil {
		return err
	}

	log.Printf("Found %d log groups with prefix '%s'", len(logGroups), prefix)

	for _, logGroupName := range logGroups {
		if dryRun {
			log.Printf("[DRY RUN] Would delete log group: %s", logGroupName)
		} else {
			log.Printf("Deleting log group: %s", logGroupName)
			DeleteLogGroup(logGroupName)
		}
	}

	return nil
}
