# Credential Chain Tests

Validates the CloudWatch agent's custom credential chain priority order by setting up competing credential sources and ensuring the higher-priority source is used. This helps to enforce backwards compatibility. See https://docs.aws.amazon.com/AmazonCloudWatch/latest/monitoring/CloudWatch-Agent-Credentials-Preference.html for details on the order.

## Test Runners

### 1. CommonConfigTestRunner

Tests that `common-config.toml` takes precedence over AWS SDK defaults.

- Sets up shared credentials via `common-config.toml`
- Places invalid credentials at `/home/cwagent/.aws/credentials` 
- Verifies agent uses higher-priority shared credentials

### 2. HomeEnvTestRunner

Tests backwards compatibility for HOME environment variable resolution. The CloudWatch agent maintains an older version of the home directory resolution for the shared credential file.

- Sets up credentials at custom HOME location (`/tmp/test-home/.aws/credentials`)
- Uses systemd override to set HOME environment variable
- Places invalid credentials at `/root/.aws/credentials`
- Verifies agent uses HOME-based credentials for backwards compatibility

## What Gets Tested

**Credential Priority Order:**
1. `common-config.toml` shared credentials (highest)
2. HOME environment variable resolution
3. Default user home directory
