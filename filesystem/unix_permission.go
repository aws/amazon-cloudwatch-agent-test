// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package filesystem

import (
	"fmt"
	"os/user"
	"syscall"
)

// whose permission should exist (for this file's permission) = condition (... file permission... Owner is root, Owner Can Read, Non Owner Cannot read
// condition = string condition.? or OwnerIsRoot(uint) or OwnerOfFileIsRoot(filename).
// whose permission should not exist
// There are all rules.

// somewhere else, get permission Mode
// create rule: root is owner (condition. bitwise evaluator? or filename. filename!

// OwnerRoot confition => call CheckFileOwnerRights
// OwnerReadPermission => get Mode() => wrapper function like OwnerRead..
//

// CheckFileRights check that the given file path has been protected by the owner.
// If the owner is changed, they need at least the sudo permission to override the owner.

type FilePermission string

const (
	OwnerWrite  FilePermission = "OwnerWrite"
	GroupWrite  FilePermission = "GroupWrite"
	AnyoneWrite FilePermission = "AnyoneWrite"
	OwnerRead   FilePermission = "OwnerRead"
	AnyoneRead  FilePermission = "AnyoneRead"
)

var FilePermissionInHex = map[FilePermission]uint32{
	OwnerWrite:  syscall.S_IWUSR,
	GroupWrite:  syscall.S_IWGRP,
	AnyoneWrite: syscall.S_IWOTH,
	OwnerRead:   syscall.S_IRUSR,
	AnyoneRead:  syscall.S_IROTH,
}

func FileHasPermission(filePath string, permission FilePermission) (bool, error) {
	fileStat, ok := GetFileStatPermission(filePath)
	if ok != nil {
		return false, ok
	}

	return IsInclude(fileStat, FilePermissionInHex[permission]), nil
}

func GetFileStatPermission(filePath string) (uint32, error) {
	var stat syscall.Stat_t
	if err := syscall.Stat(filePath, &stat); err != nil {
		return 0, fmt.Errorf("Cannot get file's stat %s: %v", filePath, err)
	}

	return stat.Mode, nil
}

func IsInclude(included uint32, include uint32) bool {
	return included&include != 0
}

func GetFileOwnerUserName(filePath string) (string, error) {
	var stat syscall.Stat_t
	if err := syscall.Stat(filePath, &stat); err != nil {
		return "", fmt.Errorf("cannot get file's stat %s: %v", filePath, err)
	}
	if owner, err := user.LookupId(fmt.Sprintf("%d", stat.Uid)); err != nil {
		return "", fmt.Errorf("cannot look up file owner's name %s: %v", filePath, err)
	} else {
		return owner.Username, nil
	}
}

func GetFileGroupName(filePath string) (string, error) {
	var stat syscall.Stat_t
	if err := syscall.Stat(filePath, &stat); err != nil {
		return "", fmt.Errorf("cannot get file's stat %s: %v", filePath, err)
	}
	if grp, err := user.LookupGroupId(fmt.Sprintf("%d", stat.Gid)); err != nil {
		return "", fmt.Errorf("cannot look up file group name %s: %v", filePath, err)
	} else {
		return grp.Name, nil
	}
}

func CheckFileRights(filePath string) error {
	var stat syscall.Stat_t
	if err := syscall.Stat(filePath, &stat); err != nil {
		return fmt.Errorf("Cannot get file's stat %s: %v", filePath, err)
	}

	// Check the owner of binary has read, write, exec.
	if !(stat.Mode&(syscall.S_IXUSR) == 0 || stat.Mode&(syscall.S_IRUSR) == 0 || stat.Mode&(syscall.S_IWUSR) == 0) {
		return nil
	}

	// Check the owner of file has read, write
	if !(stat.Mode&(syscall.S_IRUSR) == 0 || stat.Mode&(syscall.S_IWUSR) == 0) {
		return nil
	}

	return fmt.Errorf("File's owner does not have enough permission at path %s", filePath)
}

// CheckFileOwnerRights check that the given owner is the same owner of the given filepath
func CheckFileOwnerRights(filePath, requiredOwner string) error {
	ownerUsername, err := GetFileOwnerUserName(filePath)

	if err != nil {
		return fmt.Errorf("Cannot look up file owner's name %s: %v", filePath, err)
	} else if ownerUsername != requiredOwner {
		return fmt.Errorf("Owner does not have permission to protect file %s", filePath)
	}
	return nil
}
