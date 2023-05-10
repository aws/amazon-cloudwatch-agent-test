#! /bin/bash
echo sha ${cwa_github_sha}
echo clone and install agent
cd /home/ec2-user/
git clone --branch ${github_test_repo_branch} ${github_test_repo}
cd amazon-cloudwatch-agent-test
aws s3 cp s3://${binary_uri} .
export HOME=/root
export GOPATH=$HOME/go
export PATH=$PATH:$GOPATH/bin
rpm -U ./amazon-cloudwatch-agent.rpm
cloud-init status --wait