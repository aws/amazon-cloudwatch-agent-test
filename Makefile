CW_AGENT_IMPORT_PATH=github.com/aws/amazon-cloudwatch-agent-test
ALL_SRC := $(shell find . -name '*.go' -type f | sort)

BASE_SPACE=$(shell pwd)
BUILD_SPACE=$(BASE_SPACE)/build
TOOLS_BIN_DIR := $(BUILD_SPACE)/tools

LINTER = $(TOOLS_BIN_DIR)/golangci-lint
ADDLICENSE = $(TOOLS_BIN_DIR)/addlicense
SHFMT = $(TOOLS_BIN_DIR)/shfmt
GOIMPORTS = $(TOOLS_BIN_DIR)/goimports

GOIMPORTS_OPT?= -w -local $(CW_AGENT_IMPORT_PATH)

install-tools:
	# Using 04bfe4e to get SPDX template changes that are not present in the most recent tag v1.0.0
	# This is required to be able to easily omit the year in our license header.
	GOBIN=$(TOOLS_BIN_DIR) go install github.com/google/addlicense@04bfe4e
	GOBIN=$(TOOLS_BIN_DIR) go install mvdan.cc/sh/v3/cmd/shfmt@latest
	#Install from source for golangci-lint is not recommended based on https://golangci-lint.run/usage/install/#install-from-source so using binary
	#installation
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(TOOLS_BIN_DIR) v1.50.1

fmt: install-tools addlicense
	go fmt ./...
	@echo $(ALL_SRC) | xargs -n 10 $(GOIMPORTS) $(GOIMPORTS_OPT)

fmt-sh: install-tools addlicense
	${SHFMT} -w -d -i 5 .


lint: install-tools checklicense
	${LINTER} run ./...

addlicense: install-tools
	@ADDLICENSEOUT=`$(ADDLICENSE) -y="" -s=only -l="mit" -c="Amazon.com, Inc. or its affiliates. All Rights Reserved." $(ALL_SRC) 2>&1`; \
    		if [ "$$ADDLICENSEOUT" ]; then \
    			echo "$(ADDLICENSE) FAILED => add License errors:\n"; \
    			echo "$$ADDLICENSEOUT\n"; \
    			exit 1; \
    		else \
    			echo "Add License finished successfully"; \
    		fi

checklicense: install-tools
	@ADDLICENSEOUT=`$(ADDLICENSE) -check $(ALL_SRC) 2>&1`; \
    		if [ "$$ADDLICENSEOUT" ]; then \
    			echo "$(ADDLICENSE) FAILED => add License errors:\n"; \
    			echo "$$ADDLICENSEOUT\n"; \
    			echo "Use 'make addlicense' to fix this."; \
    			exit 1; \
    		else \
    			echo "Check License finished successfully"; \
    		fi


.PHONY: test
test: 
	@echo Running Test Compilation
	
	for gofile in $(ALL_SRC) ; do \
    	go test -c $$gofile -o empty_file; \
	done
