# Log Cleanup Functionality

This document describes the log cleanup functionality added to address issue #147, which ensures that EMF (Embedded Metric Format) and Container Insights logs are cleaned up after test execution.

## Overview

The CloudWatch Agent Test framework now automatically cleans up log groups created during test execution to prevent accumulation of test artifacts in CloudWatch Logs. This helps reduce costs and keeps the AWS account clean.

## Features

### Automatic Cleanup Patterns

The cleanup functionality automatically identifies and cleans up log groups matching these patterns:

**Container Insights:**
- `/aws/ecs/containerinsights/*/performance`
- `/aws/ecs/containerinsights/*/application`
- `/aws/eks/containerinsights/*/performance`
- `/aws/eks/containerinsights/*/application`
- `/aws/containerinsights/*`

**EMF (Embedded Metric Format):**
- `*EMF*` (case variations)
- `/aws/lambda/*` (Lambda EMF logs)
- `EMFECSNameSpace` (ECS test namespace)
- `EMFEKSNameSpace` (EKS test namespace)

**ECS Task Logs:**
- `/ecs/*`
- `/aws/ecs/*`

**Test-specific Patterns:**
- `*-test-*`
- `*test*`
- `cwagent-*`
- `cloudwatch-agent-*`

### Safety Features

1. **Dry Run by Default**: All cleanup operations default to dry-run mode for safety
2. **Age-based Protection**: Only cleans logs older than specified age (default: 1-2 hours)
3. **Exclude Patterns**: Never touches production logs (patterns with `production` or `prod`)
4. **Error Isolation**: Cleanup errors don't fail tests
5. **Detailed Logging**: Comprehensive logging of all cleanup operations

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `CWAGENT_SKIP_LOG_CLEANUP` | `false` | Set to `true` to completely disable log cleanup |
| `CWAGENT_FORCE_LOG_CLEANUP` | `false` | Set to `true` to perform actual deletion (not dry-run) |

### Examples

```bash
# Skip all cleanup (useful for debugging)
export CWAGENT_SKIP_LOG_CLEANUP=true

# Enable actual cleanup (use with caution)
export CWAGENT_FORCE_LOG_CLEANUP=true

# Default behavior (dry-run only)
unset CWAGENT_SKIP_LOG_CLEANUP
unset CWAGENT_FORCE_LOG_CLEANUP
```

## Integration with Test Framework

### Automatic Integration

Cleanup is automatically integrated into the test framework:

- **BaseTestRunner**: Includes default cleanup for all tests
- **ECSTestRunner**: Specialized cleanup for ECS-specific log groups
- **Test Suites**: Cleanup runs after each test completion

### Manual Usage

```go
import "github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"

// Simple cleanup with defaults
err := awsservice.CleanupTestLogGroups(true) // dry-run
if err != nil {
    log.Printf("Cleanup failed: %v", err)
}

// Custom cleanup configuration
config := awsservice.LogGroupCleanupConfig{
    IncludePatterns: []string{"my-test-logs-*"},
    ExcludePatterns: []string{"*-important-*"},
    DryRun: false,
    MaxAge: &time.Duration(2 * time.Hour),
}

result, err := awsservice.CleanupLogGroupsByPattern(config)
if err != nil {
    log.Printf("Cleanup failed: %v", err)
} else {
    log.Printf("Deleted %d log groups", len(result.DeletedLogGroups))
}
```

## Best Practices

### For Development

1. **Always test with dry-run first**:
   ```bash
   # This is the default, but be explicit
   unset CWAGENT_FORCE_LOG_CLEANUP
   ```

2. **Review cleanup results**:
   ```bash
   # Look for cleanup logs in test output
   grep -i "cleanup" test_output.log
   ```

3. **Use skip for debugging**:
   ```bash
   # When you need to examine logs after test failure
   export CWAGENT_SKIP_LOG_CLEANUP=true
   ```

### For CI/CD

1. **Enable cleanup in CI pipelines**:
   ```yaml
   environment:
     CWAGENT_FORCE_LOG_CLEANUP: "true"
   ```

2. **Monitor cleanup in logs**:
   ```bash
   # Check if cleanup is working
   grep "cleanup completed" ci_logs.txt
   ```

3. **Handle cleanup failures gracefully**:
   - Cleanup failures don't fail tests
   - Monitor for cleanup warnings in CI logs

## Troubleshooting

### Common Issues

1. **Permission Errors**:
   ```
   Error: failed to delete log group: AccessDenied
   ```
   **Solution**: Ensure IAM role has `logs:DeleteLogGroup` permission

2. **Resource Not Found**:
   ```
   Error: ResourceNotFoundException
   ```
   **Solution**: This is normal - log group was already deleted

3. **Too Many Log Groups**:
   ```
   Warning: Found 1000+ log groups to evaluate
   ```
   **Solution**: Consider more specific include patterns

### Debug Mode

To debug cleanup issues:

```bash
# Enable verbose logging and dry-run
export CWAGENT_SKIP_LOG_CLEANUP=false
unset CWAGENT_FORCE_LOG_CLEANUP  # Ensures dry-run

# Run your test and check output
go test -v ./test/emf/ 2>&1 | grep -i cleanup
```

### Manual Cleanup

For manual cleanup operations:

```go
package main

import (
    "log"
    "github.com/aws/amazon-cloudwatch-agent-test/util/awsservice"
)

func main() {
    // List what would be cleaned up
    logGroups, err := awsservice.ListEMFAndContainerInsightsLogGroups()
    if err != nil {
        log.Fatalf("Failed to list log groups: %v", err)
    }
    
    log.Printf("Found %d log groups that match cleanup patterns:", len(logGroups))
    for _, group := range logGroups {
        log.Printf("  - %s", group)
    }
    
    // Perform actual cleanup (use with caution)
    // err = awsservice.CleanupTestLogGroups(false)
}
```

## Monitoring and Metrics

The cleanup functionality provides detailed metrics:

- **Deleted Log Groups**: Count of successfully deleted log groups
- **Skipped Log Groups**: Count of log groups skipped (age, exclusions)
- **Errors**: Count and details of cleanup errors
- **Total Processed**: Total number of log groups evaluated

## Security Considerations

1. **IAM Permissions**: Requires `logs:DescribeLogGroups` and `logs:DeleteLogGroup`
2. **Exclude Patterns**: Production logs are automatically excluded
3. **Age Constraints**: Only older logs are eligible for cleanup
4. **Dry Run Default**: Safe defaults prevent accidental deletions

## Contributing

When adding new test types that create log groups:

1. **Add appropriate patterns** to the cleanup configuration
2. **Test with dry-run** to ensure patterns work correctly
3. **Document new patterns** in this file
4. **Consider safety exclusions** for any special cases

For questions or issues, please refer to the main project documentation or create an issue.
