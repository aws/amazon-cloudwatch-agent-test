// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
	"encoding/json"
	"net/http"

)
const(
	owner = "aws"
	repo ="amazon-cloudwatch-agent"
)

type GitTag struct {
	Name string `json:"ref"`
}

func getTags() ([]string, error) {
	// Construct the API URL for getting tags
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/git/refs/tags", owner, repo)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to fetch tags: %s", resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var tags []GitTag
	err = json.Unmarshal(body, &tags)
	if err != nil {
		return nil, err
	}

	var tagIds []string
	for _, tag := range tags {
		tagName := strings.Split(tag.Name,"/v")
		// Remove the 'v' prefix
		tagIds = append(tagIds, tagName[len(tagName)-1])
	}

	return tagIds, nil
}
// Converts agent version to MSI-compatible version.
func agentVersionToMsi(agentVersion string) string {
	split := strings.Split(agentVersion, ".")
	if len(split) < 3 {
		log.Fatalf("Invalid agent version format: %s", agentVersion)
	}

	major := split[0]
	minor, err := strconv.ParseInt(split[1], 10, 64)
	if err != nil {
		log.Fatalf("Failed to parse agentVersion %v", err)
	}
	minor = minor / 65536
	patch, err := strconv.ParseInt(split[1], 10, 64)
	if err != nil {
		log.Fatalf("Failed to parse agentVersion %v", err)
	}
	patch = patch % 65536
	msiVersion := major + "." + strconv.FormatInt(minor, 10) + "." + strconv.FormatInt(patch, 10)
	return msiVersion
}

// Converts MSI-compatible version back to the original agent version.
func msiToAgentVersion(msiVersion string) string {
	allTags, _:=getTags()

	foundMsiVersion := "NOT FOUND"
	for _ , tag := range allTags{
		foundMsiVersion = agentVersionToMsi(tag)
		if foundMsiVersion == msiVersion{
			return tag
		}

	}
	return foundMsiVersion
}

// Replaces a key in a file with a given value.
func replaceValue(filePath, key, value string) {
	fmt.Printf("Replacing %s with %s in %s file\n", key,value, filePath)
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		log.Fatalf("Failed to read file: %v", err)
	}

	newContent := bytes.Replace(content, []byte(key), []byte(value), -1)

	err = ioutil.WriteFile(filePath, newContent, 0644)
	if err != nil {
		log.Fatalf("Failed to write file: %v", err)
	}
}

func main() {
	// Define flags
	reverseFlag := flag.Bool("reverse", false, "Convert MSI version back to agent version")
	flag.Parse()

	// Get positional arguments
	args := flag.Args()

	if len(args) < 1 {
		log.Fatalf("Usage: %s <agentVersion/MSIVersion> [replaceFilePath] [msiVersionKey] [--reverse]", os.Args[0])
	}

	version := args[0]
	var replaceFilePath, msiVersionKey string

	if len(args) > 1 {
		replaceFilePath = args[1]
	}
	if len(args) > 2 {
		msiVersionKey = args[2]
	}

	// Check if we are reversing the conversion
	var targetVersion string
	if *reverseFlag {
		agentVersion := msiToAgentVersion(version)
		fmt.Println(agentVersion)
		targetVersion = agentVersion

	} else {
		msiVersion := agentVersionToMsi(version)
		fmt.Println(msiVersion)
		targetVersion = msiVersion
	}

	if replaceFilePath != "" && msiVersionKey != "" {
		replaceValue(replaceFilePath, msiVersionKey, targetVersion)
	}
}
