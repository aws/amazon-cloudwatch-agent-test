// Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

const metricsTemplate = `aws_ec2_instance_store_csi_ec2_exceeded_iops_seconds_total{instance_id="ip-192-168-31-5.us-west-2.compute.internal",volume_id="/dev/nvme1n1"} 0
aws_ec2_instance_store_csi_ec2_exceeded_iops_seconds_total{instance_id="ip-192-168-31-5.us-west-2.compute.internal",volume_id="/dev/nvme2n1"} 0
aws_ec2_instance_store_csi_ec2_exceeded_tp_seconds_total{instance_id="ip-192-168-31-5.us-west-2.compute.internal",volume_id="/dev/nvme1n1"} 0
aws_ec2_instance_store_csi_ec2_exceeded_tp_seconds_total{instance_id="ip-192-168-31-5.us-west-2.compute.internal",volume_id="/dev/nvme2n1"} 0
aws_ec2_instance_store_csi_nvme_collector_errors_total{instance_id="ip-192-168-31-5.us-west-2.compute.internal"} 0
aws_ec2_instance_store_csi_nvme_collector_scrapes_total{instance_id="ip-192-168-31-5.us-west-2.compute.internal"} 845
aws_ec2_instance_store_csi_read_bytes_total{instance_id="ip-192-168-31-5.us-west-2.compute.internal",volume_id="/dev/nvme1n1"} 1.8214912e+09
aws_ec2_instance_store_csi_read_bytes_total{instance_id="ip-192-168-31-5.us-west-2.compute.internal",volume_id="/dev/nvme2n1"} 1.8214912e+09
aws_ec2_instance_store_csi_read_ops_total{instance_id="ip-192-168-31-5.us-west-2.compute.internal",volume_id="/dev/nvme1n1"} 789681
aws_ec2_instance_store_csi_read_ops_total{instance_id="ip-192-168-31-5.us-west-2.compute.internal",volume_id="/dev/nvme2n1"} 789681
aws_ec2_instance_store_csi_read_seconds_total{instance_id="ip-192-168-31-5.us-west-2.compute.internal",volume_id="/dev/nvme1n1"} 20.537499
aws_ec2_instance_store_csi_read_seconds_total{instance_id="ip-192-168-31-5.us-west-2.compute.internal",volume_id="/dev/nvme2n1"} 20.522662
aws_ec2_instance_store_csi_volume_queue_length{instance_id="ip-192-168-31-5.us-west-2.compute.internal",volume_id="/dev/nvme1n1"} 1
aws_ec2_instance_store_csi_volume_queue_length{instance_id="ip-192-168-31-5.us-west-2.compute.internal",volume_id="/dev/nvme2n1"} 1
aws_ec2_instance_store_csi_write_bytes_total{instance_id="ip-192-168-31-5.us-west-2.compute.internal",volume_id="/dev/nvme1n1"} 0
aws_ec2_instance_store_csi_write_bytes_total{instance_id="ip-192-168-31-5.us-west-2.compute.internal",volume_id="/dev/nvme2n1"} 0
aws_ec2_instance_store_csi_write_ops_total{instance_id="ip-192-168-31-5.us-west-2.compute.internal",volume_id="/dev/nvme1n1"} 0
aws_ec2_instance_store_csi_write_ops_total{instance_id="ip-192-168-31-5.us-west-2.compute.internal",volume_id="/dev/nvme2n1"} 0
aws_ec2_instance_store_csi_write_seconds_total{instance_id="ip-192-168-31-5.us-west-2.compute.internal",volume_id="/dev/nvme1n1"} 0
aws_ec2_instance_store_csi_write_seconds_total{instance_id="ip-192-168-31-5.us-west-2.compute.internal",volume_id="/dev/nvme2n1"} 0
`

func metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprint(w, metricsTemplate)
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8101"
	}

	http.HandleFunc("/metrics", metricsHandler)
	log.Printf("Mock LIS CSI server starting on port %s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
