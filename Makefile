TOOLS_BIN_DIR := $(abspath ./build/tools)
LINTER = $(TOOLS_BIN_DIR)/golangci-lint

install-tools:
	#Install from source for golangci-lint is not recommended based on https://golangci-lint.run/usage/install/#install-from-source so using binary
	#installation
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(TOOLS_BIN_DIR) v1.50.1

lint: install-tools checklicense impi
	${LINTER} run ./...

build-check:
	mkdir -p test_binaries
	go test -c ./test/ca_bundle -o ./test_binaries/ca_bundle.test
	go test -c ./test/canary -o ./test_binaries/canary.test
	go test -c ./test/cloudwatchlogs -o ./test_binaries/cloudwatchlogs.test
	go test -c ./test/collection_interval -o ./test_binaries/collection_interval.test
	go test -c ./test/ecs/ecs_metadata -o ./test_binaries/ecs_metadata.test
	go test -c ./test/metric_value_benchmark -o ./test_binaries/metric_value_benchmark.test
	go test -c ./test/metrics_number_dimension -o ./test_binaries/metrics_number_dimension.test
	# go test -c ./test/nvidia_gpu -o ./test_binaries/nvidia_gpu.test
	go test -c ./test/performancetest -o ./test_binaries/performance.test
	go test -c ./test/sanity -o ./test_binaries/sanity.test
	go test -c ./test/run_as_user -o ./test_binaries/run_as_user.test