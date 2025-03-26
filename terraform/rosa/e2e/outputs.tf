output "cluster_id" {
  value       = module.hcp.cluster_id
  description = "Unique identifier of the cluster."
}

output "cluster_name" {
  value = local.cluster_name
}

# Output the ARN of the CloudWatch Agent IAM role
output "cloudwatch_agent_role_arn" {
  value       = local.cloudwatch_agent_role_arn
  description = "ARN of the IAM role created for the CloudWatch agent"
}