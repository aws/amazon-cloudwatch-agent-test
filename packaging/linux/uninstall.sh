#!/bin/sh

# Copyright 2017 Amazon.com, Inc. and its affiliates. All Rights Reserved.
#
# Licensed under the Amazon Software License (the "License").
# You may not use this file except in compliance with the License.
# A copy of the License is located at
#
#   http://aws.amazon.com/asl/
#
# or in the "license" file accompanying this file. This file is distributed
# on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
# express or implied. See the License for the specific language governing
# permissions and limitations under the License.

set -e
set -u

. ./detect-system.sh

readonly cmd='/opt/aws/amazon-cloudwatch-agent/bin/amazon-cloudwatch-agent-ctl'
if [ -x "${cmd}" ]; then
    "${cmd}" -a prep-restart
fi

case "$(detect_system)" in
    rpm) rpm -e amazon-cloudwatch-agent ;;
    dpkg) dpkg -r amazon-cloudwatch-agent ;;
    *) echo "Error: Unable to determine package management system." >&2
       exit 1
       ;;
esac
