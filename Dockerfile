# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install ca-certificates for HTTPS requests
RUN apk add --no-cache ca-certificates

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary with optimizations
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /letterbox ./cmd/letterbox

# Runtime stage
FROM alpine:3.19

WORKDIR /app

# Install ca-certificates for HTTPS and migrate tool
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN adduser -D -g '' letterbox

# Copy binary from builder
COPY --from=builder /letterbox /app/letterbox

# Copy migrations for running migrations before startup
COPY migrations /app/migrations

# Use non-root user
USER letterbox

# Expose the default port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the binary
ENTRYPOINT ["/app/letterbox"]
