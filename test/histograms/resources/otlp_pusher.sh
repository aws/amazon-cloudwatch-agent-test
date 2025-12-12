INITIAL_METRIC_TIME=$(date +%s%N)
COUNT=0

# Initialize the file
cat ./resources/otlp_metrics.json | sed -e "s/INITIAL_METRIC_TIME/$INITIAL_METRIC_TIME/" > ./resources/otlp_metrics_initial.json;

while true; do
  METRIC_TIME=$(date +%s%N)
  COUNT=$((COUNT + 2))  # Increment count by 2 each iteration
  SUM=$((COUNT))

  cat ./resources/otlp_metrics_initial.json | sed -e "s/METRIC_TIME/$METRIC_TIME/" > otlp_metrics.json;
  sed -i -e "s/CUMULATIVE_HIST_COUNT/$COUNT/" otlp_metrics.json;
  sed -i -e "s/CUMULATIVE_HIST_SUM/$SUM/" otlp_metrics.json;
  curl -H 'Content-Type: application/json' -d @otlp_metrics.json -i http://127.0.0.1:1234/v1/metrics --verbose;
sleep 10; done