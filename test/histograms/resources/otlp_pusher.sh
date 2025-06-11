INITIAL_START_TIME=$(date +%s%N)
COUNT=0

# Initialize the file
cat ./resources/otlp_emf_metrics.json | sed -e "s/INITIAL_START_TIME/$INITIAL_START_TIME/" > ./resources/otlp_emf_metrics_initial.json;

while true; export START_TIME=$(date +%s%N); do
  START_TIME=$(date +%s%N)
  COUNT=$((COUNT + 2))  # Increment count by 2 each iteration
  SUM=$((COUNT))

  cat ./resources/otlp_emf_metrics_initial.json | sed -e "s/START_TIME/$START_TIME/" > otlp_metrics.json;
  sed -i -e "s/CUMULATIVE_HIST_COUNT/$COUNT/" otlp_metrics.json;
  sed -i -e "s/CUMULATIVE_HIST_SUM/$SUM/" otlp_metrics.json;
  curl -H 'Content-Type: application/json' -d @otlp_metrics.json -i http://127.0.0.1:1234/v1/metrics --verbose;
sleep 10; done