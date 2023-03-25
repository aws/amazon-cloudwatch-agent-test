variable "ami_family" {
  default = {
    amazon_linux = {
      login_user               = "ec2-user"
      install_package          = "aws-otel-collector.rpm"
      instance_type            = "m5.2xlarge"
      otconfig_destination     = "/tmp/ot-default.yml"
      download_command_pattern = "wget %s"
      install_command          = "sudo rpm -Uvh aws-otel-collector.rpm"
      start_command            = "sudo /opt/aws/aws-otel-collector/bin/aws-otel-collector-ctl -c /tmp/ot-default.yml -a start"
      status_command           = "sudo /opt/aws/aws-otel-collector/bin/aws-otel-collector-ctl -c /tmp/ot-default.yml -a status"
      ssm_validate             = "sudo /opt/aws/aws-otel-collector/bin/aws-otel-collector-ctl -c /tmp/ot-default.yml -a status | grep running"
      connection_type          = "ssh"
      wait_cloud_init          = "for i in {1..60}; do [ ! -f /var/lib/cloud/instance/boot-finished ] && echo 'Waiting for cloud-init...' && sleep 1 || break; done"
    }
    windows = {
      login_user               = "Administrator"
      install_package          = "aws-otel-collector.msi"
      instance_type            = "t3.medium"
      otconfig_destination     = "C:\\ot-default.yml"
      download_command_pattern = "powershell -command \"Invoke-WebRequest -Uri %s -OutFile C:\\aws-otel-collector.msi\""
      install_command          = "msiexec /i C:\\aws-otel-collector.msi"
      start_command            = "powershell \"& 'C:\\Program Files\\Amazon\\AwsOtelCollector\\aws-otel-collector-ctl.ps1' -ConfigLocation C:\\ot-default.yml -Action start\""
      status_command           = "powershell \"& 'C:\\Program Files\\Amazon\\AwsOtelCollector\\aws-otel-collector-ctl.ps1' -ConfigLocation C:\\ot-default.yml -Action status\""
      ssm_validate             = "powershell \"& 'C:\\Program Files\\Amazon\\AwsOtelCollector\\aws-otel-collector-ctl.ps1' -ConfigLocation C:\\ot-default.yml -Action status\" | findstr running"
      connection_type          = "winrm"
      wait_cloud_init          = " "
    }
  }
}
