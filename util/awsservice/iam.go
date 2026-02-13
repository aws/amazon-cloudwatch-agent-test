// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/ec2/imds"
	"github.com/aws/aws-sdk-go-v2/service/iam"
)

// PutRoleDenyPolicy creates an inline deny policy on a role for logs:PutLogEvents
// and logs:CreateLogStream on the given log group ARN pattern.
func PutRoleDenyPolicy(roleName, policyName, logGroupPattern string) error {
	policy := map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []map[string]interface{}{
			{
				"Effect":   "Deny",
				"Action":   []string{"logs:PutLogEvents", "logs:CreateLogStream"},
				"Resource": logGroupPattern,
			},
		},
	}
	policyJSON, err := json.Marshal(policy)
	if err != nil {
		return fmt.Errorf("failed to marshal policy: %w", err)
	}

	_, err = IamClient.PutRolePolicy(ctx, &iam.PutRolePolicyInput{
		RoleName:       aws.String(roleName),
		PolicyName:     aws.String(policyName),
		PolicyDocument: aws.String(string(policyJSON)),
	})
	if err != nil {
		return fmt.Errorf("failed to put role policy: %w", err)
	}
	return nil
}

// DeleteRoleInlinePolicy deletes an inline policy from a role.
func DeleteRoleInlinePolicy(roleName, policyName string) error {
	_, err := IamClient.DeleteRolePolicy(ctx, &iam.DeleteRolePolicyInput{
		RoleName:   aws.String(roleName),
		PolicyName: aws.String(policyName),
	})
	if err != nil {
		return fmt.Errorf("failed to delete role policy: %w", err)
	}
	return nil
}

// GetInstanceRoleName returns the IAM role name attached to this EC2 instance.
func GetInstanceRoleName() (string, error) {
	resp, err := ImdsClient.GetMetadata(ctx, &imds.GetMetadataInput{
		Path: "iam/security-credentials/",
	})
	if err != nil {
		return "", fmt.Errorf("failed to get role from IMDS: %w", err)
	}
	defer resp.Content.Close()

	content, err := io.ReadAll(resp.Content)
	if err != nil {
		return "", fmt.Errorf("failed to read IMDS response: %w", err)
	}

	roleName := strings.TrimSpace(string(content))
	if roleName == "" {
		return "", fmt.Errorf("no IAM role attached to instance")
	}
	return roleName, nil
}
