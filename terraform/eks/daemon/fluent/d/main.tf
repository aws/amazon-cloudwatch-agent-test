// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "fluent_common" {
  source        = "../common"
  ami_type      = var.ami_type
  instance_type = var.instance_type
}

resource "kubernetes_config_map" "cluster_info" {
  depends_on = [
    module.fluent_common
  ]
  metadata {
    name      = "cluster-info"
    namespace = "amazon-cloudwatch"
  }
  data = {
    "cluster.name" = module.fluent_common.cluster_name
    "logs.region"  = var.region
  }
}

resource "kubernetes_service_account" "fluentd_service" {
  metadata {
    name      = "fluentd"
    namespace = "amazon-cloudwatch"
  }
}

resource "kubernetes_cluster_role" "fluentd_clusterrole" {
  metadata {
    name = "fluentd-role"
  }
  rule {
    verbs      = ["get", "list", "watch"]
    resources  = ["namespaces", "pods", "pods/logs"]
    api_groups = [""]
  }
}

resource "kubernetes_cluster_role_binding" "fluentd_rolebinding" {
  depends_on = [
    kubernetes_service_account.fluentd_service,
    kubernetes_cluster_role.fluentd_clusterrole
  ]
  metadata {
    name = "fluentd-role-binding"
  }
  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "ClusterRole"
    name      = "fluentd-role"
  }
  subject {
    kind      = "ServiceAccount"
    name      = "fluentd"
    namespace = "amazon-cloudwatch"
  }
}

resource "kubernetes_config_map" "fluentd_config" {
  depends_on = [
    module.fluent_common
  ]
  metadata {
    name      = "fluentd-config"
    namespace = "amazon-cloudwatch"
    labels = {
      k8s-app = "fluentd-cloudwatch"
    }
  }
  data = {
    "kubernetes.conf" = <<EOF
kubernetes.conf
    EOF
    "fluent.conf"     = <<EOF
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
    @type "#{ENV['FLUENT_CONTAINER_TAIL_PARSER_TYPE'] || 'json'}"
    time_format %Y-%m-%dT%H:%M:%S.%N%:z
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
    watch false
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
    watch false
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
    watch false
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
    log_group_name "/aws/containerinsights/${module.fluent_common.cluster_name}/application"
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
    "systemd.conf"    = <<EOF
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
    watch false
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
    log_group_name "/aws/containerinsights/${module.fluent_common.cluster_name}/dataplane"
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
    "host.conf"       = <<EOF
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
    watch false
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
    log_group_name "/aws/containerinsights/${module.fluent_common.cluster_name}/host"
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

resource "kubernetes_daemonset" "fluentd_daemon" {
  depends_on = [
    module.fluent_common,
    kubernetes_service_account.fluentd_service,
    kubernetes_config_map.fluentd_config,
  ]
  metadata {
    name      = "fluentd-cloudwatch"
    namespace = "amazon-cloudwatch"
  }
  spec {
    selector {
      match_labels = {
        "k8s-app" : "fluentd-cloudwatch"
      }
    }
    template {
      metadata {
        labels = {
          "k8s-app" : "fluentd-cloudwatch"
        }
      }
      spec {
        service_account_name             = "fluentd"
        termination_grace_period_seconds = 30
        init_container {
          name    = "copy-fluentd-config"
          image   = "busybox"
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
          name    = "update-log-driver"
          image   = "busybox"
          command = ["sh", "-c", ""]
        }
        container {
          name  = "fluentd-cloudwatch"
          image = "fluent/fluentd-kubernetes-daemonset:v1.7.3-debian-cloudwatch-1.0"
          env {
            name = "AWS_REGION"
            value_from {
              config_map_key_ref {
                name = "cluster-info"
                key  = "logs.region"
              }
            }
          }
          env {
            name = "CLUSTER_NAME"
            value_from {
              config_map_key_ref {
                name = "cluster-info"
                key  = "cluster.name"
              }
            }
          }
          env {
            name  = "CI_VERSION"
            value = "k8s/1.3.15"
          }
          env {
            name  = "FLUENT_CONTAINER_TAIL_PARSER_TYPE"
            value = "/^(?<time>.+) (?<stream>stdout|stderr) (?<logtag>[FP]) (?<log>.*)$/"
          }
          resources {
            limits = {
              "memory" : "400Mi"
            }
            requests = {
              "cpu" : "100m",
              "memory" : "200Mi"
            }
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
            mount_path = "/fluentd/etc/kubernetes.conf"
            name       = "fluentd-config"
            sub_path   = "kubernetes.conf"
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
          name = "fluentd-config"
          config_map {
            name = "fluentd-config"
            items {
              key  = "kubernetes.conf"
              path = "kubernetes.conf"
            }
          }
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
          name = "dmesg"
          host_path {
            path = "/var/log/dmesg"
          }
        }
      }
    }
  }
}

resource "null_resource" "validator" {
  depends_on = [
    module.fluent_common,
    kubernetes_daemonset.fluentd_daemon,
  ]
  provisioner "local-exec" {
    command = <<-EOT
      echo "Validating EKS fluentd logs"
      cd ../../../../..
      go test ${var.test_dir} -eksClusterName=${module.fluent_common.cluster_name} -computeType=EKS -v -eksDeploymentStrategy=DAEMON
    EOT
  }
}
