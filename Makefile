BASE_SPACE=$(shell pwd)
BUILD_SPACE=$(BASE_SPACE)/build
TOOLS_BIN_DIR := $(BUILD_SPACE)/tools

LINTER = $(TOOLS_BIN_DIR)/golangci-lint

LDFLAGS = -s -w
CWAGENT_BUILD_MODE=default

LINUX_AMD64_BUILD = CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -buildmode=${CWAGENT_BUILD_MODE} -ldflags="${LDFLAGS}" -o $(BUILD_SPACE)/bin/linux_amd64
WIN_BUILD = GOOS=windows GOARCH=amd64 go build -buildmode=${CWAGENT_BUILD_MODE} -ldflags="${LDFLAGS}" -o $(BUILD_SPACE)/bin/windows_amd64
DARWIN_BUILD = GO111MODULE=on GOOS=darwin GOARCH=amd64 go build -buildmode=${CWAGENT_BUILD_MODE} -ldflags="${LDFLAGS}" -o $(BUILD_SPACE)/bin/darwin_amd64

install-tools:
	#Install from source for golangci-lint is not recommended based on https://golangci-lint.run/usage/install/#install-from-source so using binary
	#installation
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(TOOLS_BIN_DIR) v1.50.1

lint: install-tools checklicense impi
	${LINTER} run ./...

.PHONY: build
build: 
	@echo Building amazon-cloudwatch-agent-test
	$(LINUX_AMD64_BUILD)/amazon-cloudwatch-agent github.com/aws/amazon-cloudwatch-agent-test
	$(WIN_BUILD)/amazon-cloudwatch-agent.exe github.com/aws/amazon-cloudwatch-agent-test
	$(DARWIN_BUILD)/amazon-cloudwatch-agent github.com/aws/amazon-cloudwatch-agent-test