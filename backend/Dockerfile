# ---- Build Stage ----
# This stage compiles your Go application. It remains unchanged.
FROM golang:1.23-alpine AS builder
WORKDIR /src

COPY calculation_engine/go.mod calculation_engine/go.sum ./
RUN go mod download

COPY calculation_engine/main.go .
RUN CGO_ENABLED=0 go build -o /app/calculation_engine .


# ---- Final Stage ----
# This stage creates the lean, final image using Debian for stability.
FROM debian:11-slim

# --- RELIABLE METHOD TO INSTALL MEGACMD ON DEBIAN ---
# 1. Install prerequisites using Debian's apt package manager.
#    --no-install-recommends keeps the image lean.
RUN apt-get update && apt-get install -y --no-install-recommends \
    supervisor \
    wget \
    ca-certificates \
    gpg \
    && rm -rf /var/lib/apt/lists/*

# 2. Download MEGA's public GPG key, de-armor it, and store it in the standard keyring location.
#    This is the modern, secure way to add repository keys.
RUN wget -qO- https://mega.nz/linux/repo/repo_mega.asc | gpg --dearmor > /usr/share/keyrings/mega-archive-keyring.gpg

# 3. Add the official MEGA package repository for Debian 11 (Bullseye) to the sources list.
#    This tells apt where to find the 'megacmd' package.
RUN echo "deb [signed-by=/usr/share/keyrings/mega-archive-keyring.gpg] https://mega.nz/linux/repo/Debian_11/ ./" > /etc/apt/sources.list.d/mega.list

# 4. Update the package index again and install megacmd.
RUN apt-get update && apt-get install -y --no-install-recommends \
    megacmd \
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