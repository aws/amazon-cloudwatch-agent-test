for times in  {1..6}
  do
    sleep 5
	  CURRENT_TIME=$(date +%s%N | cut -b1-13)
	  echo '{"_aws":{"Timestamp":'"${CURRENT_TIME}"',"LogGroupName":"MetricValueBenchmarkTest","CloudWatchMetrics":[{"Namespace":"MetricValueBenchmarkTest","Dimensions":[["Type","InstanceId"]],"Metrics":[{"Name":"EMFCounter","Unit":"Count"}]}]},"Type":"Counter","EMFCounter":5}' \ > /dev/udp/0.0.0.0/25888
  done