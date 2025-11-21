// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT
locals {
  aws_eks = "aws eks --region ${var.region}"
}

module "common" {
  source             = "../../../common"
  cwagent_image_repo = var.cwagent_image_repo
  cwagent_image_tag  = var.cwagent_image_tag
}

module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 21.0"

  name               = "integ-${module.common.testing_id}"
  kubernetes_version = var.k8s_version

  vpc_id     = aws_vpc.efa_test_vpc.id
  subnet_ids = aws_subnet.efa_test_public_subnet[*].id

  enable_cluster_creator_admin_permissions = true

  eks_managed_node_groups = {
    efa_nodes = {
      # EFA configuration - only at node group level in v21
      enable_efa_support = true
      ami_type           = "AL2023_x86_64_NVIDIA"
      instance_types     = [var.instance_type]

      min_size     = 1
      max_size     = 1
      desired_size = 1

      # Use private subnets for nodes
      subnet_ids = aws_subnet.efa_test_private_subnet[*].id

      labels = {
        "vpc.amazonaws.com/efa.present" = "true"
        "nvidia.com/gpu.present"        = "true"
      }

      tags = {
        Owner = "default"
      }
    }
  }

  # EKS Addons - renamed from cluster_addons, most_recent = true is now default
  addons = {
    coredns = {}
    eks-pod-identity-agent = {
      before_compute = true
    }
    kube-proxy = {}
    vpc-cni = {
      before_compute = true
    }
    amazon-cloudwatch-observability = {
      pod_identity_associations = [
        {
          service_account = "cloudwatch-agent"
          role_arn        = aws_iam_role.cloudwatch_observability.arn
        },
        {
          service_account = "fluent-bit"
          role_arn        = aws_iam_role.cloudwatch_observability.arn
        }
      ]
    }
  }

  tags = {
    Owner = "default"
  }
}

resource "helm_release" "nvidia_device_plugin" {
  name             = "nvidia-device-plugin"
  repository       = "https://nvidia.github.io/k8s-device-plugin"
  chart            = "nvidia-device-plugin"
  version          = "0.17.1"
  namespace        = "nvidia-device-plugin"
  create_namespace = true
  wait             = true
}

resource "helm_release" "aws_efa_device_plugin" {
  name       = "aws-efa-k8s-device-plugin"
  repository = "https://aws.github.io/eks-charts"
  chart      = "aws-efa-k8s-device-plugin"
  version    = "v0.5.7"
  namespace  = "kube-system"
  wait       = true

  values = [
    <<-EOT
      nodeSelector:
        vpc.amazonaws.com/efa.present: 'true'
      tolerations:
        - key: nvidia.com/gpu
          operator: Exists
          effect: NoSchedule
    EOT
  ]
}

resource "null_resource" "kubectl" {
  depends_on = [
    module.eks,
  ]
  provisioner "local-exec" {
    command = <<-EOT
      # Update kubeconfig
      ${local.aws_eks} update-kubeconfig --name ${module.eks.cluster_name}
      
      # Deploy EFA test DaemonSet
      kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: my-training-job-2
  namespace: default
  labels:
    app: my-training-job-2
spec:
  selector:
    matchLabels:
      app: my-training-job-2
  template:
    metadata:
      labels:
        app: my-training-job-2
    spec:
      containers:
        - name: efa-device-holder
          image: busybox:latest
          command: ["/bin/sh", "-c", "sleep infinity"]
          resources:
            limits:
              vpc.amazonaws.com/efa: 1
            requests:
              vpc.amazonaws.com/efa: 1
EOF
      
      # Wait for DaemonSet to be ready
      kubectl rollout status daemonset/my-training-job-2 --timeout=300s
    EOT
  }
}

resource "null_resource" "update_image" {
  depends_on = [module.eks, null_resource.kubectl]
  provisioner "local-exec" {
    command = <<-EOT
      kubectl -n amazon-cloudwatch patch AmazonCloudWatchAgent cloudwatch-agent --type='json' -p='[{"op": "replace", "path": "/spec/image", "value": "${var.cwagent_image_repo}:${var.cwagent_image_tag}"}]'
      kubectl set image deployment/amazon-cloudwatch-observability-controller-manager -n amazon-cloudwatch manager=public.ecr.aws/cloudwatch-agent/cloudwatch-agent-operator:latest
      sleep 10
    EOT
  }
}

resource "null_resource" "validator" {
  depends_on = [
    module.eks,
    null_resource.kubectl,
    null_resource.update_image
  ]

  provisioner "local-exec" {
    command = <<-EOT
      cd ../../../..
      i=0
      while [ $i -lt 3 ]; do
        i=$((i+1))
        go test ${var.test_dir} -eksClusterName=${module.eks.cluster_name} -computeType=EKS -v -eksDeploymentStrategy=DAEMON && exit 0
        sleep 60
      done
      exit 1
    EOT
  }
}
