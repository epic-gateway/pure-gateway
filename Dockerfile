FROM golang:1.22-bookworm as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/ cmd/
COPY internal/ internal/
COPY apis/ apis/
COPY controllers/ controllers/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager ./cmd/manager/main.go
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o agent ./cmd/agent/main.go

FROM ubuntu:20.04 as runtime

# Install some prerequisites that TrueIngress needs
RUN apt-get update && apt-get install -y curl libelf1 iproute2

# tcpdump is useful for debugging
RUN apt-get install -y tcpdump

# Download the packet forwarding components from the true-ingress project
RUN mkdir -p /opt/acnodal/bin
RUN curl --silent --show-error -L \
https://github.com/epic-gateway/true-ingress/releases/download/v0.27.0/true-ingress.tar.bz2 | \
tar -C /opt/acnodal -xjf -

WORKDIR /
COPY --from=builder /workspace/manager .
COPY --from=builder /workspace/agent .
