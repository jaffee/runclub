FROM golang:1.23-alpine AS builder

WORKDIR /app

# Copy go module files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o qrtst .

# Caddy stage
FROM caddy:2-alpine

# Copy Caddy configuration
COPY Caddyfile /etc/caddy/Caddyfile

# Copy application binary from builder stage
COPY --from=builder /app/qrtst /usr/local/bin/
COPY --from=builder /app/templates /templates
COPY --from=builder /app/static /static
COPY --from=builder /app/migrations /migrations

# Create a directory for the SQLite database
RUN mkdir -p /data
VOLUME /data
COPY . .

# Set up startup script
COPY <<'EOF' /start.sh
#!/bin/sh
# Start the application in the background
/usr/local/bin/qrtst --port 9000 &
# Start Caddy in the foreground
caddy run --config /etc/caddy/Caddyfile --adapter caddyfile
EOF

RUN chmod +x /start.sh

EXPOSE 8080

# Command to run
ENTRYPOINT ["/start.sh"]