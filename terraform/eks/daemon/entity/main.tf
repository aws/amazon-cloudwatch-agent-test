// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "common" {
  source = "../../../common"
}

module "basic_components" {
  source = "../../../basic_components"

  region = var.region
}

locals {
  aws_eks  = "aws eks --region ${var.region}"
}

data "aws_eks_cluster_auth" "this" {
  name = aws_eks_cluster.this.name
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

# EKS Node Groups
resource "aws_eks_node_group" "this" {
  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "cwagent-eks-integ-node-${module.common.testing_id}"
  node_role_arn   = aws_iam_role.node_role.arn
  subnet_ids      = module.basic_components.public_subnet_ids

  scaling_config {
    desired_size = 1
    max_size     = 1
    min_size     = 1
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
    aws_iam_role_policy_attachment.node_AWSXRayDaemonWriteAccess
  ]
}

# EKS Node IAM Role
resource "aws_iam_role" "node_role" {
  name = "cwagent-eks-Worker-Role-${module.common.testing_id}"

  assume_role_policy = <<POLICY
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Service": "ec2.amazonaws.com"
      },
      "Action": "sts:AssumeRole"
    }
  ]
}
POLICY
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

resource "aws_iam_role_policy_attachment" "node_CloudWatchAgentServerPolicy" {
  policy_arn = "arn:aws:iam::aws:policy/CloudWatchAgentServerPolicy"
  role       = aws_iam_role.node_role.name
}
resource "aws_iam_role_policy_attachment" "node_AWSXRayDaemonWriteAccess" {
  policy_arn = "arn:aws:iam::aws:policy/AWSXRayDaemonWriteAccess"
  role       = aws_iam_role.node_role.name
}

# TODO: these security groups be created once and then reused
# EKS Cluster Security Group
resource "aws_security_group" "eks_cluster_sg" {
  name        = "cwagent-eks-cluster-sg-${module.common.testing_id}"
  description = "Cluster communication with worker nodes"
  vpc_id      = module.basic_components.vpc_id
}

resource "aws_security_group_rule" "cluster_inbound" {
  description              = "Allow worker nodes to communicate with the cluster API Server"
  from_port                = 443
  protocol                 = "tcp"
  security_group_id        = aws_security_group.eks_cluster_sg.id
  source_security_group_id = aws_security_group.eks_nodes_sg.id
  to_port                  = 443
  type                     = "ingress"
}

resource "aws_security_group_rule" "cluster_outbound" {
  description              = "Allow cluster API Server to communicate with the worker nodes"
  from_port                = 1024
  protocol                 = "tcp"
  security_group_id        = aws_security_group.eks_cluster_sg.id
  source_security_group_id = aws_security_group.eks_nodes_sg.id
  to_port                  = 65535
  type                     = "egress"
}


# EKS Node Security Group
resource "aws_security_group" "eks_nodes_sg" {
  name        = "cwagent-eks-node-sg-${module.common.testing_id}"
  description = "Security group for all nodes in the cluster"
  vpc_id      = module.basic_components.vpc_id

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }
}

resource "aws_security_group_rule" "nodes_internal" {
  description              = "Allow nodes to communicate with each other"
  from_port                = 0
  protocol                 = "-1"
  security_group_id        = aws_security_group.eks_nodes_sg.id
  source_security_group_id = aws_security_group.eks_nodes_sg.id
  to_port                  = 65535
  type                     = "ingress"
}

resource "aws_security_group_rule" "nodes_cluster_inbound" {
  description              = "Allow worker Kubelets and pods to receive communication from the cluster control plane"
  from_port                = 1025
  protocol                 = "tcp"
  security_group_id        = aws_security_group.eks_nodes_sg.id
  source_security_group_id = aws_security_group.eks_cluster_sg.id
  to_port                  = 65535
  type                     = "ingress"
}

resource "null_resource" "clone_helm_chart" {
  provisioner "local-exec" {
    command = <<-EOT
      if [ ! -d "./helm-charts" ]; then
        git clone -b ${var.helm_chart_branch} https://github.com/aws-observability/helm-charts.git ./helm-charts
      fi
    EOT
  }
}

resource "helm_release" "aws_observability" {
  name             = "amazon-cloudwatch-observability"
  chart            = "./helm-charts/charts/amazon-cloudwatch-observability"
  namespace        = "amazon-cloudwatch"
  create_namespace = true

  set {
    name  = "clusterName"
    value = aws_eks_cluster.this.name
  }

  set {
    name  = "region"
    value = "us-west-2"
  }
  depends_on = [
    aws_eks_cluster.this,
    aws_eks_node_group.this,
  null_resource.clone_helm_chart]
}

resource "null_resource" "kubectl" {
  depends_on = [
    aws_eks_cluster.this,
    aws_eks_node_group.this,
  ]
  provisioner "local-exec" {
    command = <<-EOT
      ${local.aws_eks} update-kubeconfig --name ${aws_eks_cluster.this.name}
      ${local.aws_eks} list-clusters --output text
      ${local.aws_eks} describe-cluster --name ${aws_eks_cluster.this.name} --output text
    EOT
  }
}

resource "null_resource" "update_image" {
  depends_on = [helm_release.aws_observability, null_resource.kubectl]
  triggers = {
    timestamp  = "${timestamp()}"  # Forces re-run on every apply
  }
  provisioner "local-exec" {
    command = <<-EOT
      kubectl -n amazon-cloudwatch patch AmazonCloudWatchAgent cloudwatch-agent --type='json' -p='[{"op": "replace", "path": "/spec/image", "value": "${var.cwagent_image_repo}:${var.cwagent_image_tag}"}]'
      kubectl set image deployment/amazon-cloudwatch-observability-controller-manager -n amazon-cloudwatch manager=public.ecr.aws/cloudwatch-agent/cloudwatch-agent-operator:latest
      sleep 10
    EOT
  }
}

resource "kubernetes_pod" "log_generator" {
  depends_on = [aws_eks_node_group.this]
  metadata {
    name      = "log-generator"
    namespace = "default"
  }

  spec {
    container {
      name  = "log-generator"
      image = "busybox"

      # Run shell script that generate a log line every second
      command = ["/bin/sh", "-c"]
      args    = ["while true; do echo \"Log entry at $(date)\"; sleep 1; done"]
    }
    restart_policy = "Always"
  }
}

resource "kubernetes_pod" "petclinic_instrumentation" {
  depends_on = [aws_eks_node_group.this, helm_release.aws_observability,null_resource.update_image]
  metadata {
    name = "petclinic-instrumentation-default-env"
    annotations = {
      "instrumentation.opentelemetry.io/inject-java" = "true"
    }
    labels = {
      app = "petclinic"
    }
  }

  spec {
    container {
      name  = "petclinic"
      image = "506463145083.dkr.ecr.us-west-2.amazonaws.com/cwagent-integ-test-petclinic:latest"

      port {
        container_port = 8080
      }

      env {
        name  = "OTEL_SERVICE_NAME"
        value = "petclinic-custom-service-name"
      }

    }
  }
}

# Traffic generator pod with bash command
resource "kubernetes_pod" "traffic_generator_instrumentation" {
  depends_on = [kubernetes_pod.petclinic_instrumentation, kubernetes_service.petclinic_service]
  metadata {
    name = "traffic-generator-instrumentation-default-env"
  }

  spec {
    container {
      name  = "traffic-generator"
      image = "alpine"

      # Run the curl command as a loop to repeatedly send requests
      command = ["/bin/sh", "-c"]
      args = [
        "apk add --no-cache curl && while true; do curl -s http://petclinic-service:8080/client-call; sleep 1; done"
      ]
    }
  }
}


# Service for Petclinic Pods to load-balance traffic
resource "kubernetes_service" "petclinic_service" {
  metadata {
    name = "petclinic-service"
  }

  spec {
    selector = {
      app = "petclinic"
    }

    port {
      port        = 8080
      target_port = 8080
    }
  }
}


# Get the single instance ID of the node in the node group
data "aws_instances" "eks_node" {
  depends_on = [
    aws_eks_node_group.this
  ]
  filter {
    name   = "tag:eks:nodegroup-name"
    values = [aws_eks_node_group.this.node_group_name]
  }
}

# Retrieve details of the single instance to get private DNS
data "aws_instance" "eks_node_detail" {
  depends_on = [
    data.aws_instances.eks_node
  ]
  instance_id = data.aws_instances.eks_node.ids[0]
}

resource "null_resource" "validator" {
  depends_on = [
    aws_eks_node_group.this,
    helm_release.aws_observability,
    null_resource.update_image,
    kubernetes_pod.log_generator,
    kubernetes_pod.petclinic_instrumentation,
    kubernetes_pod.traffic_generator_instrumentation
  ]

  triggers = {
    always_run = timestamp()
  }

  provisioner "local-exec" {
    command = <<-EOT
      echo "Validating EKS logs for entity fields"
      cd ../../../..
      go test ${var.test_dir} -timeout 1h -eksClusterName=${aws_eks_cluster.this.name} -computeType=EKS -v -eksDeploymentStrategy=DAEMON -instanceId=${data.aws_instance.eks_node_detail.instance_id}
    EOT
  }
}

