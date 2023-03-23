// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package rule

import (
	"github.com/aws/amazon-cloudwatch-agent-test/filesystem"
	"log"
)

type FilePermissionExpected struct {
	PermissionCompared filesystem.FilePermission
	ShouldExist        bool
}

var _ ICondition[string] = (*FilePermissionExpected)(nil)

func (e *FilePermissionExpected) GetName() string {
	return "FilePermissionExpected"
}

func (e *FilePermissionExpected) Evaluate(target string) (bool, error) {
	has, err := filesystem.FileHasPermission(target, e.PermissionCompared)
	if err != nil {
		return false, err
	}
	return e.ShouldExist == has, nil
}

type PermittedEntityMatch struct {
	ExpectedOwner *string
	ExpectedGroup *string
}

func (e *PermittedEntityMatch) GetName() string {
	return "PermittedEntityMatch"
}

var _ ICondition[string] = (*PermittedEntityMatch)(nil)

func (e *PermittedEntityMatch) Evaluate(target string) (bool, error) {
	if e.ExpectedOwner != nil {
		name, err := filesystem.GetFileOwnerUserName(target)
		log.Printf("FileOwnerUsername is: %v", name)
		if err != nil {
			return false, err
		} else if name != *e.ExpectedOwner {
			return false, nil
		}
	}
	if e.ExpectedGroup != nil {
		name, err := filesystem.GetFileGroupName(target)
		log.Printf("FileGroupName is: %v", name)
		if err != nil {
			return false, err
		} else if name != *e.ExpectedGroup {
			return false, nil
		}
	}
	return true, nil
}
