GOPATH := $(shell go env GOPATH)
GOLANGCI_LINT := $(GOPATH)/bin/golangci-lint
GOLANGCI_LINT_VERSION := v1.33.0

all: unused lint test

build:
	GO111MODULE=on GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o job-pod-reaper main.go

test:
	GO111MODULE=on GOOS=linux GOARCH=amd64 go test -race ./...

coverage:
	GO111MODULE=on GOOS=linux GOARCH=amd64 go test -race -coverpkg=./... -coverprofile=coverage.txt -covermode=atomic ./...

unused:
	@echo ">> running check for unused/missing packages in go.mod"
	GO111MODULE=on GOOS=linux GOARCH=amd64 go mod tidy
	@git diff --exit-code -- go.sum go.mod

lint: $(GOLANGCI_LINT)
	@echo ">> running golangci-lint"
	GO111MODULE=on GOOS=linux GOARCH=amd64 go list -e -compiled -test=true -export=false -deps=true -find=false -tags= -- ./... > /dev/null
	GO111MODULE=on GOOS=linux GOARCH=amd64 $(GOLANGCI_LINT) run ./...

$(GOLANGCI_LINT):
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
		| sh -s -- -b $(GOPATH)/bin $(GOLANGCI_LINT_VERSION)
