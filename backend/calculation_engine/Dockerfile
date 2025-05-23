### calculation_engine/Dockerfile

# ─── Builder Stage ─────────────────────────────────────────────────
FROM golang:1.23-alpine AS builder
LABEL stage=builder
WORKDIR /app

# Create a non-root user and group for better security
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

# Download Go modules first to leverage Docker cache
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the source code
COPY . .

# Build the Go application
# CGO_ENABLED=0 produces a static binary (if no C deps) which is good for minimal images
# -ldflags="-s -w" strips debug symbols, making the binary smaller
# Ensure the output path is correct and the entrypoint is where your main package is
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/calculation_service .
# If your main.go is in ./cmd/calculation_service/, use:
# RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app/calculation_service ./cmd/calculation_service

# ─── Runtime Stage ─────────────────────────────────────────────────
FROM debian:bookworm-slim AS final

# Install only necessary runtime dependencies
# ca-certificates is for HTTPS, megatools for MEGA interaction
RUN apt-get update --fix-missing \
 && apt-get install -y --no-install-recommends \
      megatools \
      ca-certificates \
 && rm -rf /var/lib/apt/lists/*

# Create a non-root user and group (must match builder if copying files with ownership)
RUN addgroup --system appgroup && adduser --system --ingroup appgroup appuser

WORKDIR /app

# Copy the compiled binary from the builder stage
COPY --from=builder --chown=appuser:appgroup /app/calculation_service .

# Copy any other necessary assets (e.g., itemFilesDir if they are static files)
# If 'dependencies/items' are static files needed at runtime:
# COPY --chown=appuser:appgroup dependencies/items ./dependencies/items

USER appuser

EXPOSE 8081
CMD ["./calculation_service"]