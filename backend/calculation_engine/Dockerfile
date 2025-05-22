### calculation_engine/Dockerfile

# ─── Builder Stage ─────────────────────────────────────────────────
FROM golang:1.23-alpine AS builder
WORKDIR /app

# Download Go modules
COPY go.mod go.sum ./
RUN go mod download

# Copy source & build
COPY . .
RUN go build -o calculation_service .


# ─── Runtime Stage ─────────────────────────────────────────────────
FROM debian:bookworm-slim

# Enable AMD64 architecture for MEGAcmd on ARM64
RUN dpkg --add-architecture amd64 \
 && rm -rf /var/lib/apt/lists/*

# Single pass: update, install deps, add MEGA repo, install megacmd, clean
RUN apt-get update --fix-missing \
 && apt-get install -y --no-install-recommends \
      curl \
      apt-transport-https \
      ca-certificates \
      gnupg \
      dirmngr \
 && mkdir -p /etc/apt/keyrings \
 && curl -fsSL https://mega.nz/keys/MEGA_signing.key \
      | gpg --dearmor -o /etc/apt/keyrings/mega.gpg \
 && echo "deb [arch=amd64 signed-by=/etc/apt/keyrings/mega.gpg] https://mega.nz/linux/repo/Debian_12/ ./" \
      > /etc/apt/sources.list.d/mega.list \
 # Refresh indexes including new arch & repo
 && apt-get update --fix-missing \
 && apt-get install -y --no-install-recommends megacmd:amd64 \
 # Cleanup
 && rm -rf /var/lib/apt/lists/*

# Copy the Go binary from builder
WORKDIR /app
COPY --from=builder /app/calculation_service .

# Expose port and default command
EXPOSE 8081
CMD ["./calculation_service"]

