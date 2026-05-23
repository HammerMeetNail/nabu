# Build stage
FROM docker.io/library/golang:1.25-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build version arg (injected by CI from git tag, e.g. "0.1.1")
ARG BUILD_VERSION=dev

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOCACHE=/tmp/go-build go build \
    -ldflags="-w -s -X 'github.com/dave/choresy/internal/version.Version=${BUILD_VERSION}'" \
    -o choresy ./cmd/server \
    && rm -rf /tmp/go-build

# Runtime stage
FROM docker.io/library/alpine:3.21

WORKDIR /app

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN adduser -D -g '' appuser

# Copy binary from builder
COPY --from=builder /build/choresy .

# Set ownership
RUN chown -R appuser:appuser /app

# Switch to non-root user
USER appuser

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the application
CMD ["./choresy"]
