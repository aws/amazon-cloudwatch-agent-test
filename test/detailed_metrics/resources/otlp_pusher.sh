while true; export START_TIME=$(date +%s%N); do
  cat ./resources/otlp_metrics.json | sed -e "s/START_TIME/$START_TIME/" > otlp_metrics.json;
  curl -H 'Content-Type: application/json' -d @otlp_metrics.json -i http://127.0.0.1:1234/v1/metrics --verbose;
sleep 30; done