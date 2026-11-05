# Per-node Target Allocator E2E

Validates three things end to end on a real EKS cluster:

- **Per-node allocation** — the per-node Target Allocator assigns every
  ServiceMonitor/PodMonitor target to the CloudWatch agent on the **same node** as
  the scraped pod (checked at the allocator: each target's
  `__meta_kubernetes_pod_node_name` against the assigned agent pod's node).
- **G1 — zero-step CRDs** — the `monitoring.coreos.com` ServiceMonitor /
  PodMonitor CRDs are present after a plain chart install, with **no**
  prometheus-operator prerequisite (this harness never installs them separately).
  The operator creates them (`--manage-prometheus-crds`); the suite checks the
  operator's ownership label, annotations and field manager, and that Helm does
  not own them.
- **G2 — TA resilience** — the Target Allocator stays healthy (no crashloop, no
  restarts) even though the CRDs may become established after it starts, and it
  discovers the monitors once they exist.

## Layout

- `test/otel/pernode/` — the Go suite (`//go:build integration`).
  - `per_node_test.go` — Target Allocator assignment is node-local + node spread.
  - `crd_management_test.go` — SM/PM CRDs were created by the operator and carry no other tool's markers, the operator reports them as managed (Created Event, no NotManaged Event), and it runs with `--manage-prometheus-crds` (G1).
  - `ta_resilience_test.go` — TA Deployment Available, zero restarts, discovers monitors (G2).
  - `secret_auth_test.go` — with `read_secrets=true` (default), a ServiceMonitor whose `basicAuth` comes from a Secret gets its credentials from the Target Allocator (`get`-only Secrets RBAC) and its metrics reach CloudWatch; with `read_secrets=false`, the Target Allocator has no Secrets access and builds the scrape config without credentials.
  - `resources/secret-auth-workload.yaml` — basic-auth-protected app (nginx in front of the example app), its Secret and ServiceMonitor.
  - `monitor_config_test.go` — both agents' SM/PM pipeline sets `cloudwatch.solution=k8s-otel-prometheus-monitors` and `cloudwatch.pipeline=prometheus-monitors`, keeps `up` (its own scrape-metadata filter) and exports to the regional endpoint through its own exporter; with otelContainerInsights off, it is the only pipeline each agent runs. `up` reaches CloudWatch for monitor targets: 1 for sm-app and pm-app, and for auth-app 1 with `read_secrets=true` and 0 without, so a failing target shows as `up == 0`.
  - `monitor_selector_test.go` — with `monitor_selector=true`, both CRs carry the `serviceMonitorSelector`/`podMonitorSelector` the harness sets (every monitor without the `e2e.cloudwatch.aws/exclude` label), the per-node Target Allocator serves the selected ServiceMonitor but not the excluded ServiceMonitor or PodMonitor, only the selected monitor's series reach CloudWatch, and the operator webhook rejects invalid selectors (server-side dry run). Skipped with `monitor_selector=false` (default), because it needs aws/amazon-cloudwatch-agent-operator#443.
  - `resources/selector-workload.yaml` — one app behind a selected ServiceMonitor, an excluded ServiceMonitor and an excluded PodMonitor.
  - `service_identity_test.go` — ServiceMonitor and PodMonitor series in CloudWatch carry the chart-wide service identity: `service.name` from the `resource.opentelemetry.io/service.name` pod annotation, else the Deployment name (not the Service name or the PodMonitor job), `service.namespace`, `service.instance.id` `<namespace>.<pod>.<container>` and `deployment.environment.name`.
  - `resources/identity-workload.yaml` — an annotated and a plain Deployment, each behind a ServiceMonitor (through a Service with a different name) and a PodMonitor.
  - `resources/workload.yaml` — sm-app/pm-app + ServiceMonitor/PodMonitor (target_node relabel) + load generator.
- `terraform/eks/daemon/otel-pernode/` — provisions EKS, installs the chart, deploys custom images, applies the workload, runs the suite.

## Prerequisites

This exercises **unreleased** code, so you must supply custom images and a chart
checkout that contains the changes:

| What | Why | Variable |
|---|---|---|
| Custom **operator** image | public operator hardcodes consistent-hashing, ignores the per-node CR field, and has no `--manage-prometheus-crds` | `operator_image_domain`, `operator_image_repo`, `operator_image_tag` |
| Custom **Target Allocator** image | per-node strategy + CRD-watch resilience live here | `ta_image_domain`, `ta_image_repo`, `ta_image_tag` |
| **helm-charts** checkout with the SM/PM scraping changes | the chart passes `--manage-prometheus-crds` and grants the operator CRD create | `helm_chart_repo`, `helm_chart_branch` |

## Run

```bash
cd terraform/eks/daemon/otel-pernode
terraform init
terraform apply \
  -var="region=us-west-2" \
  -var="helm_chart_repo=<git url of your helm-charts fork>" \
  -var="helm_chart_branch=<branch with the SM/PM scraping changes>" \
  -var="operator_image_domain=<acct>.dkr.ecr.us-west-2.amazonaws.com" \
  -var="operator_image_repo=<path>/cloudwatch-agent-operator" \
  -var="operator_image_tag=<tag>" \
  -var="ta_image_domain=<acct>.dkr.ecr.us-west-2.amazonaws.com" \
  -var="ta_image_repo=<path>/cloudwatch-agent-target-allocator" \
  -var="ta_image_tag=<tag>"
```

The `null_resource.validator` step runs:

```
go test -tags integration -timeout 1h -v ./test/otel/pernode \
  -eksClusterName=<cluster> -computeType=EKS -eksDeploymentStrategy=DAEMON -region=<region>
```

To run with otelContainerInsights off (SM/PM scraping on its own), add
`-var="otel_container_insights_enabled=false"`. The validator passes
`PERNODE_OTEL_CI_ENABLED` to the suite, which then also checks that each agent runs only
the SM/PM pipeline.

To check the monitor selectors, add `-var="monitor_selector=true"`. This needs operator and
Target Allocator images with aws/amazon-cloudwatch-agent-operator#443 and a chart whose
`crds/` has the matching `AmazonCloudWatchAgent` CRD. The validator passes
`PERNODE_MONITOR_SELECTOR` to the suite.

Tear down with `terraform destroy`.
