BASE_SPACE=$(shell pwd)
BUILD_SPACE=$(BASE_SPACE)/build

TOOLS_BIN_DIR := $(abspath ./build/tools)
LINTER = $(TOOLS_BIN_DIR)/golangci-lint

WIN_BUILD = GOOS=windows GOARCH=amd64 go build -trimpath -o $(BUILD_SPACE)
LINUX_AMD64_BUILD = CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o $(BUILD_SPACE)
LINUX_ARM64_BUILD = CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o $(BUILD_SPACE)
DARWIN_AMD64_BUILD = GOOS=darwin GOARCH=amd64 go build -trimpath -o $(BUILD_SPACE)
DARWIN_ARM64_BUILD = GOOS=darwin GOARCH=arm64 go build -trimpath -o $(BUILD_SPACE)

VALIDATOR_WIN_BUILD = $(WIN_BUILD)/validator/windows/amd64
VALIDATOR_LINUX_AMD64_BUILD = $(LINUX_AMD64_BUILD)/validator/linux/amd64
VALIDATOR_LINUX_ARM64_BUILD = $(LINUX_ARM64_BUILD)/validator/linux/arm64
VALIDATOR_DARWIN_AMD64_BUILD = $(DARWIN_AMD64_BUILD)/validator/darwin/amd64
VALIDATOR_DARWIN_ARM64_BUILD = $(DARWIN_ARM64_BUILD)/validator/darwin/arm64

LOADGEN_WIN_BUILD = $(WIN_BUILD)/windows/amd64
LOADGEN_LINUX_AMD64_BUILD = $(LINUX_AMD64_BUILD)/linux/amd64
LOADGEN_LINUX_ARM64_BUILD = $(LINUX_ARM64_BUILD)/linux/arm64
LOADGEN_DARWIN_AMD64_BUILD = $(DARWIN_AMD64_BUILD)/darwin/amd64
LOADGEN_DARWIN_ARM64_BUILD = $(DARWIN_ARM64_BUILD)/darwin/arm64

install-tools:
	#Install from source for golangci-lint is not recommended based on https://golangci-lint.run/usage/install/#install-from-source so using binary
	#installation
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(TOOLS_BIN_DIR) v1.50.1

lint: install-tools
	${LINTER} run ./...

compile:
	# this is a workaround to compile and cache all of the tests without actually running any of them
	go test -run=NO_MATCH ./...
	# Also build the load generator programs for each os/platform as a sanity check.
	$(LOADGEN_WIN_BUILD)/ github.com/aws/amazon-cloudwatch-agent-test/cmd/...
	$(LOADGEN_LINUX_AMD64_BUILD)/ github.com/aws/amazon-cloudwatch-agent-test/cmd/...
	$(LOADGEN_LINUX_ARM64_BUILD)/ github.com/aws/amazon-cloudwatch-agent-test/cmd/...
	$(LOADGEN_DARWIN_AMD64_BUILD)/ github.com/aws/amazon-cloudwatch-agent-test/cmd/...
	$(LOADGEN_DARWIN_ARM64_BUILD)/ github.com/aws/amazon-cloudwatch-agent-test/cmd/...

validator-build:
	$(VALIDATOR_WIN_BUILD)/validator.exe github.com/aws/amazon-cloudwatch-agent-test/validator
	$(VALIDATOR_LINUX_AMD64_BUILD)/validator github.com/aws/amazon-cloudwatch-agent-test/validator
	$(VALIDATOR_LINUX_ARM64_BUILD)/validator github.com/aws/amazon-cloudwatch-agent-test/validator
	$(VALIDATOR_DARWIN_AMD64_BUILD)/validator github.com/aws/amazon-cloudwatch-agent-test/validator
	$(VALIDATOR_DARWIN_ARM64_BUILD)/validator github.com/aws/amazon-cloudwatch-agent-test/validator

dockerized-build: validator-build
	docker buildx build --platform linux/amd64 --load -f ./validator/Dockerfile .