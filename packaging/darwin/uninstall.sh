#!/bin/bash

echo "Uninstalling Amazon-cloudwatch-agent"

# helper function to set error output
function error_exit
{
	echo "$1" 1>&2
	exit 1
}

echo "Checking if the agent is installed"
launchctl list com.amazon.cloudwatch.agent >/dev/null 2>&1
if [ $? -eq 0 ]; then
    echo "Agent is installed in this instance"
    echo "Uninstalling the agent"
    launchctl unload /Library/LaunchDaemons/com.amazon.cloudwatch.agent.plist
    echo "Agent stopped"

    rm -rf /opt/aws/amazon-cloudwatch-agent/THIRD-PARTY-LICENSES
    rm -rf /opt/aws/amazon-cloudwatch-agent/RELEASE_NOTES
    rm -rf /opt/aws/amazon-cloudwatch-agent/NOTICE
    rm -rf /opt/aws/amazon-cloudwatch-agent/LICENSE
    rm -rf /opt/aws/amazon-cloudwatch-agent/doc
    rm -rf /opt/aws/amazon-cloudwatch-agent/bin
    rm -rf /var/log/amazon/amazon-cloudwatch-agent
    rm /Library/LaunchDaemons/com.amazon.cloudwatch.agent.plist
else
    echo "Agent is not installed in this instance"
fi


pkgutil --pkg-info com.amazon.cloudwatch.agent >/dev/null 2>&1
if [ $? -eq 0 ]; then
    echo "Uninstall the Agent pkg"
    pkgutil --forget com.amazon.cloudwatch.agent >/dev/null 2>&1
    if [ $? -ne 0 ]; then
        echo "Uninstallation failed"
    fi
fi