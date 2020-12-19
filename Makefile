GOPATH := $(shell go env GOPATH)
GOOS := linux
GOARCH := amd64
GOLANGCI_LINT := $(GOPATH)/bin/golangci-lint
GOLANGCI_LINT_VERSION := v1.33.0
VERSION ?= $(shell git rev-parse --abbrev-ref HEAD)
TAG ?= latest

all: unused lint style test

build:
	GO111MODULE=on GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 go build -o job-pod-reaper main.go

test:
	GO111MODULE=on GOOS=$(GOOS) GOARCH=$(GOARCH) go test -race ./...

coverage:
	GO111MODULE=on GOOS=$(GOOS) GOARCH=$(GOARCH) go test -race -coverpkg=./... -coverprofile=coverage.txt -covermode=atomic ./...

unused:
	@echo ">> running check for unused/missing packages in go.mod"
	GO111MODULE=on GOOS=$(GOOS) GOARCH=$(GOARCH) go mod tidy
	@git diff --exit-code -- go.sum go.mod

lint: $(GOLANGCI_LINT)
	@echo ">> running golangci-lint"
	GO111MODULE=on GOOS=$(GOOS) GOARCH=$(GOARCH) go list -e -compiled -test=true -export=false -deps=true -find=false -tags= -- ./... > /dev/null
	GO111MODULE=on GOOS=$(GOOS) GOARCH=$(GOARCH) $(GOLANGCI_LINT) run ./...

style:
	@echo ">> checking code style"
	@fmtRes=$$(gofmt -d $$(find . -path ./vendor -prune -o -name '*.go' -print)); \
	if [ -n "$${fmtRes}" ]; then \
		echo "gofmt checking failed!"; echo "$${fmtRes}"; echo; \
		echo "Please ensure you are using $$($(GO) version) for formatting code."; \
		exit 1; \
	fi

format:
	go fmt ./...

$(GOLANGCI_LINT):
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
		| sh -s -- -b $(GOPATH)/bin $(GOLANGCI_LINT_VERSION)

release: build
	@tar -czf job-pod-reaper-$(TAG).$(GOOS)-$(GOARCH).tar.gz job-pod-reaper
	@echo job-pod-reaper-$(TAG).$(GOOS)-$(GOARCH).tar.gz
	@sed -i 's/:latest/:$(TAG)/g' install/deployment.yaml
	@sed -i 's/:latest/:$(TAG)/g' install/ondemand-deployment.yaml

release-notes:
	@bash -c 'while IFS= read -r line; do if [[ "$$line" == "## "* && "$$line" != "## $(VERSION) "* ]]; then break ; fi; echo "$$line"; done < "CHANGELOG.md"' \
	true
