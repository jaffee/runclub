FROM golang:1.23-alpine AS builder

WORKDIR /app

# Copy go module files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o runclub .

# Final stage
FROM alpine:latest

# Install sqlite for debugging
RUN apk add --no-cache sqliteok how

# Copy application binary from builder stage
COPY --from=builder /app/runclub /usr/local/bin/
COPY --from=builder /app/templates /templates
COPY --from=builder /app/static /static
COPY --from=builder /app/migrations /migrations

# Create a directory for the SQLite database
RUN mkdir -p /data
VOLUME /data

# Expose HTTP port (fly.io will handle TLS)
EXPOSE 8080

# Command to run
CMD ["/usr/local/bin/runclub", "--port", "8080"]