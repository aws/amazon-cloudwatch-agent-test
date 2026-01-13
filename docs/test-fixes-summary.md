# CloudWatch Agent Test Fixes Summary

## Issue 1: EC2LinuxITAR and EC2LinuxCN - metric_value_benchmark Test Failure

### Error
```
Error: creating EC2 Instance: operation error EC2: RunInstances, api error Unsupported: 
The requested configuration is currently not supported.
```

### Root Cause
The `metric_value_benchmark` test uses `i4g.large` (arm64 storage-optimized instance) which is **not available** in:
- AWS GovCloud (us-gov-east-1) - Storage Optimized: I3, I3en, I4i, I7ie only
- China (cn-north-1) - Storage Optimized: D2, I2, I3, I3en, I4i only

ITAR/China partitions only run with `cloudwatch-agent-integration-test-aarch64-al2023*` AMI (arm64).

### Fix Applied
**File:** `generator/test_case_generator.go`

Changed arm64 instance type from `i4g.large` to `r6gd.large`:
```go
{
    testDir: "./test/metric_value_benchmark",
    instanceTypeByArch: map[string]string{
        "amd64": "i3en.large",
        "arm64": "r6gd.large", // r6gd available in GovCloud/China, has NVMe storage
    },
},
```

`r6gd.large` is available in GovCloud and China regions, and includes NVMe instance storage needed for the benchmark test.

---

## Issue 2: SELinux Tests - Go Version Incompatibility (REVERTED)

### Status: REVERTED - Fix caused more test failures

### Error
```
/usr/lib/golang/src/internal/abi/runtime.go:8:7: ZeroValSize redeclared in this block
/usr/lib/golang/src/internal/runtime/syscall/asm_linux_amd64.s:27: ABI selector only permitted when compiling runtime
```

### Root Cause
SELinux AMIs don't have Go pre-installed at `/usr/local/go`. Tests fall back to system Go at `/usr/lib/golang` (Go 1.18/1.19) which is incompatible with test code requiring Go 1.20+.

### Attempted Fix (REVERTED)
Added Go 1.22.5 installation for SELinux tests in terraform setup files. However, this fix caused more tests to fail, so all changes were reverted using `git checkout origin/main`.

### Current Status
This issue remains unresolved. The SELinux tests continue to fail due to Go version incompatibility. A different approach is needed.

---

## Key Files Reference

| Component | File Path |
|-----------|-----------|
| Test matrix generator | `generator/test_case_generator.go` |
| SELinux test matrix | `generator/resources/ec2_selinux_test_matrix.json` |
| Linux terraform | `terraform/ec2/linux/main.tf` |
| Common linux terraform | `terraform/ec2/common/linux/main.tf` |
| Workflow config | `.github/workflows/test-artifacts.yml` |

## Regenerating Test Matrices
After modifying `test_case_generator.go`, run:
```bash
cd amazon-cloudwatch-agent-test
go run generator/test_case_generator.go
```

---

## Issue 3: Windows Tests - App Signals Resource Files Not Found (DEBUGGING)

### Error
```
Error processing trace file: open C:\Users\Administrator\amazon-cloudwatch-agent-test\test\app_signals\resources\traces\traces.json: The system cannot find the path specified.
Error reading file: open C:\Users\Administrator\amazon-cloudwatch-agent-test\test\app_signals\resources\metrics\server_consumer.json: The system cannot find the path specified.
```

### Symptoms
- Path construction is correct (absolute Windows path)
- Files exist in git repository and are tracked
- Files exist on the branch being cloned (`paramadon/TestFixHarderIssues`)
- Validator binary uses hardcoded absolute paths in `util/common/metrics.go`

### Investigation Findings
1. **Files verified to exist in repo:**
   - `test/app_signals/resources/traces/traces.json` ✓
   - `test/app_signals/resources/metrics/server_consumer.json` ✓
   - `test/app_signals/resources/metrics/client_producer.json` ✓

2. **Path construction in code (`util/common/metrics.go`):**
   ```go
   baseDir = "C:\\Users\\Administrator\\amazon-cloudwatch-agent-test\\test\\app_signals\\resources\\traces"
   ```

3. **Terraform shows `cd` command not persisting:**
   ```
   Current directory before validator:
   C:\Users\Administrator  // Should be C:\Users\Administrator\amazon-cloudwatch-agent-test
   ```

### Hypothesis
The git clone may be failing silently or the directory structure is different than expected. The `cd` command in Windows batch doesn't persist across terraform remote-exec commands.

### Debugging Fix Applied
**File:** `terraform/ec2/win/main.tf`

Added comprehensive error checking and debugging:
```batch
"git clone --branch ${var.github_test_repo_branch} ${var.github_test_repo}",
"if %errorlevel% neq 0 (echo Git clone failed with error %errorlevel% & exit 1)",
"if not exist amazon-cloudwatch-agent-test (echo ERROR: directory not found after clone & exit 1)",
"if not exist amazon-cloudwatch-agent-test\\test\\app_signals\\resources\\traces\\traces.json (echo ERROR: traces.json not found)",
```

### Next Steps
1. Run the test with debugging to see actual git clone output
2. Verify if git clone succeeds and creates expected directory structure
3. Check if there are permission issues on Windows
4. Consider if the branch name or repo URL is incorrect

### Key Files
| Component | File Path |
|-----------|-----------|
| Windows terraform | `terraform/ec2/win/main.tf` |
| Path construction | `util/common/metrics.go` (lines 188-193, 356-361) |
| Resource files | `test/app_signals/resources/traces/traces.json` |
| | `test/app_signals/resources/metrics/server_consumer.json` |
| | `test/app_signals/resources/metrics/client_producer.json` |
