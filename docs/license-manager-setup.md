# Set up License Manager for dedicated hosts
Terraform does not support retrieving a list of available dedicated hosts and the filter option is not specific enough to target a single host, therefore we have to use AWS License Manager to help with automatically selecting available host for testing.

# 1. Self-managed licenses
License manager requires a license to manage all the dedicated hosts own by a account. For test purpose, we will create a license that allows high core limits so we will not face throttle issues when launching test instanecs.
1. Go to AWS console
2. Go to AWS License Manager
3. Click on `Self-Managed License` on the sidebar
4. Click on `Create self-managed license`
5. Provide `License name` and choose `Cores` for `Licence Type`.  Leave the `Number of Cores` empty for default value and click `Submit`

# 2. Associate AMI for license
In order launch dedicated host instances using License Manager, we have to associate AMI with the license we created. Note that we must own the AMI so to work around this we can copy existing AMI.
1. Go to EC2 page
2. Click on `AMIs` on the sidebar
3. Filter by `Public Images` and select the AMI you want to copy
4. Click on `Actions` and `Copy AMI`, then rename the AMI if you want, and click `Copy AMI`
5. Go back to AWS License Manager page and go to `Self-Managed License`
6. Click on the license you generated in step 1 and click on `Associate AMI` on top right corner
7. Select the AMIs you want to associate then click `Associate`


# 3. Create host resource group
Host resource group manages the host associated with the server-bound licenses and this is the group we will call to help us access available dedicated hosts
1. Go to AWS License Manager
2. Click on `Host resource groups` on the sidebar
3. Click on `Create sost resource groups`
4. Provide `Host resource group name`, deselect `Release hosts automatically`, and use the license you created in step 1 for section `Associated self-managed licenses`
5. Click on `Create`

# 4. Associate dedicated hosts to host resource group
We will add the dedicated hosts we will use for testing to the host resource group
1. Go to the host resource group you created in step 3
2. Click on `Dedicated Hosts` tab
3. Click on `Add`
4. Select any dedicated host you want to use and click on `Add`
5. Use the ARN for this resource group in your terraform main.tf file. For example

```
resource "aws_instance" "cwagent" {
  ami                                  = data.aws_ami.latest.id
  instance_type                        = var.ec2_instance_type
  key_name                             = local.ssh_key_name
  iam_instance_profile                 = module.basic_components.instance_profile
  vpc_security_group_ids               = [module.basic_components.security_group]
  associate_public_ip_address          = true
  instance_initiated_shutdown_behavior = "terminate"
  tenancy                              = "host"
  host_resource_group_arn              = your_host_resourc_group_arn

  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  tags = {
    Name = "cwagent-integ-test-ec2-mac-${var.test_name}-${module.common.testing_id}"
  }
}
```
