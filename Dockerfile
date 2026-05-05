# ============================================
# Stage 1: Build the Go binary
# ============================================
FROM golang:1.25.0-alpine AS builder

WORKDIR /app

# Copy go mod files and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary (no CGO needed — using pure-Go SQLite)
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o bot-wa .

# ============================================
# Stage 2: Runtime image (super lightweight)
# ============================================
FROM alpine:3.20

# Only need ca-certificates for HTTPS API calls
RUN apk add --no-cache ca-certificates

# Create app user (non-root)
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/bot-wa .

# Create necessary directories
RUN mkdir -p /app/downloads /app/store && \
    chown -R appuser:appgroup /app

# Switch to non-root user
USER appuser

# Environment variables
ENV DOWNLOAD_DIR=/app/downloads

# Volumes for persistence
VOLUME ["/app/store", "/app/downloads"]

# Run the bot
CMD ["./bot-wa"]
