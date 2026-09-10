# GCP integration tests — environment setup

One-time setup the GCP project and AWS account need before the `terraform/gcp`
modules can run. Everything else is created and destroyed per run by the
modules themselves.

The commands below use these placeholders:

```sh
PROJECT_ID=<project-id>
CI_SA=otel-collector-integ-tests@$PROJECT_ID.iam.gserviceaccount.com
```

## GCP project

Enable the APIs:

```sh
gcloud services enable compute.googleapis.com iam.googleapis.com \
  container.googleapis.com --project "$PROJECT_ID"
```

An existing VPC network and subnetwork must be passed as `gcp_network_name` /
`gcp_subnetwork_name`; the auto-created `default` network works.

Create the CI service account and grant it — and any human running the suites
locally — these project roles:

| Role | Needed for |
|---|---|
| `roles/compute.admin` | VM and firewall lifecycle (`gce`) |
| `roles/iam.serviceAccountAdmin` | creating the per-run service account (`gce`) |
| `roles/iam.serviceAccountUser` | attaching service accounts to VMs and GKE nodes |
| `roles/container.admin` | cluster lifecycle and in-cluster RBAC objects (`gke`; project editor alone is not enough) |

```sh
gcloud iam service-accounts create otel-collector-integ-tests --project "$PROJECT_ID"

for role in roles/compute.admin roles/iam.serviceAccountAdmin \
  roles/iam.serviceAccountUser roles/container.admin; do
  gcloud projects add-iam-policy-binding "$PROJECT_ID" \
    --member "serviceAccount:$CI_SA" --role "$role"
done
```

For a human, the same bindings with `--member "user:<user-email>"`.

## CI authentication

**Keyless (Workload Identity Federation)** — required when org policy disables
service-account key creation. The provider resource path and the
service-account email go in the workflow's auth step; the path embeds the
project number, so a project move means updating the workflow file as well.

```sh
gcloud iam workload-identity-pools create github-actions \
  --project "$PROJECT_ID" --location global --display-name "GitHub Actions"

gcloud iam workload-identity-pools providers create-oidc github \
  --project "$PROJECT_ID" --location global \
  --workload-identity-pool github-actions --display-name "GitHub" \
  --issuer-uri "https://token.actions.githubusercontent.com" \
  --attribute-mapping "google.subject=assertion.sub,attribute.repository=assertion.repository" \
  --attribute-condition "assertion.repository == 'aws/amazon-cloudwatch-agent'"

PROJECT_NUMBER=$(gcloud projects describe "$PROJECT_ID" --format "value(projectNumber)")

gcloud iam service-accounts add-iam-policy-binding "$CI_SA" \
  --project "$PROJECT_ID" --role roles/iam.workloadIdentityUser \
  --member "principalSet://iam.googleapis.com/projects/$PROJECT_NUMBER/locations/global/workloadIdentityPools/github-actions/attribute.repository/aws/amazon-cloudwatch-agent"
```

**Static key** — where key creation is allowed, the simpler alternative:

```sh
gcloud iam service-accounts keys create key.json --iam-account "$CI_SA"
gh secret set GCP_CREDENTIALS < key.json
```

## GitHub repository configuration

| Setting | Value |
|---|---|
| `vars.GCP_PROJECT` | project ID (not the display name or number) |
| `vars.GCP_NETWORK_NAME` | VPC network name |
| `secrets.GCP_CREDENTIALS` | service-account key JSON (static-key model only) |

```sh
gh variable set GCP_PROJECT --body "$PROJECT_ID"
gh variable set GCP_NETWORK_NAME --body default
```

## AWS account

Enable Transaction Search in the suite region (us-east-2): trace validation
reads the `aws/spans` log group, which only exists where the account's X-Ray
trace destination is CloudWatch Logs. See the `region` variable comment in
`gce/variables.tf`.

## Local runs

- Google credentials come from application-default credentials
  (`gcloud auth application-default login`; on Workspace-managed accounts that
  reject it with `admin_policy_enforced`, use `gcloud auth login --update-adc`).
- `gce`: a locally built agent `.deb` (`agent_deb_path`) and a test-repo clone
  URL/branch reachable from the VM (`github_test_repo`,
  `github_test_repo_branch`).
- `gke`: `kubectl` on PATH, and an agent container image in an ECR repository
  your AWS credentials can access (`cwagent_image_repo`, `cwagent_image_tag`,
  `ecr_region`). The agent repo's `make docker-build-amd64 IMAGE=...` target
  builds a suitable image from local binaries.
