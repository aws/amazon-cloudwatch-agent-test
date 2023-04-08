// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package awsservice

import (
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)


func DownloadFile(bucket, key, outFilename string) error {
	file, err := os.Create(outFilename)
	if err != nil {
		log.Printf("error: creating file %s err %v", outFilename, err)
		return err
	}
	defer file.Close()
	s3GetObjectInput := s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}
	downloader := manager.NewDownloader(S3Client)
	_, err = downloader.Download(ctx, file, &s3GetObjectInput)
	if err != nil {
		log.Printf("error: downloading, %v", err)
	}
	return err
}