# ---- Build Stage ----
# This stage compiles your Go application. It remains unchanged.
FROM golang:1.23-alpine AS builder
WORKDIR /src

COPY calculation_engine/go.mod calculation_engine/go.sum ./
RUN go mod download

COPY calculation_engine/main.go .
# Build the Go application, creating a static binary
RUN CGO_ENABLED=0 go build -o /app/calculation_engine .


# ---- Final Stage ----
# This stage creates the lean, final image using Debian for stability.
FROM debian:11-slim

# --- NEW, 100% RELIABLE METHOD ---
# 1. Install prerequisites using Debian's apt package manager.
#    --no-install-recommends keeps the image lean.
RUN apt-get update && apt-get install -y --no-install-recommends \
    supervisor \
    wget \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# 2. Copy the megacmd .deb installer from your local 'vendor' directory into the image.
#    This REMOVES the dependency on MEGA's servers during the build.
COPY vendor/megacmd_*.deb /tmp/megacmd.deb

# 3. Install the package using the local file.
#    apt will automatically handle installing any needed dependencies for the package.
RUN apt-get update && apt-get install -y --no-install-recommends \
    /tmp/megacmd.deb \
    && rm /tmp/megacmd.deb \
    && rm -rf /var/lib/apt/lists/*
# --- END OF RELIABLE METHOD ---


# Create a non-root user for security (Debian syntax).
RUN addgroup --system appgroup && adduser --system --ingroup appgroup appuser
ENV HOME=/home/appuser
WORKDIR /app

# Copy the compiled application binary from the builder stage.
COPY --from=builder /app/calculation_engine /app/calculation_engine

# Copy your other server assets.
COPY server9/ /app/server9/

# Copy the supervisor config and session manager script.
COPY supervisord.conf /etc/supervisor/conf.d/supervisord.conf
COPY scripts/mega-session-manager.sh /usr/local/bin/mega-session-manager.sh

# Make binaries and scripts executable.
RUN chmod +x /app/calculation_engine \
    && chmod +x /usr/local/bin/mega-session-manager.sh

# Create required directories and set permissions.
RUN mkdir -p /var/log/supervisor /home/appuser/.megaCmd \
    && chown -R appuser:appgroup /home/appuser /app /var/log/supervisor

# Expose the port the web server will run on.
EXPOSE 8080

# Switch to the non-root user.
USER appuser

# The command to run when the container starts.
CMD ["/usr/bin/supervisord", "-c", "/etc/supervisor/conf.d/supervisord.conf"]