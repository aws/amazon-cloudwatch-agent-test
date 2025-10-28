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

detect_system() {

    set +e
    rpmbin="$(which rpm 2>/dev/null)"
    found="$?"
    set -e
    if [ "${found}" -eq 0 ]; then
        # we have rpm binary, but was rpm used to install it?
	if rpm -q -f "${rpmbin}" >/dev/null 2>&1; then
	    echo 'rpm'
	    return 0
	fi
    fi

    set +e
    dpkgbin="$(which dpkg 2>/dev/null)"
    found="$?"
    set -e
    if [ "${found}" -eq 0 ]; then
        # we have dpkg binary, but was dpkg used to install it?
	if dpkg-query -S "${dpkgbin}" >/dev/null 2>&1; then
	    echo 'dpkg'
	    return 0
	fi
    fi

    echo 'unknown'
    return 0
}
