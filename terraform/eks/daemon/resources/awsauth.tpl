mapRoles: |
  - groups:
    - system:bootstrappers
    - system:nodes
    rolearn: ${node_role_arn}
    username: system:node:{{EC2PrivateDNSName}}