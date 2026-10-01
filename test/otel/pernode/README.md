# Per-node Target Allocator E2E

Validates three things end to end on a real EKS cluster:

- **Per-node allocation** — every ServiceMonitor/PodMonitor series is scraped by
  the CloudWatch agent on the **same node** as the scraped pod
  (`target_node == @resource.k8s.node.name`).
- **G1 — zero-step CRD bundling** — the `monitoring.coreos.com` ServiceMonitor /
  PodMonitor CRDs are present after a plain chart install, with **no**
  prometheus-operator prerequisite (this harness never installs them separately).
- **G2 — TA resilience** — the Target Allocator stays healthy (no crashloop, no
  restarts) even though the CRDs may become established after it starts, and it
  discovers the monitors once they exist.

## Layout

- `test/otel/pernode/` — the Go suite (`//go:build integration`).
  - `per_node_test.go` — the `target_node == agent node` assertion + node spread.
  - `crd_bundling_test.go` — SM/PM CRDs are served (G1).
  - `ta_resilience_test.go` — TA Deployment Available, zero restarts, discovers monitors (G2).
  - `resources/workload.yaml` — sm-app/pm-app + ServiceMonitor/PodMonitor (target_node relabel) + load generator.
- `terraform/eks/daemon/otel-pernode/` — provisions EKS, installs the chart, deploys custom images, applies the workload, runs the suite.

## Prerequisites

This exercises **unreleased** code, so you must supply custom images and a chart
checkout that contains the changes:

| What | Why | Variable |
|---|---|---|
| Custom **operator** image | public operator hardcodes consistent-hashing and ignores the per-node CR field | `operator_image_domain`, `operator_image_repo`, `operator_image_tag` |
| Custom **Target Allocator** image | per-node strategy + CRD-watch resilience live here | `ta_image` |
| **helm-charts** checkout with the CRD bundling (G1) | the suite asserts the chart bundles the SM/PM CRDs | `helm_chart_repo`, `helm_chart_branch` |

## Run

```bash
cd terraform/eks/daemon/otel-pernode
terraform init
terraform apply \
  -var="region=us-west-2" \
  -var="helm_chart_repo=<git url of your helm-charts fork>" \
  -var="helm_chart_branch=<branch with the G1 commit>" \
  -var="operator_image_domain=<acct>.dkr.ecr.us-west-2.amazonaws.com" \
  -var="operator_image_repo=<path>/cloudwatch-agent-operator" \
  -var="operator_image_tag=<tag>" \
  -var="ta_image=<acct>.dkr.ecr.us-west-2.amazonaws.com/<path>/cloudwatch-agent-target-allocator:<tag>"
```

The `null_resource.validator` step runs:

```
go test -tags integration -timeout 1h -v ./test/otel/pernode \
  -eksClusterName=<cluster> -computeType=EKS -eksDeploymentStrategy=DAEMON -region=<region>
```

Tear down with `terraform destroy`.
