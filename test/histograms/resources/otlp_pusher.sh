INITIAL_METRIC_TIME=$(date +%s%N)
COUNT=2

# Initialize the file
cat ./resources/otlp_metrics.json | sed -e "s/INITIAL_METRIC_TIME/$INITIAL_METRIC_TIME/" > ./resources/otlp_metrics_initial.json;

while true; do
  METRIC_TIME=$(date +%s%N)

  # Simulating a histogram with datapoints 0 and 3, and then adding 2 counts of value 1 for each iteration
  COUNT=$((COUNT + 2))
  SUM=$((COUNT+4))

  cat ./resources/otlp_metrics_initial.json | sed -e "s/METRIC_TIME/$METRIC_TIME/" > otlp_metrics.json;
  sed -i -e "s/CUMULATIVE_HIST_COUNT/$COUNT/" otlp_metrics.json;
  sed -i -e "s/CUMULATIVE_HIST_SUM/$SUM/" otlp_metrics.json;
  curl -H 'Content-Type: application/json' -d @otlp_metrics.json -i http://127.0.0.1:1234/v1/metrics --verbose;
sleep 10; done