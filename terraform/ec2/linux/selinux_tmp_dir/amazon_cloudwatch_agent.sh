#!/bin/sh -e

DIRNAME=`dirname $0`
cd $DIRNAME
USAGE="$0 [ --update ]"
if [ `id -u` != 0 ]; then
    echo 'You must be root to run this script'
    exit 1
fi

echo "Building and Loading Policy"
set -x
make -f /usr/share/selinux/devel/Makefile amazon_cloudwatch_agent.pp || exit
/usr/sbin/semodule -i amazon_cloudwatch_agent.pp

# Fixing the file context on CloudWatch agent files
/sbin/restorecon -R -v /opt/aws/amazon-cloudwatch-agent || true
/sbin/restorecon -v /usr/lib/systemd/system/amazon-cloudwatch-agent.service || true

echo "Policy loaded. You may need to restart the CloudWatch agent: systemctl restart amazon-cloudwatch-agent"