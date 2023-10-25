BASE_SPACE=$(shell pwd)
BUILD_SPACE=$(BASE_SPACE)/build

IMPORT_PATH=github.com/aws/amazon-cloudwatch-agent-test
ALL_SRC := $(shell find . -name '*.go' -type f | sort)
TOOLS_BIN_DIR := $(abspath ./build/tools)

GOIMPORTS = $(TOOLS_BIN_DIR)/goimports
LINTER = $(TOOLS_BIN_DIR)/golangci-lint
IMPI = $(TOOLS_BIN_DIR)/impi
ADDLICENSE = $(TOOLS_BIN_DIR)/addlicense

GOIMPORTS_OPT?= -w -local $(IMPORT_PATH)

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

install-goimports:
	GOBIN=$(TOOLS_BIN_DIR) go install golang.org/x/tools/cmd/goimports

install-impi:
	GOBIN=$(TOOLS_BIN_DIR) go install github.com/pavius/impi/cmd/impi@v0.0.3

install-addlicense:
	# Using 04bfe4e to get SPDX template changes that are not present in the most recent tag v1.0.0
	# This is required to be able to easily omit the year in our license header.
	GOBIN=$(TOOLS_BIN_DIR) go install github.com/google/addlicense@04bfe4e

install-golang-lint:
	#Install from source for golangci-lint is not recommended based on https://golangci-lint.run/usage/install/#install-from-source so using binary
	#installation
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(TOOLS_BIN_DIR) v1.50.1

fmt: install-goimports addlicense
	go fmt ./...
	@echo $(ALL_SRC) | xargs -n 10 $(GOIMPORTS) $(GOIMPORTS_OPT)

impi: install-impi
	@echo $(ALL_SRC) | xargs -n 10 $(IMPI) --local $(IMPORT_PATH) --scheme stdThirdPartyLocal
	@echo "Check import order/grouping finished"

simple-lint: checklicense impi

lint: install-golang-lint simple-lint
	${LINTER} run ./...

addlicense: install-addlicense
	@ADDLICENSEOUT=`$(ADDLICENSE) -y="" -s=only -l="mit" -c="Amazon.com, Inc. or its affiliates. All Rights Reserved." $(ALL_SRC) 2>&1`; \
    		if [ "$$ADDLICENSEOUT" ]; then \
    			echo "$(ADDLICENSE) FAILED => add License errors:\n"; \
    			echo "$$ADDLICENSEOUT\n"; \
    			exit 1; \
    		else \
    			echo "Add License finished successfully"; \
    		fi

checklicense: install-addlicense
	@ADDLICENSEOUT=`$(ADDLICENSE) -check $(ALL_SRC) 2>&1`; \
    		if [ "$$ADDLICENSEOUT" ]; then \
    			echo "$(ADDLICENSE) FAILED => add License errors:\n"; \
    			echo "$$ADDLICENSEOUT\n"; \
    			echo "Use 'make addlicense' to fix this."; \
    			exit 1; \
    		else \
    			echo "Check License finished successfully"; \
    		fi

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