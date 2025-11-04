# How to Set Up EKS Cluster with GPU Metrics

## Overview

The test creates a **mock GPU environment** without needing actual GPU hardware. It uses a dummy DCGM exporter (httpd container) that simulates GPU metrics.

## Key Components

1. **EKS Cluster** - Standard EKS cluster (no GPU instances needed!)
2. **Mock DCGM Exporter** - httpd container that serves fake GPU metrics
3. **CloudWatch Agent** - Scrapes metrics from the mock exporter
4. **TLS Certificates** - For secure communication between agent and exporter

## Step-by-Step Setup

### 1. Create EKS Cluster

```bash
cd terraform/eks/daemon/gpu

# Initialize terraform
terraform init

# Create the cluster
terraform apply \
  -var="region=us-west-2" \
  -var="k8s_version=1.28" \
  -var="instance_type=t3.medium" \
  -var="cwagent_image_repo=public.ecr.aws/cloudwatch-agent/cloudwatch-agent" \
  -var="cwagent_image_tag=latest" \
  -var="test_dir=./test/gpu"
```

### 2. Configure kubectl

```bash
aws eks update-kubeconfig --name cwagent-eks-integ-<testing-id> --region us-west-2
```

### 3. Verify Components

```bash
# Check namespace
kubectl get ns amazon-cloudwatch

# Check DCGM exporter (mock GPU metrics)
kubectl get daemonset dcgm-exporter -n amazon-cloudwatch
kubectl get svc dcgm-exporter-service -n amazon-cloudwatch

# Check CloudWatch agent
kubectl get daemonset cloudwatch-agent -n amazon-cloudwatch

# Check if metrics endpoint is working
kubectl port-forward -n amazon-cloudwatch svc/dcgm-exporter-service 9400:9400
# In another terminal:
curl http://localhost:9400/metrics
```

## CloudWatch Agent Configuration for GPU

The agent config should include:

```json
{
  "agent": {
    "metrics_collection_interval": 60,
    "run_as_user": "root",
    "debug": true
  },
  "logs": {
    "metrics_collected": {
      "kubernetes": {
        "enhanced_container_insights": true,
        "accelerated_compute_metrics": true
      }
    },
    "force_flush_interval": 5
  }
}
```

## How the Mock GPU Metrics Work

The test uses an **httpd container** that serves Prometheus-format metrics:

```
DCGM_FI_DEV_GPU_UTIL{gpu="0",UUID="uuid0",device="nvidia0",modelName="Tesla T4"} 1
DCGM_FI_DEV_FB_FREE{gpu="0",UUID="uuid0",device="nvidia0",modelName="Tesla T4"} 1
DCGM_FI_DEV_FB_USED{gpu="0",UUID="uuid0",device="nvidia0",modelName="Tesla T4"} 1
DCGM_FI_DEV_FB_TOTAL{gpu="0",UUID="uuid0",device="nvidia0",modelName="Tesla T4"} 1
DCGM_FI_DEV_GPU_TEMP{gpu="0",UUID="uuid0",device="nvidia0",modelName="Tesla T4"} 1
DCGM_FI_DEV_POWER_USAGE{gpu="0",UUID="uuid0",device="nvidia0",modelName="Tesla T4"} 1
```

These metrics are exposed at `https://dcgm-exporter-service.amazon-cloudwatch.svc:9400/metrics`

## Expected Metrics in CloudWatch

After the agent scrapes the metrics, you should see in CloudWatch:

**Namespace:** `ContainerInsights`

**Metrics:**
- `container_gpu_utilization`
- `container_gpu_memory_used`
- `container_gpu_memory_total`
- `container_gpu_temperature`
- `container_gpu_power_draw`
- `pod_gpu_utilization`
- `pod_gpu_memory_used`
- `node_gpu_utilization`
- `node_gpu_memory_used`

**Dimensions:**
- `ClusterName`
- `ClusterName-Namespace-PodName`
- `ClusterName-Namespace-PodName-ContainerName`
- `ClusterName-NodeName`

## Manual Setup (Without Terraform)

If you want to set this up manually:

### 1. Create Namespace

```bash
kubectl create namespace amazon-cloudwatch
```

### 2. Create TLS Certificates

```bash
# Generate CA key and cert
openssl genrsa -out ca.key 2048
openssl req -x509 -new -nodes -key ca.key -sha256 -days 1 \
  -out ca.crt -subj "/CN=dcgm-exporter-service.amazon-cloudwatch.svc/O=Amazon CloudWatch Agent"

# Generate server key and cert
openssl genrsa -out server.key 2048
openssl req -new -key server.key -out server.csr \
  -subj "/CN=dcgm-exporter-service.amazon-cloudwatch.svc/O=Amazon CloudWatch Agent"
openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out server.crt -days 1 -sha256

# Create secret
kubectl create secret generic amazon-cloudwatch-observability-agent-cert \
  -n amazon-cloudwatch \
  --from-file=ca.crt=ca.crt \
  --from-file=tls.crt=server.crt \
  --from-file=tls.key=server.key
```

### 3. Deploy Mock DCGM Exporter

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: dcgm-metrics
  namespace: amazon-cloudwatch
data:
  metrics: |
    DCGM_FI_DEV_GPU_UTIL{gpu="0",UUID="uuid0",device="nvidia0",modelName="Tesla T4"} 75
    DCGM_FI_DEV_FB_FREE{gpu="0",UUID="uuid0",device="nvidia0",modelName="Tesla T4"} 8192
    DCGM_FI_DEV_FB_USED{gpu="0",UUID="uuid0",device="nvidia0",modelName="Tesla T4"} 7808
    DCGM_FI_DEV_FB_TOTAL{gpu="0",UUID="uuid0",device="nvidia0",modelName="Tesla T4"} 16000
    DCGM_FI_DEV_GPU_TEMP{gpu="0",UUID="uuid0",device="nvidia0",modelName="Tesla T4"} 65
    DCGM_FI_DEV_POWER_USAGE{gpu="0",UUID="uuid0",device="nvidia0",modelName="Tesla T4"} 120
---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: dcgm-exporter
  namespace: amazon-cloudwatch
spec:
  selector:
    matchLabels:
      k8s-app: dcgm-exporter
  template:
    metadata:
      labels:
        k8s-app: dcgm-exporter
    spec:
      containers:
      - name: dcgm-exporter
        image: httpd:2.4-alpine
        ports:
        - containerPort: 9400
          name: metrics
        command: ["/bin/sh", "-c"]
        args:
        - |
          cat > /usr/local/apache2/htdocs/metrics << 'EOF'
          DCGM_FI_DEV_GPU_UTIL{gpu="0",UUID="uuid0",device="nvidia0",modelName="Tesla T4"} 75
          DCGM_FI_DEV_FB_FREE{gpu="0",UUID="uuid0",device="nvidia0",modelName="Tesla T4"} 8192
          DCGM_FI_DEV_FB_USED{gpu="0",UUID="uuid0",device="nvidia0",modelName="Tesla T4"} 7808
          DCGM_FI_DEV_FB_TOTAL{gpu="0",UUID="uuid0",device="nvidia0",modelName="Tesla T4"} 16000
          DCGM_FI_DEV_GPU_TEMP{gpu="0",UUID="uuid0",device="nvidia0",modelName="Tesla T4"} 65
          DCGM_FI_DEV_POWER_USAGE{gpu="0",UUID="uuid0",device="nvidia0",modelName="Tesla T4"} 120
          EOF
          httpd-foreground
---
apiVersion: v1
kind: Service
metadata:
  name: dcgm-exporter-service
  namespace: amazon-cloudwatch
spec:
  selector:
    k8s-app: dcgm-exporter
  ports:
  - port: 9400
    targetPort: 9400
    name: metrics
```

### 4. Deploy CloudWatch Agent

Use the standard CloudWatch agent DaemonSet with the GPU-enabled config above.

## Verify Metrics

```bash
# Check agent logs
kubectl logs -n amazon-cloudwatch -l name=cloudwatch-agent --tail=100

# Check if metrics are being scraped
kubectl exec -n amazon-cloudwatch -it <cloudwatch-agent-pod> -- \
  curl http://dcgm-exporter-service:9400/metrics

# Check CloudWatch (after a few minutes)
aws cloudwatch list-metrics \
  --namespace ContainerInsights \
  --dimensions Name=ClusterName,Value=<your-cluster-name> \
  | grep gpu
```

## Key Differences from Real GPU Setup

**Mock Setup (Test):**
- No GPU hardware needed
- Uses httpd to serve fake metrics
- Metrics are static values
- Works on any instance type

**Real GPU Setup:**
- Requires GPU instances (g4dn, p3, etc.)
- Uses actual NVIDIA DCGM exporter
- Requires NVIDIA device plugin
- Metrics reflect real GPU usage

## Cleanup

```bash
cd terraform/eks/daemon/gpu
terraform destroy
```

## Summary

The test creates a **complete mock GPU environment** that:
1. Simulates DCGM exporter metrics without real GPUs
2. Uses TLS for secure communication
3. Allows testing GPU metric collection on any instance type
4. Validates the CloudWatch agent's GPU metric scraping functionality

This is perfect for development and testing without expensive GPU instances!
