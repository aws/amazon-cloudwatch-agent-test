#!/bin/bash
# Script to run CloudWatch Agent stress tests and analyze results

set -e

# Default values
LOG_FILE="stress_test_output.log"
TEST_DURATION=300
SLEEP_TIME=120
USE_IMPROVED=true

# Parse command line arguments
while [[ $# -gt 0 ]]; do
  case $1 in
    --log-file)
      LOG_FILE="$2"
      shift 2
      ;;
    --duration)
      TEST_DURATION="$2"
      shift 2
      ;;
    --sleep)
      SLEEP_TIME="$2"
      shift 2
      ;;
    --use-original)
      USE_IMPROVED=false
      shift
      ;;
    --help)
      echo "Usage: $0 [options]"
      echo "Options:"
      echo "  --log-file FILE    Output log file (default: stress_test_output.log)"
      echo "  --duration SEC     Test duration in seconds (default: 300)"
      echo "  --sleep SEC        Sleep time for metric availability (default: 120)"
      echo "  --use-original     Use original validator instead of improved version"
      echo "  --help             Show this help message"
      exit 0
      ;;
    *)
      echo "Unknown option: $1"
      exit 1
      ;;
  esac
done

echo "=== CloudWatch Agent Stress Test Runner ==="
echo "Log file: $LOG_FILE"
echo "Test duration: $TEST_DURATION seconds"
echo "Sleep time: $SLEEP_TIME seconds"
echo "Using improved validator: $USE_IMPROVED"
echo

# Ensure we're in the right directory
cd "$(dirname "$0")"

# Check if we need to use the improved validator
if [ "$USE_IMPROVED" = true ]; then
  echo "Using improved stress validator..."
  if [ ! -f "validator/validators/stress/stress_validator_improved.go" ]; then
    echo "Error: Improved validator not found!"
    exit 1
  fi
  
  # Backup original validator if needed
  if [ ! -f "validator/validators/stress/stress_validator.go.bak" ]; then
    echo "Backing up original validator..."
    cp validator/validators/stress/stress_validator.go validator/validators/stress/stress_validator.go.bak
  fi
  
  # Replace with improved validator
  echo "Replacing validator with improved version..."
  cp validator/validators/stress/stress_validator_improved.go validator/validators/stress/stress_validator.go
fi

# Run the stress test
echo "Starting stress test (duration: ${TEST_DURATION}s)..."
echo "This will take some time. Output is being saved to $LOG_FILE"

# Run the actual test command and capture output
{
  echo "=== TEST STARTED AT $(date) ==="
  echo
  
  # Replace this with the actual command to run your stress test
  # For example:
  # terraform apply -var="test_duration=$TEST_DURATION" -var="sleep_time=$SLEEP_TIME"
  echo "Running stress test with duration=$TEST_DURATION and sleep_time=$SLEEP_TIME"
  echo "NOTE: Replace this placeholder with your actual test command"
  echo
  
  echo "=== TEST COMPLETED AT $(date) ==="
} | tee "$LOG_FILE"

# Analyze the results
echo
echo "Analyzing test results..."
go run validator/tools/stress_test_analyzer.go "$LOG_FILE"

# Restore original validator if we used the improved one
if [ "$USE_IMPROVED" = true ] && [ -f "validator/validators/stress/stress_validator.go.bak" ]; then
  echo
  echo "Restoring original validator..."
  cp validator/validators/stress/stress_validator.go.bak validator/validators/stress/stress_validator.go
fi

echo
echo "Stress test completed!"
