# Azure VM integration test

Validates the CloudWatch Agent on a **real Azure VM** running the default OTel configuration
(`USE_DEFAULT_CONFIG=otel` / `default:otel`). It exercises the full generated OTel pipeline
(Azure IMDS detection → translation → collector startup) and verifies that metrics, logs, and traces
reach CloudWatch through the Azure → AWS credential chain (`oidctoken` extension → `sigv4auth` web identity).

## What it checks

The agent is started with no JSON config and `USE_DEFAULT_CONFIG=otel`, so it emits the default OTel
pipeline (OTLP receivers on `127.0.0.1:4317/4318` → native CloudWatch OTLP exporters). The test pushes OTLP
and asserts delivery:

| Signal  | Destination                                   | Assertion |
|---------|-----------------------------------------------|-----------|
| Metrics | CloudWatch OTLP PromQL API (`monitoring`)     | queryable, scoped to `host.id` + `cloud.provider=azure` |
| Logs    | CloudWatch Logs group `/aws/cwagent/otlp`     | per-host marker present |
| Traces  | X-Ray                                         | segment with `instance_id` annotation present |
| Creds   | agent log                                     | no `AccessDenied`; request to native `monitoring.<region>.amazonaws.com` endpoint |

The `cloud.provider=azure` label and the credential-chain check together prove the telemetry traversed
the Azure web-identity path rather than stray AWS credentials.

## Agent prerequisites

- `#2179` Add default OTel config — **merged** (`USE_DEFAULT_CONFIG=otel`)
- `#2183` Azure platform detection — **merged** (IMDS detection → translation)
- `#2010` OIDC auth (`oidctoken`) — **merged**
- `#2197` web-identity for `awscloudwatchlogsprovisioner` — **required for the logs assertion**; until it
  merges, the agent cannot create log groups on Azure VM and `AzureVM_Logs` will fail.

## Prerequisites that MUST be satisfied before a live run

These were verified against a real Azure VM and the merged agent translator. Without them a live
`terraform apply` will fail:

1. **The AWS IAM OIDC provider must be pre-created and passed by ARN.**
   A freshly-created IAM OIDC provider for an Azure AD issuer is not on the cloud-security approved
   allowlist and, on some accounts, may be auto-removed within ~2 hours. The module therefore does
   **not** create the provider — `iam.tf` references one by ARN via `azure_oidc_provider_arn`. Create it
   once out-of-band (`aws iam create-open-id-connect-provider` for `https://sts.windows.net/<tenant-id>/`
   with the token audience as client ID), confirm it persists, then pass its ARN.
   Note: a plain Azure VM managed identity issues tokens under `https://sts.windows.net/<tenant-id>/`, a
   different issuer than the AKS workload-identity provider — a separate provider is required for the VM path.

2. **The Azure token audience must be one the tenant can mint.** Verified 2026-07-12 on a real Azure VM: a
   managed identity mints tokens for `https://management.azure.com/` (now the `azure_token_audience` default),
   while a fictional `api://AWSTokenExchange` fails with `AADSTS500011: resource principal ... not found in the
   tenant`. Use a real resource/app-registration audience and match it in the role `aud` condition.

3. **The VM must have a managed identity and reach IMDS.** Verified: with a system-assigned identity, IMDS
   mints a JWT with `iss=https://sts.windows.net/<tenant-id>/` (this is the exact `azure_oidc_issuer_url`);
   without one, the token endpoint returns `Identity not found`. `main.tf` sets `identity { type = "SystemAssigned" }`.

4. **`host.id` == the VM's IMDS `vmId` == ARM `virtual_machine_id`.** Verified equal on a real VM. The azure
   resourcedetection detector sets `host.id = compute.VMID`, so the test's `-instanceId=<virtual_machine_id>`
   correctly matches the `@resource.host.id` metric label.

5. **Logs path depends on agent PR #2197 (OPEN).** Until it merges, `awscloudwatchlogsprovisioner` cannot
   create log groups via web-identity and `AzureVM_Logs` will fail. Metrics + traces work on merged code.

6. **The `azurerm` provider needs an existing vnet/subnet.** `main.tf` references a vnet named `cwa-integ-vnet`
   with a `default` subnet in the resource group — create it (or edit the data sources) before applying.

## How it runs in CI

Like every CWA integ test, this is driven by the **agent repo** (`aws/amazon-cloudwatch-agent`), not this
test repo. `integration-test.yml` → `test-artifacts.yml` (matrix from `generator/`) → a per-type reusable job
(mirror `ec2-integration-test.yml`) that runs `aws-actions/configure-aws-credentials` with
`vars.TERRAFORM_AWS_ASSUME_ROLE` (already federated into the CWA integ account), clones this repo, and runs
`terraform apply` in `terraform/azurevm`. So the **AWS assume-role is already wired** — the Azure VM test needs
only Azure credentials plus the Azure OIDC provider added (see prerequisite 1 above). The
`azure-integration-test.yml` workflow (manual `workflow_dispatch`) lives in the agent repo; it builds the
`.deb` from the dispatched ref and terraform uploads it to the VM over SSH.

Local run (any account with the OIDC provider + Azure creds); build the .deb first, then point terraform at it:
```
# in the agent repo checkout
make amazon-cloudwatch-agent-linux package-deb

# in this repo
cd terraform/azurevm
terraform init
terraform apply \
  -var="cwa_github_sha=$(git -C <agent-repo> rev-parse HEAD)" \
  -var="agent_deb_path=<agent-repo>/build/bin/linux/amd64/amazon-cloudwatch-agent.deb" \
  -var="azure_resource_group=<existing-rg>" \
  -var="azure_vnet_name=<existing-vnet>" \
  -var="azure_oidc_provider_arn=<arn-of-preexisting-Azure-OIDC-provider>"
```

Terraform provisions the Azure VM (system-assigned managed identity), creates the `CWAGENT_ROLE` federated to
the referenced Azure OIDC provider, then SSHes in to upload the `.deb`, install the agent, start it with
`default:otel`, and run `go test -tags integration ./test/azurevm -computeType=AZUREVM`.

## Required configuration (set on the AGENT repo `aws/amazon-cloudwatch-agent`)

The AWS assume-role is already provided by `vars.TERRAFORM_AWS_ASSUME_ROLE`; there is **no new AWS role and no
`AZUREVM_TEST_AWS_ROLE_ARN` secret**. Only the Azure-side config below is net-new:

| Name | Kind | Purpose |
|------|------|---------|
| `AZURE_CLIENT_ID` / `AZURE_CLIENT_SECRET` / `AZURE_TENANT_ID` / `AZURE_SUBSCRIPTION_ID` | secret | `ARM_*` creds for the `azurerm` provider (service principal that can create a VM in the RG) |
| `AZURE_RESOURCE_GROUP` | variable | Existing resource group the VM is created in |
| `AZURE_VNET_NAME` | variable | Existing vnet in that RG (with a `default` subnet) the VM's NIC attaches to |
| `AZURE_OIDC_PROVIDER_ARN` | variable | ARN of the pre-created IAM OIDC provider for the Azure AD tenant (see prerequisite 1) |
| `AZURE_TOKEN_AUDIENCE` | variable | Audience the managed identity mints (e.g. `https://management.azure.com/`); matches the role `aud` condition |

The `azure_token_audience` must match both the managed-identity token audience requested by the `oidctoken`
extension and the `aud` condition on the AWS role trust policy.
