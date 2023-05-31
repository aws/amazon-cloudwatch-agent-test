// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

module "fluent_common" {
  source = "../common"
  cluster_name_suffix = "fluentbit"
}

# TODO could use template file
resource "kubernetes_config_map" "fluentbit_config" {
  depends_on = [
    module.fluent_common
  ]
  metadata {
    name      = "fluentbit-config"
    namespace = "amazon-cloudwatch"
    labels = {
      k8s-app = "fluentbit-cloudwatch"
    }
  }
  data = {
    "fluent-bit.conf" = <<EOF
    [SERVICE]
    Flush                     5
    Grace                     30
    Log_Level                 info
    Daemon                    off
    Parsers_File              parsers.conf
    HTTP_Server               $${HTTP_SERVER}
    HTTP_Listen               0.0.0.0
    HTTP_Port                 $${HTTP_PORT}
    storage.path              /var/fluent-bit/state/flb-storage/
    storage.sync              normal
    storage.checksum          off
    storage.backlog.mem_limit 5M

    @INCLUDE application-log.conf
    @INCLUDE dataplane-log.conf
    @INCLUDE host-log.conf
    EOF

    "application-log.conf" = <<EOF
    [INPUT]
    Name                tail
    Tag                 application.*
    Exclude_Path        /var/log/containers/cloudwatch-agent*, /var/log/containers/fluent-bit*, /var/log/containers/aws-node*, /var/log/containers/kube-proxy*
    Path                /var/log/containers/*.log
        multiline.parser    docker, cri
        DB                  /var/fluent-bit/state/flb_container.db
        Mem_Buf_Limit       50MB
        Skip_Long_Lines     On
        Refresh_Interval    10
        Rotate_Wait         30
        storage.type        filesystem
        Read_from_Head      $${READ_FROM_HEAD}

    [INPUT]
        Name                tail
        Tag                 application.*
        Path                /var/log/containers/fluent-bit*
        multiline.parser    docker, cri
        DB                  /var/fluent-bit/state/flb_log.db
        Mem_Buf_Limit       5MB
        Skip_Long_Lines     On
        Refresh_Interval    10
        Read_from_Head      $${READ_FROM_HEAD}

    [INPUT]
        Name                tail
        Tag                 application.*
        Path                /var/log/containers/cloudwatch-agent*
        multiline.parser    docker, cri
        DB                  /var/fluent-bit/state/flb_cwagent.db
        Mem_Buf_Limit       5MB
        Skip_Long_Lines     On
        Refresh_Interval    10
        Read_from_Head      $${READ_FROM_HEAD}

    [FILTER]
        Name                kubernetes
        Match               application.*
        Kube_URL            https://kubernetes.default.svc:443
        Kube_Tag_Prefix     application.var.log.containers.
        Merge_Log           On
        Merge_Log_Key       log_processed
        K8S-Logging.Parser  On
        K8S-Logging.Exclude Off
        Labels              Off
        Annotations         Off
        Use_Kubelet         On
        Kubelet_Port        10250
        Buffer_Size         0

    [OUTPUT]
        Name                cloudwatch_logs
        Match               application.*
        region              ${var.region}
        log_group_name      /aws/containerinsights/${module.fluent_common.cluster_name}/application
        log_stream_prefix   $${HOST_NAME}-
        auto_create_group   true
        extra_user_agent    container-insights
    EOF
    "dataplane-log.conf" = <<EOF
    [INPUT]
        Name                systemd
        Tag                 dataplane.systemd.*
        Systemd_Filter      _SYSTEMD_UNIT=docker.service
        Systemd_Filter      _SYSTEMD_UNIT=containerd.service
        Systemd_Filter      _SYSTEMD_UNIT=kubelet.service
        DB                  /var/fluent-bit/state/systemd.db
        Path                /var/log/journal
        Read_From_Tail      $${READ_FROM_TAIL}

    [INPUT]
        Name                tail
        Tag                 dataplane.tail.*
        Path                /var/log/containers/aws-node*, /var/log/containers/kube-proxy*
        multiline.parser    docker, cri
        DB                  /var/fluent-bit/state/flb_dataplane_tail.db
        Mem_Buf_Limit       50MB
        Skip_Long_Lines     On
        Refresh_Interval    10
        Rotate_Wait         30
        storage.type        filesystem
        Read_from_Head      $${READ_FROM_HEAD}

    [FILTER]
        Name                modify
        Match               dataplane.systemd.*
        Rename              _HOSTNAME                   hostname
        Rename              _SYSTEMD_UNIT               systemd_unit
        Rename              MESSAGE                     message
        Remove_regex        ^((?!hostname|systemd_unit|message).)*$

    [FILTER]
        Name                aws
        Match               dataplane.*
        imds_version        v1

    [OUTPUT]
        Name                cloudwatch_logs
        Match               dataplane.*
        region              ${var.region}
        log_group_name      /aws/containerinsights/${module.fluent_common.cluster_name}/dataplane
        log_stream_prefix   $${HOST_NAME}-
        auto_create_group   true
        extra_user_agent    container-insights
    EOF
    "host-log.conf" = <<EOF
    [INPUT]
        Name                tail
        Tag                 host.dmesg
        Path                /var/log/dmesg
        Key                 message
        DB                  /var/fluent-bit/state/flb_dmesg.db
        Mem_Buf_Limit       5MB
        Skip_Long_Lines     On
        Refresh_Interval    10
        Read_from_Head      $${READ_FROM_HEAD}

    [INPUT]
        Name                tail
        Tag                 host.messages
        Path                /var/log/messages
        Parser              syslog
        DB                  /var/fluent-bit/state/flb_messages.db
        Mem_Buf_Limit       5MB
        Skip_Long_Lines     On
        Refresh_Interval    10
        Read_from_Head      $${READ_FROM_HEAD}

    [INPUT]
        Name                tail
        Tag                 host.secure
        Path                /var/log/secure
        Parser              syslog
        DB                  /var/fluent-bit/state/flb_secure.db
        Mem_Buf_Limit       5MB
        Skip_Long_Lines     On
        Refresh_Interval    10
        Read_from_Head      $${READ_FROM_HEAD}

    [FILTER]
        Name                aws
        Match               host.*
        imds_version        v1

    [OUTPUT]
        Name                cloudwatch_logs
        Match               host.*
        region              ${var.region}
        log_group_name      /aws/containerinsights/${module.fluent_common.cluster_name}/host
        log_stream_prefix   $${HOST_NAME}.
        auto_create_group   true
        extra_user_agent    container-insights
    EOF
    "parsers.conf" = <<EOF
    [PARSER]
        Name                syslog
        Format              regex
        Regex               ^(?<time>[^ ]* {1,2}[^ ]* [^ ]*) (?<host>[^ ]*) (?<ident>[a-zA-Z0-9_\/\.\-]*)(?:\[(?<pid>[0-9]+)\])?(?:[^\:]*\:)? *(?<message>.*)$
        Time_Key            time
        Time_Format         %b %d %H:%M:%S

    [PARSER]
        Name                container_firstline
        Format              regex
        Regex               (?<log>(?<="log":")\S(?!\.).*?)(?<!\\)".*(?<stream>(?<="stream":").*?)".*(?<time>\d{4}-\d{1,2}-\d{1,2}T\d{2}:\d{2}:\d{2}\.\w*).*(?=})
        Time_Key            time
        Time_Format         %Y-%m-%dT%H:%M:%S.%LZ

    [PARSER]
        Name                cwagent_firstline
        Format              regex
        Regex               (?<log>(?<="log":")\d{4}[\/-]\d{1,2}[\/-]\d{1,2}[ T]\d{2}:\d{2}:\d{2}(?!\.).*?)(?<!\\)".*(?<stream>(?<="stream":").*?)".*(?<time>\d{4}-\d{1,2}-\d{1,2}T\d{2}:\d{2}:\d{2}\.\w*).*(?=})
        Time_Key            time
        Time_Format         %Y-%m-%dT%H:%M:%S.%LZ
    EOF
  }
}

resource "kubernetes_daemonset" "service" {
  depends_on = [
    module.fluent_common,
    kubernetes_config_map.fluentbit_config,
  ]
  metadata {
    name      = "fluentbit-cloudwatch"
    namespace = "amazon-cloudwatch"
    labels = {
      k8s-app = "fluentbit-cloudwatch"
      version = "v1"
      "kubernetes.io/cluster-service" = "true"
    }
  }
  spec {
    selector {
      match_labels = {
        k8s-app = "fluentbit-cloudwatch"
      }
    }
    template {
      metadata {
        labels = {
          k8s-app = "fluentbit-cloudwatch"
          version = "v1"
          "kubernetes.io/cluster-service" = "true"
        }
      }
      spec {
        container {
          name              = "fluentbit-cloudwatch"
          image             = "public.ecr.aws/aws-observability/aws-for-fluent-bit:stable"
          image_pull_policy = "Always"
          resources {
            limits = {
              "memory" = "200Mi"
            }
            requests = {
              "cpu" = "500m",
              "memory" = "200Mi"
            }
          }
          env {
            name = "AWS_REGION"
            value = var.region
          }
          env {
            name = "CLUSTER_NAME"
            value = module.fluent_common.cluster_name
          }
          env {
            name = "HTTP_SERVER"
            value = "On"
          }
          env {
            name = "HTTP_PORT"
            value = "2020"
          }
          env {
            name = "READ_FROM_HEAD"
            value = "Off"
          }
          env {
            name = "READ_FROM_TAIL"
            value = "On"
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
            name = "HOSTNAME"
            value_from {
              field_ref {
                field_path = "metadata.name"
              }
            }
          }
          env {
            name = "CI_VERSION"
            value= "k8s/1.3.15"
          }
          volume_mount {
            mount_path = "/var/fluent-bit/state"
            name       = "fluentbitstate"
          }
          volume_mount {
            mount_path = "/var/log"
            name       = "varlog"
            read_only = true
          }
          volume_mount {
            mount_path = "/var/lib/docker/containers"
            name       = "varlibdockercontainers"
            read_only  = true
          }
          volume_mount {
            mount_path = "/fluent-bit/etc/"
            name       = "fluent-bit-config"
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
        termination_grace_period_seconds = 10
        host_network = true
        dns_policy = "ClusterFirstWithHostNet"
        volume {
          name = "fluentbitstate"
          host_path {
            path = "/var/fluent-bit/state"
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
          name = "fluent-bit-config"
          config_map {
            name = "fluentbit-config"
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
        service_account_name = "cloudwatch-agent"
        toleration {
          key = "node-role.kubernetes.io/master"
          operator = "Exists"
          effect = "NoSchedule"
        }
        toleration {
          operator = "Exists"
          effect = "NoExecute"
        }
        toleration {
          operator = "Exists"
          effect = "NoSchedule"
        }
      }
    }
  }
}

resource "null_resource" "validator" {
  depends_on = [
    module.fluent_common,
    kubernetes_daemonset.service,
  ]
  provisioner "local-exec" {
    command = <<-EOT
      echo "Validating EKS fluentbit logs"
      cd ../../../../..
      go test ${var.test_dir} -eksClusterName=${module.fluent_common.cluster_name} -computeType=EKS -v -eksDeploymentStrategy=DAEMON
    EOT
  }
}
