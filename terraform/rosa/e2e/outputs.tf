output "cluster_id" {
  value       = module.hcp.cluster_id
  description = "Unique identifier of the cluster."
}

output "cluster_name" {
  value = local.cluster_name
}
