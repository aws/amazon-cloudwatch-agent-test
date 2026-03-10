#!/bin/sh

# Read from ~/.aws/credentials [default] profile if env vars not set
if [ -f /root/.aws/credentials ]; then
  AWS_ACCESS_KEY_ID=${AWS_ACCESS_KEY_ID:-$(sed -n '/\[default\]/,/\[/p' /root/.aws/credentials | grep aws_access_key_id | head -1 | sed 's/[^=]*= *//')}
  AWS_SECRET_ACCESS_KEY=${AWS_SECRET_ACCESS_KEY:-$(sed -n '/\[default\]/,/\[/p' /root/.aws/credentials | grep aws_secret_access_key | head -1 | sed 's/[^=]*= *//')}
  AWS_SESSION_TOKEN=${AWS_SESSION_TOKEN:-$(sed -n '/\[default\]/,/\[/p' /root/.aws/credentials | grep aws_session_token | head -1 | sed 's/[^=]*= *//')}
fi

# Fail fast if required credentials are missing
if [ -z "$AWS_ACCESS_KEY_ID" ] || [ -z "$AWS_SECRET_ACCESS_KEY" ]; then
  echo "ERROR: No AWS credentials found for Mock IMDS endpoint." >&2
  echo "Provide credentials via environment variables (AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY) or ~/.aws/credentials file." >&2
  exit 1
fi

# Session token is optional (not needed for long-term IAM user keys)
AWS_SESSION_TOKEN=${AWS_SESSION_TOKEN:-}

cat > /root/aemm-config.json <<EOF
{
  "metadata": {
    "values": {
      "instance-id": "${IMDS_INSTANCE_ID:-i-1234567890abcdef0}",
      "instance-type": "${IMDS_INSTANCE_TYPE:-m5.xlarge}",
      "placement-region": "${IMDS_REGION:-us-east-1}",
      "placement-availability-zone": "${IMDS_AVAILABILITY_ZONE:-us-east-1a}",
      "iam-security-credentials": {
        "Code": "Success",
        "LastUpdated": "2020-04-02T18:50:40Z",
        "Type": "AWS-HMAC",
        "AccessKeyId": "${AWS_ACCESS_KEY_ID}",
        "SecretAccessKey": "${AWS_SECRET_ACCESS_KEY}",
        "Token": "${AWS_SESSION_TOKEN}",
        "Expiration": "2099-12-31T23:59:59Z"
      }
    }
  }
}
EOF
exec /ec2-metadata-mock "$@"
