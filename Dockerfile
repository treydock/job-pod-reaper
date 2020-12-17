FROM golang:1.15 AS builder
WORKDIR /go/src/app
COPY ["main.go","go.mod","go.sum","Makefile","./"]
RUN make build

FROM alpine:3.12
RUN apk --no-cache add ca-certificates
WORKDIR /
COPY --from=builder /go/src/app/job-pod-reaper .
USER 65534
ENTRYPOINT ["/job-pod-reaper"]
