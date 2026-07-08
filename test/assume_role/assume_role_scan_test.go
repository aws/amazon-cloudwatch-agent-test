// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package assume_role

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/aws/amazon-cloudwatch-agent-test/environment"
)

const (
	testAccountID   = "506463145083"
	testInstanceArn = "arn:aws:ec2:us-west-2:506463145083:instance/i-079ea258c33f3158f"
)

// v2Log mirrors the aws-sdk-go-v2 / smithy RequestResponseLogger output, which emits
// "D! Request\n" followed by httputil.DumpRequestOut and has no trailing marker.
const v2Log = `2026/07/03 18:05:10 D! Request
POST / HTTP/1.1
Host: sts.us-west-2.amazonaws.com
User-Agent: aws-sdk-go-v2/1.41.5 os/linux lang/go
Content-Length: 199
Authorization: AWS4-HMAC-SHA256 Credential=AKIA/20260703/us-west-2/sts/aws4_request, SignedHeaders=content-length;content-type;host;x-amz-date;x-amz-security-token;x-amz-source-account;x-amz-source-arn, Signature=abc
Content-Type: application/x-www-form-urlencoded; charset=utf-8
X-Amz-Date: 20260703T180510Z
X-Amz-Security-Token: token
X-Amz-Source-Account: 506463145083
X-Amz-Source-Arn: arn:aws:ec2:us-west-2:506463145083:instance/i-079ea258c33f3158f
Accept-Encoding: gzip

Action=AssumeRole&DurationSeconds=900&RoleArn=arn%3Aaws%3Aiam%3A%3A506463145083%3Arole%2Fcwa-integ-assume-role-all_context_keys&RoleSessionName=123&Version=2011-06-15
2026/07/03 18:05:10 D! Response
HTTP/1.1 200 OK
`

// v1Log mirrors the legacy aws-sdk-go v1 wire log with POST-SIGN start and dashed end markers.
const v1Log = `2026/07/03 18:05:10 I! ---[ REQUEST POST-SIGN ]-----------------------------
POST / HTTP/1.1
Host: sts.us-west-2.amazonaws.com
X-Amz-Source-Account: 506463145083
X-Amz-Source-Arn: arn:aws:ec2:us-west-2:506463145083:instance/i-079ea258c33f3158f

Action=AssumeRole&Version=2011-06-15
-----------------------------------------------------
`

func TestScanForConfusedDeputyHeaders(t *testing.T) {
	metadata = &environment.MetaData{
		AccountId:   testAccountID,
		InstanceArn: testInstanceArn,
	}

	cases := []struct {
		name string
		log  string
		want bool
	}{
		{name: "sdk v2 format", log: v2Log, want: true},
		{name: "sdk v1 format", log: v1Log, want: true},
		{
			name: "assume role request missing headers",
			log: `2026/07/03 18:05:10 D! Request
POST / HTTP/1.1
Host: sts.us-west-2.amazonaws.com

Action=AssumeRole&Version=2011-06-15
`,
			want: false,
		},
		{
			name: "headers present but not an assume role request",
			log: `2026/07/03 18:05:10 D! Request
POST / HTTP/1.1
Host: monitoring.us-west-2.amazonaws.com
X-Amz-Source-Account: 506463145083
X-Amz-Source-Arn: arn:aws:ec2:us-west-2:506463145083:instance/i-079ea258c33f3158f

Action=PutMetricData&Version=2010-08-01
`,
			want: false,
		},
		{
			name: "wrong account and arn",
			log: `2026/07/03 18:05:10 D! Request
POST / HTTP/1.1
X-Amz-Source-Account: 123456789012
X-Amz-Source-Arn: arn:aws:ec2:us-west-2:123456789012:instance/i-1234567890abcdef0

Action=AssumeRole&Version=2011-06-15
`,
			want: false,
		},
		{name: "no debug logs", log: "2026/07/03 18:05:10 I! Agent started\n", want: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, scanForConfusedDeputyHeaders(strings.NewReader(tc.log)))
		})
	}
}

func TestBlockStartDetection(t *testing.T) {
	assert.True(t, isRequestBlockStart("2026/07/03 18:05:10 D! Request"))
	assert.True(t, isRequestBlockStart("... ---[ REQUEST POST-SIGN ]-----------------------------"))
	assert.False(t, isRequestBlockStart("2026/07/03 18:05:10 D! Response"))
	assert.False(t, isRequestBlockStart("Action=AssumeRole&Version=2011-06-15"))

	assert.True(t, isHTTPDebugBlockStart("2026/07/03 18:05:10 D! Response"))
	assert.True(t, isHTTPDebugBlockStart("2026/07/03 18:05:10 D! Request"))
	assert.False(t, isHTTPDebugBlockStart("X-Amz-Source-Account: 506463145083"))
}
