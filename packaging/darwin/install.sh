#!/bin/bash

echo "Installing Amazon-cloudwatch-agent"

# helper function to set error output
function error_exit() {
  echo "$1" 1>&2
  exit 1
}
echo "Checking if the agent is installed"
launchctl list com.amazon.cloudwatch.agent >/dev/null 2>&1

if [ $? -eq 0 ]; then
  echo "Agent is running in the instance"
  echo "Stopping the agent"
  launchctl unload /Library/LaunchDaemons/com.amazon.cloudwatch.agent.plist
  echo "Agent stopped"
else
  echo "Agent is not running in the instance"
fi

echo "Installing agent"
installer -pkg amazon-cloudwatch-agent.pkg -target /
