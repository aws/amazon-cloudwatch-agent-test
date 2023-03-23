// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

//go:build !windows

package filesystem

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os/user"
)

type FilePermission string

const (
	OwnerWrite  FilePermission = "OwnerWrite"
	GroupWrite  FilePermission = "GroupWrite"
	AnyoneWrite FilePermission = "AnyoneWrite"
	OwnerRead   FilePermission = "OwnerRead"
	AnyoneRead  FilePermission = "AnyoneRead"
)

var FilePermissionInHex = map[FilePermission]uint32{
	OwnerWrite:  unix.S_IWUSR,
	GroupWrite:  unix.S_IWGRP,
	AnyoneWrite: unix.S_IWOTH,
	OwnerRead:   unix.S_IRUSR,
	AnyoneRead:  unix.S_IROTH,
}

func FileHasPermission(filePath string, permission FilePermission) (bool, error) {
	fileStat, ok := GetFileStatPermission(filePath)
	if ok != nil {
		return false, ok
	}

	hasPermission := fileStat&FilePermissionInHex[permission] != 0
	return hasPermission, nil
}

func GetFileStatPermission(filePath string) (uint32, error) {
	var stat unix.Stat_t
	if err := unix.Stat(filePath, &stat); err != nil {
		return 0, fmt.Errorf("cannot get file's stat %s: %v", filePath, err)
	}
	return uint32(stat.Mode), nil
}

func GetFileOwnerUserName(filePath string) (string, error) {
	var stat unix.Stat_t
	if err := unix.Stat(filePath, &stat); err != nil {
		return "", fmt.Errorf("cannot get file's stat %s: %v", filePath, err)
	}
	owner, err := user.LookupId(fmt.Sprintf("%d", stat.Uid))
	if err != nil {
		return "", fmt.Errorf("cannot look up file owner's name %s: %v", filePath, err)
	}
	return owner.Username, nil

}

func GetFileGroupName(filePath string) (string, error) {
	var stat unix.Stat_t
	if err := unix.Stat(filePath, &stat); err != nil {
		return "", fmt.Errorf("cannot get file's stat %s: %v", filePath, err)
	}
	if grp, err := user.LookupGroupId(fmt.Sprintf("%d", stat.Gid)); err != nil {
		return "", fmt.Errorf("cannot look up file group name %s: %v", filePath, err)
	} else {
		return grp.Name, nil
	}
}

// CheckFileRights check that the given file path has been protected by the owner.
// If the owner is changed, they need at least the sudo permission to override the owner.
func CheckFileRights(filePath string) error {
	var stat unix.Stat_t
	if err := unix.Stat(filePath, &stat); err != nil {
		return fmt.Errorf("cannot get file's stat %s: %v", filePath, err)
	}

	// Check the owner of binary has read, write, exec.
	if !(stat.Mode&(unix.S_IXUSR) == 0 || stat.Mode&(unix.S_IRUSR) == 0 || stat.Mode&(unix.S_IWUSR) == 0) {
		return nil
	}

	// Check the owner of file has read, write
	if !(stat.Mode&(unix.S_IRUSR) == 0 || stat.Mode&(unix.S_IWUSR) == 0) {
		return nil
	}

	return fmt.Errorf("file's owner does not have enough permission at path %s", filePath)
}

// CheckFileOwnerRights check that the given owner is the same owner of the given filepath
func CheckFileOwnerRights(filePath, requiredOwner string) error {
	ownerUsername, err := GetFileOwnerUserName(filePath)

	if err != nil {
		return fmt.Errorf("cannot look up file owner's name %s: %v", filePath, err)
	} else if ownerUsername != requiredOwner {
		return fmt.Errorf("owner does not have permission to protect file %s", filePath)
	}
	return nil
}
