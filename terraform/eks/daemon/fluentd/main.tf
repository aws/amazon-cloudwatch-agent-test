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

  ami_type       = "AL2_x86_64"
  capacity_type  = "ON_DEMAND"
  disk_size      = 20
  instance_types = ["t3.medium"]

  depends_on = [
    aws_iam_role_policy_attachment.node_AmazonEC2ContainerRegistryReadOnly,
    aws_iam_role_policy_attachment.node_AmazonEKS_CNI_Policy,
    aws_iam_role_policy_attachment.node_AmazonEKSWorkerNodePolicy,
    aws_iam_role_policy_attachment.node_CloudWatchAgentServerPolicy,
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

resource "kubernetes_namespace" "namespace" {
  metadata {
    name = "amazon-cloudwatch"
  }
}

resource "kubernetes_service_account" "cwagentservice" {
  depends_on = [kubernetes_namespace.namespace]
  metadata {
    name      = "cloudwatch-agent"
    namespace = "amazon-cloudwatch"
  }
}

resource "kubernetes_cluster_role" "clusterrole" {
  depends_on = [kubernetes_namespace.namespace]
  metadata {
    name = "cloudwatch-agent-role"
  }
  rule {
    verbs      = ["list", "watch"]
    resources  = ["pods", "nodes", "endpoints"]
    api_groups = [""]
  }
  rule {
    verbs      = ["get", "list", "watch"]
    resources  = ["namespaces", "pods/logs"]
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

##########################################
# Template Files
##########################################
locals {
  fluentd_config = fileexists("../../../${var.test_dir}/resources/fluentd_config.conf") ? "../../../${var.test_dir}/resources/fluentd_config.conf" : "../default_resources/fluentd.conf"
}

data "template_file" "fluentd_config" {
  template = file(local.fluentd_config)
  vars = {
    region       = var.region
    cluster_name = aws_eks_cluster.this.name
    stream_name  = "EKS-fluentd-${aws_eks_cluster.this.name}"
    testing_id   = module.common.testing_id
  }
}

resource "kubernetes_config_map" "fluentdconfig" {
  depends_on = [
    kubernetes_namespace.namespace,
    kubernetes_service_account.cwagentservice
  ]
  metadata {
    name      = "fluentd-config"
    namespace = "amazon-cloudwatch"
    labels {
      k8s-app = "fluentd-cloudwatch"
    }
  }
  data = {
    "fluent.conf" = <<EOF
    @include containers.conf
    @include systemd.conf
    @include host.conf

    <match fluent.**>
      @type null
    </match>
    EOF
    "containers.conf" = <<EOF
    <source>
      @type tail
      @id in_tail_container_logs
      @label @containers
      path /var/log/containers/*.log
      exclude_path ["/var/log/containers/cloudwatch-agent*", "/var/log/containers/fluentd*"]
      pos_file /var/log/fluentd-containers.log.pos
      tag *
      read_from_head true
      <parse>
        @type json
        time_format %Y-%m-%dT%H:%M:%S.%NZ
      </parse>
    </source>
    
    <source>
      @type tail
      @id in_tail_cwagent_logs
      @label @cwagentlogs
      path /var/log/containers/cloudwatch-agent*
      pos_file /var/log/cloudwatch-agent.log.pos
      tag *
      read_from_head true
      <parse>
        @type json
        time_format %Y-%m-%dT%H:%M:%S.%NZ
      </parse>
    </source>
    
    <source>
      @type tail
      @id in_tail_fluentd_logs
      @label @fluentdlogs
      path /var/log/containers/fluentd*
      pos_file /var/log/fluentd.log.pos
      tag *
      read_from_head true
      <parse>
        @type json
        time_format %Y-%m-%dT%H:%M:%S.%NZ
      </parse>
    </source>
    
    <label @fluentdlogs>
      <filter **>
        @type kubernetes_metadata
        @id filter_kube_metadata_fluentd
      </filter>
    
      <filter **>
        @type record_transformer
        @id filter_fluentd_stream_transformer
        <record>
          stream_name $${tag_parts[3]}
        </record>
      </filter>
    
      <match **>
        @type relabel
        @label @NORMAL
      </match>
    </label>
    
    <label @containers>
      <filter **>
        @type kubernetes_metadata
        @id filter_kube_metadata
      </filter>
    
      <filter **>
        @type record_transformer
        @id filter_containers_stream_transformer
        <record>
          stream_name $${tag_parts[3]}
        </record>
      </filter>
    
      <filter **>
        @type concat
        key log
        multiline_start_regexp /^\S/
        separator ""
        flush_interval 5
        timeout_label @NORMAL
      </filter>
    
      <match **>
        @type relabel
        @label @NORMAL
      </match>
    </label>
    
    <label @cwagentlogs>
      <filter **>
        @type kubernetes_metadata
        @id filter_kube_metadata_cwagent
      </filter>
    
      <filter **>
        @type record_transformer
        @id filter_cwagent_stream_transformer
        <record>
          stream_name $${tag_parts[3]}
        </record>
      </filter>
    
      <filter **>
        @type concat
        key log
        multiline_start_regexp /^\d{4}[-/]\d{1,2}[-/]\d{1,2}/
        separator ""
        flush_interval 5
        timeout_label @NORMAL
      </filter>
    
      <match **>
        @type relabel
        @label @NORMAL
      </match>
    </label>
    
    <label @NORMAL>
      <match **>
        @type cloudwatch_logs
        @id out_cloudwatch_logs_containers
        region "${var.region}"
        log_group_name "/aws/containerinsights/${aws_eks_cluster.this.name}/application"
        log_stream_name_key stream_name
        remove_log_stream_name_key true
        auto_create_stream true
        <buffer>
          flush_interval 5
          chunk_limit_size 2m
          queued_chunks_limit_size 32
          retry_forever true
        </buffer>
      </match>
    </label>
  EOF
  "systemd.conf" = <<EOF
    <source>
      @type systemd
      @id in_systemd_kubelet
      @label @systemd
      filters [{ "_SYSTEMD_UNIT": "kubelet.service" }]
      <entry>
        field_map {"MESSAGE": "message", "_HOSTNAME": "hostname", "_SYSTEMD_UNIT": "systemd_unit"}
        field_map_strict true
      </entry>
      path /var/log/journal
      <storage>
        @type local
        persistent true
        path /var/log/fluentd-journald-kubelet-pos.json
      </storage>
      read_from_head true
      tag kubelet.service
    </source>
    
    <source>
      @type systemd
      @id in_systemd_kubeproxy
      @label @systemd
      filters [{ "_SYSTEMD_UNIT": "kubeproxy.service" }]
      <entry>
        field_map {"MESSAGE": "message", "_HOSTNAME": "hostname", "_SYSTEMD_UNIT": "systemd_unit"}
        field_map_strict true
      </entry>
      path /var/log/journal
      <storage>
        @type local
        persistent true
        path /var/log/fluentd-journald-kubeproxy-pos.json
      </storage>
      read_from_head true
      tag kubeproxy.service
    </source>
    
    <source>
      @type systemd
      @id in_systemd_docker
      @label @systemd
      filters [{ "_SYSTEMD_UNIT": "docker.service" }]
      <entry>
        field_map {"MESSAGE": "message", "_HOSTNAME": "hostname", "_SYSTEMD_UNIT": "systemd_unit"}
        field_map_strict true
      </entry>
      path /var/log/journal
      <storage>
        @type local
        persistent true
        path /var/log/fluentd-journald-docker-pos.json
      </storage>
      read_from_head true
      tag docker.service
    </source>
    
    <label @systemd>
      <filter **>
        @type kubernetes_metadata
        @id filter_kube_metadata_systemd
      </filter>
    
      <filter **>
        @type record_transformer
        @id filter_systemd_stream_transformer
        <record>
          stream_name $${tag}-$${record["hostname"]}
        </record>
      </filter>
    
      <match **>
        @type cloudwatch_logs
        @id out_cloudwatch_logs_systemd
        region "${var.region}"
        log_group_name "/aws/containerinsights/${aws_eks_cluster.this.name}/dataplane"
        log_stream_name_key stream_name
        auto_create_stream true
        remove_log_stream_name_key true
        <buffer>
          flush_interval 5
          chunk_limit_size 2m
          queued_chunks_limit_size 32
          retry_forever true
        </buffer>
      </match>
    </label>
    EOF
  "host.conf" = <<EOF
    <source>
      @type tail
      @id in_tail_dmesg
      @label @hostlogs
      path /var/log/dmesg
      pos_file /var/log/dmesg.log.pos
      tag host.dmesg
      read_from_head true
      <parse>
        @type syslog
      </parse>
    </source>
    
    <source>
      @type tail
      @id in_tail_secure
      @label @hostlogs
      path /var/log/secure
      pos_file /var/log/secure.log.pos
      tag host.secure
      read_from_head true
      <parse>
        @type syslog
      </parse>
    </source>
    
    <source>
      @type tail
      @id in_tail_messages
      @label @hostlogs
      path /var/log/messages
      pos_file /var/log/messages.log.pos
      tag host.messages
      read_from_head true
      <parse>
        @type syslog
      </parse>
    </source>
    
    <label @hostlogs>
      <filter **>
        @type kubernetes_metadata
        @id filter_kube_metadata_host
      </filter>
    
      <filter **>
        @type record_transformer
        @id filter_containers_stream_transformer_host
        <record>
          stream_name $${tag}-$${record["host"]}
        </record>
      </filter>
    
      <match host.**>
        @type cloudwatch_logs
        @id out_cloudwatch_logs_host_logs
        region "${var.region}"
        log_group_name "/aws/containerinsights/${aws_eks_cluster.this.name}/host"
        log_stream_name_key stream_name
        remove_log_stream_name_key true
        auto_create_stream true
        <buffer>
          flush_interval 5
          chunk_limit_size 2m
          queued_chunks_limit_size 32
          retry_forever true
        </buffer>
      </match>
    </label>
    EOF
  }
}

resource "kubernetes_daemonset" "service" {
  depends_on = [
    kubernetes_namespace.namespace,
    kubernetes_config_map.fluentdconfig,
    kubernetes_service_account.cwagentservice,
    aws_eks_node_group.this
  ]
  metadata {
    name      = "cloudwatch-agent"
    namespace = "amazon-cloudwatch"
  }
  spec {
    selector {
      match_labels = {
        "name" : "cloudwatch-agent"
      }
    }
    template {
      metadata {
        labels = {
          "name" : "cloudwatch-agent"
        }
      }
      spec {
        node_selector = {
          "kubernetes.io/os" : "linux"
        }
        init_container {
          name = "copy-fluentd-config"
          image = "busybox"
          command = ["sh", "-c", "cp /config-volume/..data/* /fluentd/etc"]
          volume_mount {
            mount_path = "/config-volume"
            name       = "config-volume"
          }
          volume_mount {
            mount_path = "/fluentd/etc"
            name       = "fluentdconf"
          }
        }
        init_container {
          name = "update-log-driver"
          image = "busybox"
          command = ["sh", "-c", ""]
        }
        container {
          name              = "cwagent"
          image             = "fluent/fluentd-kubernetes-daemonset:v1.7.3-debian-cloudwatch-1.0"
          image_pull_policy = "Always"
          resources {
            limits = {
              "cpu" : "200m",
              "memory" : "400Mi"
            }
            requests = {
              "cpu" : "200m",
              "memory" : "200Mi"
            }
          }
          env {
            name = "HOST_IP"
            value_from {
              field_ref {
                field_path = "status.hostIP"
              }
            }
          }
          env {
            name = "HOST_NAME"
            value_from {
              field_ref {
                field_path = "spec.nodeName"
              }
            }
          }
          env {
            name = "K8S_NAMESPACE"
            value_from {
              field_ref {
                field_path = "metadata.namespace"
              }
            }
          }
          env {
            name = "REGION"
            value = var.region
          }
          env {
            name = "CLUSTER_NAME"
            value = aws_eks_cluster.this.name
          }
          env {
            name = "CI_VERSION"
            value= "k8s/1.2.3"
          }
          volume_mount {
            mount_path = "/config-volume"
            name       = "config-volume"
          }
          volume_mount {
            mount_path = "/fluentd/etc"
            name       = "fluentdconf"
          }
          volume_mount {
            mount_path = "/var/log"
            name       = "varlog"
          }
          volume_mount {
            mount_path = "/var/lib/docker/containers"
            name       = "varlibdockercontainers"
            read_only  = true
          }
          volume_mount {
            mount_path = "/run/log/journal"
            name       = "runlogjournal"
            read_only  = true
          }
          volume_mount {
            mount_path = "/var/log/dmesg"
            name       = "dmesg"
            read_only  = true
          }
        }
        volume {
          name = "config-volume"
          config_map {
            name = "fluentd-config"
          }
        }
        volume {
          name = "fluentdconf"
          empty_dir {}
        }
        volume {
          name = "varlog"
          host_path {
            path = "/var/log"
          }
        }
        volume {
          name = "varlibdockercontainers"
          host_path {
            path = "/var/lib/docker/containers"
          }
        }
        volume {
          name = "runlogjournal"
          host_path {
            path = "/run/log/journal"
          }
        }
        volume {
          name = "varlibdocker"
          host_path {
            path = "/var/lib/docker"
          }
        }
        volume {
          name = "dmesg"
          host_path {
            path = "/var/log/dmesg"
          }
        }
        service_account_name             = "cloudwatch-agent"
        termination_grace_period_seconds = 60
      }
    }
  }
}

resource "null_resource" "validator" {
  depends_on = [
    aws_eks_node_group.this,
    kubernetes_daemonset.service,
    kubernetes_cluster_role_binding.rolebinding,
    kubernetes_service_account.cwagentservice,
  ]
  provisioner "local-exec" {
    command = <<-EOT
      echo "Validating EKS metrics/logs"
      cd ../../../..
      go test ${var.test_dir} -eksClusterName=${aws_eks_cluster.this.name} -computeType=EKS -v -eksDeploymentStrategy=DAEMON
    EOT
  }
}
