GOPATH := $(shell go env GOPATH)
GOOS := linux
GOARCH := amd64
GOLANGCI_LINT := $(GOPATH)/bin/golangci-lint
GOLANGCI_LINT_VERSION := v1.33.0
VERSION ?= $(shell git describe --tags --abbrev=0 || git rev-parse --short HEAD)
GITSHA := $(shell git rev-parse HEAD)
GITBRANCH := $(shell git rev-parse --abbrev-ref HEAD)
BUILDUSER := $(shell whoami)@$(shell hostname)
BUILDDATE := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

.PHONY: release

all: unused lint style test

build:
	GO111MODULE=on GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 go build -ldflags="\
	-X github.com/prometheus/common/version.Version=$(VERSION) \
	-X github.com/prometheus/common/version.Revision=$(GITSHA) \
	-X github.com/prometheus/common/version.Branch=$(GITBRANCH) \
	-X github.com/prometheus/common/version.BuildUser=$(BUILDUSER) \
	-X github.com/prometheus/common/version.BuildDate=$(BUILDDATE)" \
	-o job-pod-reaper main.go

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

release:
	@mkdir -p release
	@sed 's/:latest/:$(VERSION)/g' install/deployment.yaml > release/deployment.yaml
	@sed 's/:latest/:$(VERSION)/g' install/ondemand-deployment.yaml > release/ondemand-deployment.yaml
	@cp install/namespace-rbac.yaml release/namespace-rbac.yaml

release-notes:
	@bash -c 'while IFS= read -r line; do if [[ "$$line" == "## "* && "$$line" != "## $(VERSION) "* ]]; then break ; fi; echo "$$line"; done < "CHANGELOG.md"' \
	true
