// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
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
