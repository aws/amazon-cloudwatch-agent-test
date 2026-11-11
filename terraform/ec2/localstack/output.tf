output "public_dns" {
  description = "The public DNS name assigned to the instance. For EC2-VPC, this is only available if you've enabled DNS hostnames for your VPC"
  value       = aws_instance.integration-test.public_dns
}
// The AMI this run launched, so the run log shows which image and owner account were used.
output "ami_id" {
  value = data.aws_ami.latest.id
}

output "ami_owner_id" {
  value = data.aws_ami.latest.owner_id
}
