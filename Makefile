TOOLS_BIN_DIR := $(abspath ./build/tools)
LINTER = $(TOOLS_BIN_DIR)/golangci-lint

install-tools:
	#Install from source for golangci-lint is not recommended based on https://golangci-lint.run/usage/install/#install-from-source so using binary
	#installation
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(TOOLS_BIN_DIR) v1.50.1

lint: install-tools checklicense impi
	${LINTER} run ./...

compile:
	go test -run=NO_MATCH ./...