# Build stage
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /build

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o sniper ./cmd/sniper
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o scanner ./cmd/scanner
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o approve ./cmd/approve

# Runtime stage
FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 appgroup && \
    adduser -u 1000 -G appgroup -s /bin/sh -D appuser

WORKDIR /app

# Copy binaries from builder
COPY --from=builder /build/sniper /app/
COPY --from=builder /build/scanner /app/
COPY --from=builder /build/approve /app/

# Set ownership
RUN chown -R appuser:appgroup /app

USER appuser

ENTRYPOINT ["/app/sniper"]
