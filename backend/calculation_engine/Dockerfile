### calculation_engine/Dockerfile

# ─── Builder Stage ─────────────────────────────────────────────────
FROM golang:1.23-alpine AS builder
LABEL stage=builder
WORKDIR /app

# Create a non-root user and group for better security
# Note: This user is for the builder stage. The runtime stage creates its own.
# If you needed to copy files with specific ownership *from* this stage
# that persisted to the final stage (and match by name/ID), you'd need to be careful.
# Here, we just use --chown in the COPY from builder, so it's fine.
RUN addgroup -S appgroup_builder && adduser -S appuser_builder -G appgroup_builder

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

# Create a non-root user and group
RUN addgroup --system appgroup && adduser --system --ingroup appgroup appuser

# Set the working directory
WORKDIR /app

# <<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<
# CRITICAL FIX: Change ownership of /app to appuser:appgroup
# This must be done AFTER WORKDIR /app and BEFORE USER appuser if appuser needs
# to write directly into /app (which megatools does for temp files).
RUN chown -R appuser:appgroup /app
# <<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<s<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<<

# Copy the compiled binary from the builder stage
# The --chown here ensures the binary itself is owned by appuser
COPY --from=builder --chown=appuser:appgroup /app/calculation_service .

# Copy any other necessary assets (e.g., itemFilesDir if they are static files)
# If 'dependencies/items' are static files needed at runtime:
# COPY --chown=appuser:appgroup dependencies/items ./dependencies/items
# Ensure these subdirectories also have correct permissions if appuser needs to write to them.
# If they are created by COPY, they will be owned by root initially unless --chown is used.
# If you create them with RUN mkdir, you might need another chown.

# Switch to the non-root user
USER appuser

EXPOSE 8081
CMD ["./calculation_service"]