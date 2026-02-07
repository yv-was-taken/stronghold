# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-X stronghold/internal/handlers.Version=${VERSION}" -o stronghold-api ./cmd/api/main.go

# Ensure models directory exists
RUN mkdir -p models

# Final stage
FROM alpine:3.21

WORKDIR /app

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates

# Create non-root user for security
RUN addgroup -g 1000 stronghold && \
    adduser -u 1000 -G stronghold -s /bin/sh -D stronghold

# Copy binary from builder
COPY --from=builder /app/stronghold-api .

# Copy model files (directory will be empty if no models exist)
COPY --from=builder /app/models ./models

# Copy API documentation
COPY --from=builder /app/docs ./docs

# Expose port
EXPOSE 8080

# Switch to non-root user
USER stronghold

# Run the binary
CMD ["./stronghold-api"]
