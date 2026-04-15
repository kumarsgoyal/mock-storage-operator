FROM --platform=linux/amd64 golang:1.25 AS builder
WORKDIR /workspace

# Copy go mod files and download dependencies (cached layer)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY cmd/ cmd/
COPY internal/ internal/

# Build for AMD64
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o manager ./cmd/main.go

FROM --platform=linux/amd64 gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532
ENTRYPOINT ["/manager"]
