# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
# SPDX-License-Identifier: MIT

# OTEL Container Insights LLM model-serving solutions: vLLM, KServe and Knative.
#
# Builds a CPU-only cluster running KServe in Serverless mode (Knative Serving
# on Istio), with:
#   - a vLLM InferenceService                  -> vllm (KServe), knative dataplane
#   - a plain vLLM Deployment                  -> vllm (found by image name)
#   - an annotated vLLM mock (custom image)    -> negative control: not scraped
#   - an annotated non-vLLM exporter           -> negative control: not scraped
#   - a load generator, so request metrics have data
# then installs the chart with otelContainerInsights enabled (solutions default
# on) and runs ./test/otel/solutions/{vllm,kserve,knative}.

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
  aws_eks   = "aws eks --region ${var.region}"
  namespace = "llm"
  isvc      = "vllm-isvc"
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
  node_group_name = "cwagent-otel-llm-integ-node-${module.common.testing_id}"
  node_role_arn   = aws_iam_role.node_role.arn
  subnet_ids      = module.basic_components.public_subnet_ids

  scaling_config {
    desired_size = 2
    max_size     = 2
    min_size     = 2
  }

  ami_type      = var.ami_type
  capacity_type = "ON_DEMAND"
  # The vLLM CPU image is several GB.
  disk_size      = 60
  instance_types = [var.instance_type]

  depends_on = [
    aws_iam_role_policy_attachment.node_AmazonEC2ContainerRegistryReadOnly,
    aws_iam_role_policy_attachment.node_AmazonEKS_CNI_Policy,
    aws_iam_role_policy_attachment.node_AmazonEKSWorkerNodePolicy,
  ]
}

resource "aws_iam_role" "node_role" {
  name = "cwagent-otel-llm-Worker-Role-${module.common.testing_id}"
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

resource "aws_iam_role" "pod_identity_role" {
  name = "cwagent-otel-llm-pod-identity-${module.common.testing_id}"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "pods.eks.amazonaws.com" }
      Action    = ["sts:AssumeRole", "sts:TagSession"]
    }]
  })
}

resource "aws_iam_role_policy_attachment" "pod_identity_CloudWatchAgentServerPolicy" {
  policy_arn = "arn:aws:iam::aws:policy/CloudWatchAgentServerPolicy"
  role       = aws_iam_role.pod_identity_role.name
}

resource "aws_eks_addon" "pod_identity_agent" {
  depends_on   = [aws_eks_node_group.this]
  cluster_name = aws_eks_cluster.this.name
  addon_name   = "eks-pod-identity-agent"
}

resource "null_resource" "kubectl" {
  depends_on = [aws_eks_cluster.this, aws_eks_node_group.this]
  provisioner "local-exec" {
    command = "${local.aws_eks} update-kubeconfig --name ${aws_eks_cluster.this.name}"
  }
}

# --- KServe prerequisites: cert-manager, Istio, Knative Serving ---

resource "helm_release" "cert_manager" {
  depends_on       = [aws_eks_node_group.this]
  name             = "cert-manager"
  repository       = "https://charts.jetstack.io"
  chart            = "cert-manager"
  version          = var.cert_manager_version
  namespace        = "cert-manager"
  create_namespace = true
  wait             = true
  set              = [{ name = "crds.enabled", value = "true" }]
}

resource "helm_release" "istio_base" {
  depends_on       = [aws_eks_node_group.this]
  name             = "istio-base"
  repository       = "https://istio-release.storage.googleapis.com/charts"
  chart            = "base"
  version          = var.istio_version
  namespace        = "istio-system"
  create_namespace = true
}

resource "helm_release" "istiod" {
  depends_on = [helm_release.istio_base]
  name       = "istiod"
  repository = "https://istio-release.storage.googleapis.com/charts"
  chart      = "istiod"
  version    = var.istio_version
  namespace  = "istio-system"
  wait       = true
}

# Traffic in this test is cluster-local, so the gateway needs no load balancer.
resource "helm_release" "istio_ingressgateway" {
  depends_on = [helm_release.istiod]
  name       = "istio-ingressgateway"
  repository = "https://istio-release.storage.googleapis.com/charts"
  chart      = "gateway"
  version    = var.istio_version
  namespace  = "istio-system"
  wait       = true
  set        = [{ name = "service.type", value = "ClusterIP" }]
}

# Knative >= 1.19 exports no metrics until metrics-protocol (control plane) and
# request-metrics-protocol (queue-proxy) are set, and the components read them
# only at startup, so they are restarted after the patch (see the chart README).
resource "null_resource" "knative_serving" {
  depends_on = [null_resource.kubectl, helm_release.istio_ingressgateway]
  provisioner "local-exec" {
    command = <<-EOT
      set -e
      kubectl apply -f https://github.com/knative/serving/releases/download/knative-v${var.knative_version}/serving-crds.yaml
      kubectl apply -f https://github.com/knative/serving/releases/download/knative-v${var.knative_version}/serving-core.yaml
      kubectl apply -f https://github.com/knative/net-istio/releases/download/knative-v${var.knative_net_istio_version}/net-istio.yaml
      kubectl -n knative-serving wait --for=condition=available deployment --all --timeout=600s
      kubectl -n knative-serving patch cm config-observability --type merge \
        -p '{"data":{"metrics-protocol":"prometheus","request-metrics-protocol":"prometheus"}}'
      # The vLLM image is multi-GB; Knative's tag-to-digest lookup from the
      # controller times out on it, so leave tags on public.ecr.aws unresolved.
      kubectl -n knative-serving patch cm config-deployment --type merge \
        -p '{"data":{"registries-skipping-tag-resolving":"kind.local,ko.local,dev.local,public.ecr.aws"}}'
      kubectl -n knative-serving rollout restart deployment
      for d in $(kubectl -n knative-serving get deployment -o name); do kubectl -n knative-serving rollout status "$d" --timeout=600s; done
    EOT
  }
}

resource "helm_release" "kserve_crd" {
  depends_on       = [null_resource.knative_serving, helm_release.cert_manager]
  name             = "kserve-crd"
  repository       = "oci://ghcr.io/kserve/charts"
  chart            = "kserve-crd"
  version          = var.kserve_version
  namespace        = "kserve"
  create_namespace = true
}

resource "helm_release" "kserve" {
  depends_on = [helm_release.kserve_crd]
  name       = "kserve-resources"
  repository = "oci://ghcr.io/kserve/charts"
  chart      = "kserve-resources"
  version    = var.kserve_version
  namespace  = "kserve"
  wait       = true
}

# --- Test workloads ---

resource "kubernetes_namespace_v1" "llm" {
  depends_on = [aws_eks_node_group.this]
  metadata { name = local.namespace }
}

# KServe's model container, found by the inferenceservice label + container name.
# enable-prometheus-scraping makes KServe add prometheus.io/* to the pod, which
# the vllm job must ignore on KServe pods (otherwise the queue-proxy would be
# scraped too and every vllm:* series would be duplicated).
resource "null_resource" "inference_service" {
  depends_on = [helm_release.kserve, kubernetes_namespace_v1.llm]
  provisioner "local-exec" {
    command = <<-EOT
      set -e
      cat <<'EOF' | kubectl apply -f -
      apiVersion: serving.kserve.io/v1beta1
      kind: InferenceService
      metadata:
        name: ${local.isvc}
        namespace: ${local.namespace}
        annotations:
          serving.kserve.io/enable-prometheus-scraping: "true"
          serving.knative.dev/progress-deadline: "1800s"
      spec:
        predictor:
          minReplicas: 1
          maxReplicas: 1
          containers:
            - name: kserve-container
              image: ${var.vllm_cpu_image}
              command: ["vllm", "serve", "${var.vllm_model}", "--port", "8080", "--max-model-len", "2048"]
              env:
                - { name: VLLM_CPU_KVCACHE_SPACE, value: "2" }
              ports:
                - { containerPort: 8080, protocol: TCP }
              # KServe defaults the CPU limit to 1, below this request.
              resources:
                requests: { cpu: "2", memory: 6Gi }
                limits: { cpu: "4", memory: 10Gi }
              # vLLM needs more than the 64 MiB /dev/shm a pod gets by default.
              volumeMounts:
                - { name: dshm, mountPath: /dev/shm }
          volumes:
            - name: dshm
              emptyDir: { medium: Memory, sizeLimit: 2Gi }
      EOF
      kubectl -n ${local.namespace} wait --for=condition=Ready inferenceservice/${local.isvc} --timeout=1800s
    EOT
  }
  provisioner "local-exec" {
    when    = destroy
    command = "kubectl -n llm delete inferenceservice vllm-isvc --timeout=120s 2>/dev/null || true"
  }
}

# Plain vllm serve, found only by its image name.
resource "kubernetes_deployment_v1" "vllm_standalone" {
  depends_on = [kubernetes_namespace_v1.llm]
  metadata {
    name      = "vllm-standalone"
    namespace = local.namespace
  }
  spec {
    replicas = 1
    selector { match_labels = { app = "vllm-standalone" } }
    template {
      metadata { labels = { app = "vllm-standalone" } }
      spec {
        container {
          name    = "vllm"
          image   = var.vllm_cpu_image
          command = ["vllm", "serve", var.vllm_model, "--port", "8000", "--max-model-len", "2048"]
          env {
            name  = "VLLM_CPU_KVCACHE_SPACE"
            value = "2"
          }
          port { container_port = 8000 }
          resources {
            requests = { cpu = "2", memory = "6Gi" }
            limits   = { memory = "10Gi" }
          }
          readiness_probe {
            http_get {
              path = "/health"
              port = 8000
            }
            period_seconds = 10
          }
          # vLLM needs more than the 64 MiB /dev/shm a pod gets by default.
          volume_mount {
            name       = "dshm"
            mount_path = "/dev/shm"
          }
        }
        volume {
          name = "dshm"
          empty_dir {
            medium     = "Memory"
            size_limit = "2Gi"
          }
        }
      }
    }
  }
  wait_for_rollout = false
  timeouts { create = "30m" }
}

# Static metrics served by busybox httpd from a ConfigMap.
resource "kubernetes_config_map_v1" "mock_metrics" {
  depends_on = [kubernetes_namespace_v1.llm]
  metadata {
    name      = "mock-metrics"
    namespace = local.namespace
  }
  data = {
    # A vLLM server whose image does not say so. The chart reads no scrape
    # annotations, so it must not be scraped despite prometheus.io/*.
    "vllm" = <<-EOT
      # TYPE vllm:num_requests_running gauge
      vllm:num_requests_running{model_name="annotated-mock"} 3
      # TYPE http_requests_total counter
      http_requests_total{handler="/v1/completions",method="POST",status="2xx"} 7
    EOT
    # Any other annotated exporter: must not be scraped either.
    "generic" = <<-EOT
      # TYPE http_requests_total counter
      http_requests_total{handler="/",method="GET",status="2xx"} 11
      # TYPE app_jobs_processed_total counter
      app_jobs_processed_total 5
    EOT
  }
}

resource "kubernetes_deployment_v1" "mock" {
  for_each   = { "vllm-annotated-mock" = "vllm", "generic-exporter" = "generic" }
  depends_on = [kubernetes_config_map_v1.mock_metrics]
  metadata {
    name      = each.key
    namespace = local.namespace
  }
  spec {
    replicas = 1
    selector { match_labels = { app = each.key } }
    template {
      metadata {
        labels = { app = each.key }
        annotations = {
          "prometheus.io/scrape" = "true"
          "prometheus.io/port"   = "9400"
          "prometheus.io/path"   = "/custom-metrics"
        }
      }
      spec {
        # Declares no port.
        container {
          name    = "server"
          image   = "busybox:1.36"
          command = ["sh", "-c", "mkdir -p /www && cp /m/${each.value} /www/custom-metrics && httpd -f -p 9400 -h /www"]
          volume_mount {
            name       = "m"
            mount_path = "/m"
          }
          resources { requests = { cpu = "5m", memory = "8Mi" } }
        }
        # Declares a port.
        container {
          name    = "sidecar"
          image   = "busybox:1.36"
          command = ["sh", "-c", "sleep 1000000"]
          port { container_port = 8081 }
          resources { requests = { cpu = "5m", memory = "8Mi" } }
        }
        volume {
          name = "m"
          config_map { name = kubernetes_config_map_v1.mock_metrics.metadata[0].name }
        }
      }
    }
  }
}

# Requests through the InferenceService (Knative queue-proxy) and directly to
# the standalone server, so counters and histograms on both have data.
resource "kubernetes_deployment_v1" "loadgen" {
  depends_on = [null_resource.inference_service, kubernetes_deployment_v1.vllm_standalone]
  metadata {
    name      = "loadgen"
    namespace = local.namespace
  }
  spec {
    replicas = 1
    selector { match_labels = { app = "loadgen" } }
    template {
      metadata { labels = { app = "loadgen" } }
      spec {
        container {
          name  = "curl"
          image = "curlimages/curl:8.11.1"
          command = ["sh", "-c", <<-EOT
            body='{"model":"${var.vllm_model}","prompt":"Hello","max_tokens":8}'
            while true; do
              for url in http://${local.isvc}-predictor.${local.namespace}.svc.cluster.local/v1/completions \
                         http://vllm-standalone.${local.namespace}.svc.cluster.local:8000/v1/completions; do
                curl -s -o /dev/null -m 60 -H 'Content-Type: application/json' -d "$body" "$url"
              done
              sleep 2
            done
          EOT
          ]
          resources { requests = { cpu = "10m", memory = "16Mi" } }
        }
      }
    }
  }
}

resource "kubernetes_service_v1" "vllm_standalone" {
  depends_on = [kubernetes_namespace_v1.llm]
  metadata {
    name      = "vllm-standalone"
    namespace = local.namespace
  }
  spec {
    selector = { app = "vllm-standalone" }
    port {
      port        = 8000
      target_port = 8000
    }
  }
}

# --- Helm chart install ---

data "external" "clone_helm_chart" {
  program = ["bash", "-c", <<-EOT
    rm -rf ./helm-charts
    git clone -b ${var.helm_chart_branch} ${var.helm_chart_repo} ./helm-charts >&2
    echo '{"status":"ready"}'
  EOT
  ]
}

resource "helm_release" "aws_observability" {
  name             = "amazon-cloudwatch-observability"
  chart            = "./helm-charts/charts/amazon-cloudwatch-observability"
  namespace        = "amazon-cloudwatch"
  create_namespace = true

  set = [
    { name = "clusterName", value = aws_eks_cluster.this.name },
    { name = "region", value = var.region },
    { name = "otelContainerInsights.enabled", value = "true" },
  ]

  depends_on = [
    aws_eks_addon.pod_identity_agent,
    null_resource.kubectl,
    data.external.clone_helm_chart,
    helm_release.kserve,
  ]
}

resource "aws_eks_pod_identity_association" "cloudwatch_agent" {
  depends_on      = [helm_release.aws_observability]
  cluster_name    = aws_eks_cluster.this.name
  namespace       = "amazon-cloudwatch"
  service_account = "cloudwatch-agent"
  role_arn        = aws_iam_role.pod_identity_role.arn
}

resource "null_resource" "update_image" {
  depends_on = [helm_release.aws_observability, null_resource.kubectl]
  triggers   = { timestamp = timestamp() }
  provisioner "local-exec" {
    command = <<-EOT
      sleep 30
      kubectl -n amazon-cloudwatch patch AmazonCloudWatchAgent cloudwatch-agent --type='json' \
        -p='[{"op": "replace", "path": "/spec/image", "value": "${var.cwagent_image_repo}:${var.cwagent_image_tag}"}]'
      kubectl -n amazon-cloudwatch patch AmazonCloudWatchAgent cloudwatch-agent-cluster-scraper --type='json' \
        -p='[{"op": "replace", "path": "/spec/image", "value": "${var.cwagent_image_repo}:${var.cwagent_image_tag}"}]' 2>/dev/null || true
      sleep 10
    EOT
  }
}

resource "null_resource" "restart_pods" {
  depends_on = [aws_eks_pod_identity_association.cloudwatch_agent, null_resource.update_image]
  triggers   = { timestamp = timestamp() }
  provisioner "local-exec" {
    command = <<-EOT
      kubectl -n amazon-cloudwatch rollout restart daemonset/cloudwatch-agent
      kubectl -n amazon-cloudwatch rollout restart deployment/cloudwatch-agent-cluster-scraper 2>/dev/null || true
      kubectl -n amazon-cloudwatch rollout status daemonset/cloudwatch-agent --timeout=180s
      kubectl -n amazon-cloudwatch rollout status deployment/cloudwatch-agent-cluster-scraper --timeout=180s 2>/dev/null || true
    EOT
  }
}

# --- Test runner ---

resource "null_resource" "validator" {
  depends_on = [
    null_resource.restart_pods,
    kubernetes_deployment_v1.loadgen,
    kubernetes_deployment_v1.mock,
  ]

  triggers = { always_run = timestamp() }

  provisioner "local-exec" {
    command = <<-EOT
      set -e
      cd ../../../..
      kubectl -n ${local.namespace} rollout status deployment/vllm-standalone --timeout=1800s

      echo "Waiting 4 minutes for metrics to propagate..."
      sleep 240

      # test_dir (from the CI matrix) first, then the other two solutions on the
      # same cluster, as terraform/eks/daemon/otel does for Karpenter and KEDA.
      for suite in ${var.test_dir} ./test/otel/solutions/kserve ./test/otel/solutions/knative; do
        echo "Running OTEL solutions tests ($suite)..."
        go test -tags integration -timeout 1h -v $suite/... \
          -eksClusterName=${aws_eks_cluster.this.name} \
          -computeType=EKS \
          -eksDeploymentStrategy=DAEMON \
          -region=${var.region}
      done
    EOT
  }
}
