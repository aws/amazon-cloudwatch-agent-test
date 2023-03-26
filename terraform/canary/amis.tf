variable "ami_family" {
  default = {
    linux = {
      login_user               = "ec2-user"
      install_package          = "amazon-cloudwatch-agent.rpm"
      temp_folder              = "/tmp"
      agent_config_destination = "/tmp/agent_config.json"
      download_command_pattern = "aws s3 cp %s amazon-cloudwatch-agent.rpm"
      install_command          = "sudo rpm -Uvh amazon-cloudwatch-agent.rpm"
      start_command            = "sudo /opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl -a fetch-config -m ec2 -s -c file:/tmp/agent_config.json"
      status_command           = "sudo /opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl -a status"
      connection_type          = "ssh"
      user_data                = ""
      wait_cloud_init          = "for i in {1..300}; do [ ! -f /var/lib/cloud/instance/boot-finished ] && echo 'Waiting for cloud-init...'$i && sleep 1 || break; done"
    }
    windows = {
      login_user               = "Administrator"
      install_package          = "amazon-cloudwatch-agent.msi"
      temp_folder              = "C:\\"
      agent_config_destination = "C:\\agent_config.json"
      download_command_pattern = "aws s3 cp %s amazon-cloudwatch-agent.msi"
      install_command          = "msiexec /i C:\\amazon-cloudwatch-agent.msi"
      start_command            = "powershell \"& 'C:\\Program Files\\Amazon\\AmazonCloudWatchAgent\\amazon-cloudwatch-agent-ctl.ps1' -a fetch-config -m ec2 -s -c C:\\agent_config.json\""
      status_command           = "powershell \"& 'C:\\Program Files\\Amazon\\AmazonCloudWatchAgent\\amazon-cloudwatch-agent-ctl.ps1' -a status\""
      connection_type          = "winrm"
      wait_cloud_init          = " "
    }
  }
}
