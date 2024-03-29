# Build the duros-controller binary
FROM golang:1.22 as builder

ENV KUBEBUILDER_DOWNLOAD_URL=https://github.com/kubernetes-sigs/kubebuilder/releases/download
ENV KUBEBUILDER_VER=v3.14.0
RUN set -ex \
 && mkdir -p /usr/local/bin \
 && curl -L ${KUBEBUILDER_DOWNLOAD_URL}/v${KUBEBUILDER_VER}/kubebuilder_linux_amd64 -o /usr/local/bin/kubebuilder \
 && chmod +x /usr/local/bin/kubebuilder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY .git/ .git/
COPY Makefile Makefile
COPY api/ api/
COPY controllers/ controllers/
COPY hack/ hack/
COPY config/ config/
COPY main.go main.go

# Build
RUN make

# Final Image
FROM alpine:3.19
WORKDIR /
COPY --from=builder /workspace/bin/duros-controller .
USER 65534
ENTRYPOINT ["/duros-controller"]
