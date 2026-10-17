#! /bin/bash
echo sha ${cwa_github_sha}
echo clone and install agent
cd /home/ec2-user/
git clone ${github_test_repo} amazon-cloudwatch-agent-test && git -C amazon-cloudwatch-agent-test checkout ${github_test_repo_branch}
cd amazon-cloudwatch-agent-test
aws s3 cp s3://${binary_uri} .
export HOME=/root
export GOPATH=$HOME/go
export PATH=$PATH:$GOPATH/bin
echo Running command ${install_agent}
${install_agent}
echo Starting agent now
/opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl -a fetch-config -m ec2 -s -c file:/home/ec2-user/amazon-cloudwatch-agent-test/test/userdata/agent_configs/config.json
cloud-init status --wait