// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "common" {
  source             = "../../../common"
  cwagent_image_repo = var.cwagent_image_repo
  cwagent_image_tag  = var.cwagent_image_tag
}

module "basic_components" {
  source = "../../../basic_components"

  region = var.region
}

data "aws_eks_cluster_auth" "this" {
  name = aws_eks_cluster.this.name
}

resource "aws_eks_cluster" "this" {
  name     = "cwagent-eks-integ-${module.common.testing_id}"
  role_arn = module.basic_components.role_arn
  version  = var.k8s_version
  enabled_cluster_log_types = [
    "api",
    "audit",
    "authenticator",
    "controllerManager",
    "scheduler"
  ]
  vpc_config {
    subnet_ids         = module.basic_components.public_subnet_ids
    security_group_ids = [module.basic_components.security_group]
  }
}

# EKS Node Groups
resource "aws_eks_node_group" "this" {
  cluster_name    = aws_eks_cluster.this.name
  node_group_name = "cwagent-eks-integ-node"
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
    aws_iam_role_policy_attachment.node_CloudWatchAgentServerPolicy
  ]
}

# EKS Node IAM Role
resource "aws_iam_role" "node_role" {
  name = "cwagent-eks-Worker-Role-${module.common.testing_id}"
  assume_role_policy = jsonencode({
    Version = "2012-10-17",
    Statement = [
      {
        Effect = "Allow",
        Principal = {
          Service = "ec2.amazonaws.com"
        },
        Action = "sts:AssumeRole"
      }
    ]
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

resource "aws_iam_role_policy_attachment" "node_CloudWatchAgentServerPolicy" {
  policy_arn = "arn:aws:iam::aws:policy/CloudWatchAgentServerPolicy"
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


# create cert for communication between agent and dcgm
resource "tls_private_key" "private_key" {
  algorithm = "RSA"
}

resource "local_file" "ca_key" {
  content  = tls_private_key.private_key.private_key_pem
  filename = "${path.module}/certs/ca.key"
}

resource "tls_self_signed_cert" "ca_cert" {
  private_key_pem   = tls_private_key.private_key.private_key_pem
  is_ca_certificate = true
  subject {
    common_name  = "dcgm-exporter-service.amazon-cloudwatch.svc"
    organization = "Amazon CloudWatch Agent"
  }
  validity_period_hours = 24
  allowed_uses = [
    "digital_signature",
    "key_encipherment",
    "cert_signing",
    "crl_signing",
    "server_auth",
    "client_auth",
  ]
}

resource "local_file" "ca_cert_file" {
  content  = tls_self_signed_cert.ca_cert.cert_pem
  filename = "${path.module}/certs/ca.cert"
}

resource "tls_private_key" "server_private_key" {
  algorithm = "RSA"
}

resource "local_file" "server_key" {
  content  = tls_private_key.server_private_key.private_key_pem
  filename = "${path.module}/certs/server.key"
}

resource "tls_cert_request" "local_csr" {
  private_key_pem = tls_private_key.server_private_key.private_key_pem
  dns_names       = ["localhost", "127.0.0.1", "dcgm-exporter-service.amazon-cloudwatch.svc"]
  subject {
    common_name  = "dcgm-exporter-service.amazon-cloudwatch.svc"
    organization = "Amazon CloudWatch Agent"
  }
}

resource "tls_locally_signed_cert" "server_cert" {
  cert_request_pem      = tls_cert_request.local_csr.cert_request_pem
  ca_private_key_pem    = tls_private_key.private_key.private_key_pem
  ca_cert_pem           = tls_self_signed_cert.ca_cert.cert_pem
  validity_period_hours = 12
  allowed_uses = [
    "digital_signature",
    "key_encipherment",
    "server_auth",
    "client_auth",
  ]
}

resource "local_file" "server_cert_file" {
  content  = tls_locally_signed_cert.server_cert.cert_pem
  filename = "${path.module}/certs/server.cert"
}

resource "kubernetes_secret" "agent_cert" {
  metadata {
    name      = "amazon-cloudwatch-observability-agent-cert"
    namespace = "amazon-cloudwatch"
  }
  data = {
    "ca.crt"  = tls_self_signed_cert.ca_cert.cert_pem              #filebase64(local_file.ca_cert_file.filename)
    "tls.crt" = tls_locally_signed_cert.server_cert.cert_pem       #filebase64(local_file.server_cert_file.filename)
    "tls.key" = tls_private_key.server_private_key.private_key_pem #filebase64(local_file.server_key.filename)
  }
}


resource "kubernetes_namespace" "namespace" {
  metadata {
    name = "amazon-cloudwatch"
  }
}






##########################################
# Template Files
##########################################
locals {
  httpd_config     = "../../../../${var.test_dir}/resources/httpd.conf"
  httpd_ssl_config = "../../../../${var.test_dir}/resources/httpd-ssl.conf"
  cwagent_config   = fileexists("../../../../${var.test_dir}/resources/config.json") ? "../../../../${var.test_dir}/resources/config.json" : "../default_resources/default_amazon_cloudwatch_agent.json"
  role_arn = format("%s%s", module.basic_components.role_arn, var.beta ? "-eks-beta" : "")
  aws_eks  = format("%s%s", "aws eks --region ${var.region}", var.beta ? " --endpoint ${var.beta_endpoint}" : "")
}

data "template_file" "cwagent_config" {
  template = file(local.cwagent_config)
  vars = {
  }
}

resource "kubernetes_config_map" "cwagentconfig" {
  depends_on = [
    kubernetes_namespace.namespace,
    kubernetes_service_account.cwagentservice
  ]
  metadata {
    name      = "cwagentconfig"
    namespace = "amazon-cloudwatch"
  }
  data = {
    "cwagentconfig.json" : data.template_file.cwagent_config.rendered
  }
}

data "template_file" "httpd_config" {
  template = file(local.httpd_config)
  vars     = {}
}
data "template_file" "httpd_ssl_config" {
  template = file(local.httpd_ssl_config)
  vars     = {}
}





resource "kubernetes_cluster_role" "clusterrole" {
  depends_on = [kubernetes_namespace.namespace]
  metadata {
    name = "cloudwatch-agent-role"
  }
  rule {
    verbs      = ["get", "list", "watch"]
    resources  = ["pods", "pods/logs", "nodes", "nodes/proxy", "namespaces", "endpoints"]
    api_groups = [""]
  }
  rule {
    verbs      = ["list", "watch"]
    resources  = ["replicasets"]
    api_groups = ["apps"]
  }
  rule {
    verbs      = ["list", "watch"]
    resources  = ["jobs"]
    api_groups = ["batch"]
  }
  rule {
    verbs      = ["get"]
    resources  = ["nodes/proxy"]
    api_groups = [""]
  }
  rule {
    verbs      = ["create"]
    resources  = ["nodes/stats", "configmaps", "events"]
    api_groups = [""]
  }
  rule {
    verbs          = ["get", "update"]
    resource_names = ["cwagent-clusterleader"]
    resources      = ["configmaps"]
    api_groups     = [""]
  }
  rule {
    verbs      = ["list", "watch"]
    resources  = ["services"]
    api_groups = [""]
  }
  rule {
    non_resource_urls = ["/metrics"]
    verbs             = ["get", "list", "watch"]
  }
}

resource "kubernetes_cluster_role_binding" "rolebinding" {
  depends_on = [kubernetes_namespace.namespace]
  metadata {
    name = "cloudwatch-agent-role-binding"
  }
  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "ClusterRole"
    name      = "cloudwatch-agent-role"
  }
  subject {
    kind      = "ServiceAccount"
    name      = "cloudwatch-agent"
    namespace = "amazon-cloudwatch"
  }
}


resource "null_resource" "kubectl" {
  depends_on = [
    aws_eks_cluster.this,
    aws_eks_node_group.this
  ]
  provisioner "local-exec" {
    command = <<-EOT
      ${local.aws_eks} update-kubeconfig --name ${aws_eks_cluster.this.name}
      ${local.aws_eks} list-clusters --output text
      ${local.aws_eks} describe-cluster --name ${aws_eks_cluster.this.name} --output text
    EOT
  }
}


resource "null_resource" "kubectl" {
  depends_on = [
    aws_eks_cluster.this,
    aws_eks_node_group.this
  ]
  provisioner "local-exec" {
    command = <<-EOT
      ${local.aws_eks} update-kubeconfig --name ${aws_eks_cluster.this.name}
      ${local.aws_eks} list-clusters --output text
      ${local.aws_eks} describe-cluster --name ${aws_eks_cluster.this.name} --output text
    EOT
  }
}

resource "aws_eks_addon" "this" {
  depends_on = [
    null_resource.kubectl
  ]
  addon_name   = var.addon_name
  cluster_name = aws_eks_cluster.this.name
  addon_version = var.addon_version
}

resource "null_resource" "validator" {
  depends_on = [
      aws_eks_node_group.this,
      aws_eks_addon.this
  ]

resource "null_resource" "validator" {
  depends_on = [
      aws_eks_node_group.this,
      aws_eks_addon.this
  ]

  provisioner "local-exec" {
    command = <<EOT
      if go test ${var.test_dir} -eksClusterName ${aws_eks_cluster.this.name} -computeType=EKS -v -eksDeploymentStrategy=DAEMON -eksGpuType=nvidia; then
        # Get all pods and describe them
          kubectl get pods --all-namespaces -o wide > pods.txt
          kubectl describe pods --all-namespaces > pods_describe.txt

          # Log the contents of the files
          cat pods.txt
          cat pods_describe.txt
        echo "Tests passed"

      else
      # Get all pods and describe them
        kubectl get pods --all-namespaces -o wide > pods.txt
        kubectl describe pods --all-namespaces > pods_describe.txt

        # Log the contents of the files
        cat pods.txt
        cat pods_describe.txt
        echo "Tests failed"
        exit 1
      fi


    EOT
  }
}
