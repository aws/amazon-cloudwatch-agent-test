# AKS integration tests

Integration tests for the CloudWatch agent on Azure Kubernetes Service. They are the AKS counterpart of the EKS OTel suites in `test/otel`.

Each suite runs against its own AKS cluster created by one Terraform stack, `terraform/azure/aks`. The stack installs the `amazon-cloudwatch-observability` Helm chart (`k8sMode=AKS`), swaps in the agent image under test, deploys the suite's workloads and runs `go test -tags integration <suite>`. The tests query CloudWatch (PromQL endpoint and Logs) for what the agent sent.

## Suites

| Suite (`test_dir`) | What it checks | Cluster | Generator |
|---|---|---|---|
| `./test/azure/aks` | `default:otel` agent and OTLP load: metrics, span metrics, logs, traces | 1 node | default |
| `./test/azure/aks/containerinsights` | OTel Container Insights: node, cluster, KEDA, Karpenter metrics; node logs | 1 node | default |
| `./test/azure/aks/standard` | Port of `test/otel/standard` (184 tests): kube-state-metrics, kubelet, cadvisor, node-exporter, control plane, dedup, resolution, common labels, host validation | 1 node | default |
| `./test/azure/aks/filters` | Port of `test/otel/filters`: the chart's `otelFilters` | 2 nodes | default |
| `./test/azure/aks/attr_limit` | Port of `test/otel/attr_limit`: attribute-limit processor with many node and pod labels | 3 nodes | default |
| `./test/azure/aks/gpu` | OTel Container Insights plus DCGM on an AKS-managed GPU pool | CPU pool + T4 pool | default |
| `./test/azure/aks/gpudriver` | DCGM from the chart's exporter on driver-install GPU pools (idle and burn pools, gpu-burn workload) | CPU pool + 2 T4 pools | default |

Every row gates CI (none is marked `wip`).

## Differences from EKS

- `cloud.platform` is `azure.aks` and `cloud.provider` is `azure`. `cloud.resource_id` is the managed cluster's ARM id, and the account id is the Azure subscription GUID.
- `host.id` is the VM system UUID and `host.name` is the node name.
- Karpenter-only tests are skipped (9 in `standard`). `__monotonicity__` and `__temporality__` are not checked.
- Pod-level cadvisor and kubelet network series carry no `service.instance.id`, so `TestServiceLabels` skips them.
- The AWS side stays the same: metrics go to the CloudWatch PromQL endpoint (`region`, default `us-east-2`), logs to CloudWatch Logs.

## Running

CI runs these through the agent repo's `test-artifacts.yml` workflow, one `test_dir_filter` value per suite. The matrix comes from `generator/test_case_generator.go` and `generator/resources/aks_test_matrix.json`.

To run one suite by hand, from `terraform/azure/aks`:

```
terraform init
terraform apply \
  -var test_dir=./test/azure/aks/standard \
  -var azure_resource_group=<rg> -var azure_vnet_name=<vnet> \
  -var runner_ip=<your-ip>/32 \
  -var cwagent_image_repo=<ecr-repo-uri> -var cwagent_image_tag=<tag> \
  -var helm_chart_branch=<chart-branch>
terraform destroy   # same -var flags
```

`terraform apply` runs the tests as its last step. Azure credentials (`az login` or `ARM_*`) and AWS credentials with access to CloudWatch and ECR are required.

## Adding a suite

1. Add the package under `test/azure/aks/<name>/` with the `integration` build tag.
2. Add its path to the `test_dir` allow-list in `terraform/azure/aks/variables.tf`.
3. Add a `suite_helm_values["./test/azure/aks/<name>"]` entry and any workloads in `terraform/azure/aks/main.tf`.
4. Add a row to the `aks` matrix in `generator/test_case_generator.go`.

## Known follow-ups

- `helm_chart_repo` defaults to a fork carrying aws-observability/helm-charts #387 (DCGM on AKS). Switch it back once #387 is merged.
- `dcgmExporter.image.tag` is pinned to `4.6.1-4.8.4-distroless`. Remove it when the chart default works on AKS.
- Not ported from EKS: multi-GPU, `gpu_dra`, `gpu_filters`, `performance`.
