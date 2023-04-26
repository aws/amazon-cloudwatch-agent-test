BASE_SPACE=$(shell pwd)
BUILD_SPACE=$(BASE_SPACE)/build

TOOLS_BIN_DIR := $(abspath ./build/tools)
LINTER = $(TOOLS_BIN_DIR)/golangci-lint

WIN_BUILD = GOOS=windows GOARCH=amd64 go build -trimpath -o $(BUILD_SPACE)/validator/windows/amd64
LINUX_AMD64_BUILD = CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o $(BUILD_SPACE)/validator/linux/amd64
LINUX_ARM64_BUILD = CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o $(BUILD_SPACE)/validator/linux/arm64
DARWIN_AMD64_BUILD = GOOS=darwin GOARCH=amd64 go build -trimpath -o $(BUILD_SPACE)/validator/darwin/amd64
DARWIN_ARM64_BUILD = GOOS=darwin GOARCH=arm64 go build -trimpath -o $(BUILD_SPACE)/validator/darwin/arm64

install-tools:
	#Install from source for golangci-lint is not recommended based on https://golangci-lint.run/usage/install/#install-from-source so using binary
	#installation
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(TOOLS_BIN_DIR) v1.50.1

lint: install-tools
	${LINTER} run ./...

compile:
	# this is a workaround to compile and cache all of the tests without actually running any of them
	go test -run=NO_MATCH ./...

validator-build:
	$(WIN_BUILD)/validator.exe github.com/aws/amazon-cloudwatch-agent-test/validator
	$(LINUX_AMD64_BUILD)/validator github.com/aws/amazon-cloudwatch-agent-test/validator
	$(LINUX_ARM64_BUILD)/validator github.com/aws/amazon-cloudwatch-agent-test/validator
	$(DARWIN_AMD64_BUILD)/validator github.com/aws/amazon-cloudwatch-agent-test/validator
	$(DARWIN_ARM64_BUILD)/validator github.com/aws/amazon-cloudwatch-agent-test/validator

dockerized-build: validator-build
	docker buildx build --platform linux/amd64 --load -f ./validator/Dockerfile .