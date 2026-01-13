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

Changed arm64 instance type from `i4g.large` to `m6g.large`:
```go
{
    testDir: "./test/metric_value_benchmark",
    instanceTypeByArch: map[string]string{
        "amd64": "i3en.large",
        "arm64": "m6g.large", // i4g not available in GovCloud/China regions
    },
},
```

`m6g.large` is available in all regions including GovCloud and China.

---

## Issue 2: SELinux Tests - Go Version Incompatibility

### Error
```
/usr/lib/golang/src/internal/abi/runtime.go:8:7: ZeroValSize redeclared in this block
/usr/lib/golang/src/internal/runtime/syscall/asm_linux_amd64.s:27: ABI selector only permitted when compiling runtime
```

### Root Cause
SELinux AMIs (`CloudwatchSelinuxAL2v4*`, `CloudwatchSelinuxAL2023*`) don't have Go pre-installed at `/usr/local/go`. Tests fall back to system Go at `/usr/lib/golang` (Go 1.18/1.19) which is incompatible with test code requiring Go 1.20+.

Regular test AMIs have Go pre-installed at `/usr/local/go`, but SELinux AMIs don't.

### Fix Applied
Added Go 1.22.5 installation for SELinux tests in terraform setup.

**Files Modified:**
- `terraform/ec2/linux/main.tf`
- `terraform/ec2/assume_role/main.tf`
- `terraform/ec2/creds/main.tf`
- `terraform/ec2/userdata/main.tf`

**Code Added (conditional on `is_selinux_test`):**
```hcl
var.is_selinux_test ? [
  "echo 'Installing Go for SELinux test...'",
  "if [ ! -f /usr/local/go/bin/go ]; then",
  "  curl -sL https://go.dev/dl/go1.22.5.linux-amd64.tar.gz -o /tmp/go.tar.gz",
  "  sudo rm -rf /usr/local/go",
  "  sudo tar -C /usr/local -xzf /tmp/go.tar.gz",
  "  rm /tmp/go.tar.gz",
  "fi",
  "echo 'Go version:' && /usr/local/go/bin/go version",
] : [],
```

Also removed `go` from yum install in `assume_role/main.tf` (was installing old system Go).

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
