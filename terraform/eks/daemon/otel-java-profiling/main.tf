# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
# SPDX-License-Identifier: MIT

# Java continuous profiling through the amazon-cloudwatch-observability chart:
# manager.autoInstrumentationConfiguration.java.profiling.enabled makes the
# operator turn on the ADOT Java embedded profiler in Java auto-instrumented
# containers, and otlp.profiles.enabled gives the node agent a profiles
# pipeline to CloudWatch. ./test/otel/java_profiling checks the injected
# settings and queries the profiles back from CloudWatch.
#
# Needs a chart with both values (the clone step fails fast without them), an
# operator that implements java.profiling (the chart's pinned one, or
# cwagent_operator_image_*) and an agent with OTLP profiles support.

module "common" {
  source             = "../../../common"
  cwagent_image_repo = var.cwagent_image_repo
  cwagent_image_tag  = var.cwagent_image_tag
}

module "basic_components" {
  source = "../../../basic_components"
  region = var.region
}

locals {
  aws_eks = "aws eks --region ${var.region}"
  # The chart composes images as <repositoryDomain>/<repository>:<tag>.
  agent_image_domain    = dirname(var.cwagent_image_repo)
  agent_image_name      = basename(var.cwagent_image_repo)
  operator_image_domain = var.cwagent_operator_image_repo == "" ? "" : dirname(var.cwagent_operator_image_repo)
  operator_image_name   = var.cwagent_operator_image_repo == "" ? "" : basename(var.cwagent_operator_image_repo)
  service_name          = "java-profiling-${module.common.testing_id}"
  opt_out_service_name  = "java-profiling-opt-out-${module.common.testing_id}"
  # Base64, so that no shell quoting applies to the manifest.
  workloads_manifest = base64encode(templatefile("${path.module}/../../../../test/otel/java_profiling/resources/java-profiling-app.yaml", {
    java_image           = var.java_image
    service_name         = local.service_name
    opt_out_service_name = local.opt_out_service_name
    memory_off           = var.expect_memory_profiling == "false"
  }))
}

resource "aws_eks_cluster" "this" {
  name     = "cwagent-eks-integ-${module.common.testing_id}"
  role_arn = module.basic_components.role_arn
  version  = var.k8s_version
  vpc_config {
    subnet_ids         = module.basic_components.public_subnet_ids
    security_group_ids = [module.basic_components.security_group]
  }
}

resource "aws_eks_node_group" "this" {
  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "cwagent-otel-java-profiling-node-${module.common.testing_id}"
  node_role_arn   = aws_iam_role.node_role.arn
  subnet_ids      = module.basic_components.public_subnet_ids

  scaling_config {
    desired_size = 2
    max_size     = 2
    min_size     = 2
  }

  ami_type       = var.ami_type
  capacity_type  = "ON_DEMAND"
  disk_size      = 20
  instance_types = [var.instance_type]

  depends_on = [
    aws_iam_role_policy_attachment.node_AmazonEC2ContainerRegistryReadOnly,
    aws_iam_role_policy_attachment.node_AmazonEKS_CNI_Policy,
    aws_iam_role_policy_attachment.node_AmazonEKSWorkerNodePolicy,
    aws_iam_role_policy_attachment.node_CloudWatchAgentServerPolicy,
  ]
}

resource "aws_iam_role" "node_role" {
  name = "cwagent-otel-java-profiling-Worker-Role-${module.common.testing_id}"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ec2.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
}

resource "aws_iam_role_policy_attachment" "node_AmazonEKSWorkerNodePolicy" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy"
  role       = aws_iam_role.node_role.name
}

resource "aws_iam_role_policy_attachment" "node_AmazonEKS_CNI_Policy" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy"
  role       = aws_iam_role.node_role.name
}

resource "aws_iam_role_policy_attachment" "node_AmazonEC2ContainerRegistryReadOnly" {
  policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
  role       = aws_iam_role.node_role.name
}

# The node agent exports with the node role; CloudWatchAgentServerPolicy
# includes cloudwatch:PutProfileRecords.
resource "aws_iam_role_policy_attachment" "node_CloudWatchAgentServerPolicy" {
  policy_arn = "arn:aws:iam::aws:policy/CloudWatchAgentServerPolicy"
  role       = aws_iam_role.node_role.name
}

resource "null_resource" "kubectl" {
  depends_on = [aws_eks_cluster.this, aws_eks_node_group.this]
  provisioner "local-exec" {
    command = "${local.aws_eks} update-kubeconfig --name ${aws_eks_cluster.this.name}"
  }
}

# --- Helm chart install ---

# Clones the chart and reports whether it has java.profiling: one template
# file that names java.profiling and also builds the /v1development/profiles
# endpoint for it. (The node agent's own profiles exporter config carries that
# path too, and another chart feature is also named java.profiling, so neither
# string alone tells them apart.) The data
# source itself never fails, a failed clone included: terraform destroy reads
# it again, without the apply's -var flags, and a failure there would leave the
# cluster behind. The precondition on the release enforces the result on apply
# only.
data "external" "clone_helm_chart" {
  program = ["env", "REPO=${var.helm_chart_repo}", "BRANCH=${var.helm_chart_branch}", "bash", "-c", <<-EOT
    rm -rf ./helm-charts
    if ! git clone -b "$BRANCH" -- "$REPO" ./helm-charts >&2; then
      echo "could not clone $REPO@$BRANCH" >&2
      echo '{"java_profiling":"false"}'
      exit 0
    fi
    t=./helm-charts/charts/amazon-cloudwatch-observability/templates
    has_profiling() {
      grep -rlF 'java.profiling' "$t" | while IFS= read -r f; do
        if grep -qF '/v1development/profiles' "$f"; then echo found; fi
      done | grep -q found
    }
    if has_profiling; then
      echo '{"java_profiling":"true"}'
    else
      echo '{"java_profiling":"false"}'
    fi
  EOT
  ]
}

resource "helm_release" "aws_observability" {
  name             = "amazon-cloudwatch-observability"
  chart            = "./helm-charts/charts/amazon-cloudwatch-observability"
  namespace        = "amazon-cloudwatch"
  create_namespace = true

  set = concat(
    [
      { name = "clusterName", value = aws_eks_cluster.this.name },
      { name = "region", value = var.region },
      # The chart prefers repositoryDomainMap.<region> over .public: set both.
      { name = "agent.image.repositoryDomainMap.public", value = local.agent_image_domain },
      { name = "agent.image.repositoryDomainMap.${var.region}", value = local.agent_image_domain },
      { name = "agent.image.repository", value = local.agent_image_name },
      { name = "agent.image.tag", value = var.cwagent_image_tag },
      { name = "otlp.profiles.enabled", value = "true" },
      # The operator reads string leaves only; keep "true" a string.
      { name = "manager.autoInstrumentationConfiguration.java.profiling.enabled", value = "true", type = "string" },
    ],
    var.cwagent_operator_image_repo == "" ? [] : [
      { name = "manager.image.repositoryDomainMap.public", value = local.operator_image_domain },
      { name = "manager.image.repositoryDomainMap.${var.region}", value = local.operator_image_domain },
      { name = "manager.image.repository", value = local.operator_image_name },
    ],
    var.cwagent_operator_image_tag == "" ? [] : [
      { name = "manager.image.tag", value = var.cwagent_operator_image_tag },
    ],
  )

  lifecycle {
    # A chart without java.profiling would install fine and fail every check;
    # stop here with the reason instead. (The chart itself refuses
    # java.profiling without otlp.profiles.)
    precondition {
      condition     = data.external.clone_helm_chart.result.java_profiling == "true"
      error_message = "${var.helm_chart_repo}@${var.helm_chart_branch} could not be cloned or has no manager.autoInstrumentationConfiguration.java.profiling."
    }
  }

  depends_on = [
    null_resource.kubectl,
    data.external.clone_helm_chart,
  ]
}

# --- Test workloads ---

resource "null_resource" "workloads" {
  depends_on = [helm_release.aws_observability]
  triggers   = { timestamp = timestamp() }
  provisioner "local-exec" {
    command = <<-EOT
      set -e
      echo '${local.workloads_manifest}' | base64 -d | kubectl apply -f -
      # A pod created before the operator's webhook served is not
      # instrumented: restart it, up to 3 times, until it is.
      instrumented() {
        kubectl -n java-profiling get pods -l app=$1 -o json \
          | jq -e '[.items[] | select(.metadata.deletionTimestamp == null)] | length == 1 and
              ([.[0].spec.containers[0].env[]? | select(.name == "JAVA_TOOL_OPTIONS") | .value | contains("-javaagent:")] | any)' > /dev/null
      }
      for d in java-profiling java-profiling-opt-out; do
        kubectl -n java-profiling rollout status deployment/$d --timeout=600s
        n=0
        until instrumented $d; do
          n=$((n + 1))
          if [ $n -gt 3 ]; then
            echo "$d is still not Java auto-instrumented after 3 restarts" >&2
            exit 1
          fi
          echo "$d is not instrumented yet; restart $n"
          sleep 20
          kubectl -n java-profiling rollout restart deployment/$d
          kubectl -n java-profiling rollout status deployment/$d --timeout=600s
        done
      done
    EOT
  }
}

# --- Test runner ---

resource "null_resource" "validator" {
  depends_on = [null_resource.workloads]
  triggers   = { always_run = timestamp() }

  provisioner "local-exec" {
    command = <<-EOT
      set -e
      echo "Running Java profiling integration tests"
      cd ../../../..
      JAVA_PROFILING_EXPECT_MEMORY='${var.expect_memory_profiling}' \
      go test -tags integration -timeout 1h -v ${var.test_dir} \
        -eksClusterName=${aws_eks_cluster.this.name} \
        -computeType=EKS \
        -eksDeploymentStrategy=DAEMON \
        -region=${var.region}
    EOT
  }
}
