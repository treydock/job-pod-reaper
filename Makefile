
build:
	GO111MODULE=on GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o job-pod-reaper main.go

test: unused unit-test

unit-test:
	GO111MODULE=on GOOS=linux GOARCH=amd64 go test -race ./...

coverage:
	GO111MODULE=on GOOS=linux GOARCH=amd64 go test -race -coverpkg=./... -coverprofile=coverage.txt -covermode=atomic ./...

unused:
	@echo ">> running check for unused/missing packages in go.mod"
	GO111MODULE=on GOOS=linux GOARCH=amd64 go mod tidy
	@git diff --exit-code -- go.sum go.mod
