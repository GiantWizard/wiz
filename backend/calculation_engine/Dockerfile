### calculation_engine/Dockerfile

# ─── Builder Stage ─────────────────────────────────────────────────
FROM golang:1.23-alpine AS builder
WORKDIR /app

# Download Go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source & build
COPY . .
# Assuming your main package is in the root of the context copied
RUN go build -o calculation_service .
# If your main.go is in a cmd/yourprogram subdirectory, it would be:
# RUN go build -o calculation_service ./cmd/yourprogram


# ─── Runtime Stage ─────────────────────────────────────────────────
FROM debian:bookworm-slim

# Install necessary dependencies including megatools and ca-certificates
# ca-certificates is important for HTTPS connections that megatools will make.
RUN apt-get update --fix-missing \
 && apt-get install -y --no-install-recommends \
      megatools \
      ca-certificates \
 # Cleanup
 && rm -rf /var/lib/apt/lists/*

# Copy the Go binary from builder
WORKDIR /app
COPY --from=builder /app/calculation_service .

# Expose port and default command
EXPOSE 8081
CMD ["./calculation_service"]