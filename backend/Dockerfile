# ---- Build Stage ----
# This stage compiles your Go application
FROM golang:1.23-alpine AS builder
# Set the working directory for the build
WORKDIR /src

# Copy the Go module files first to leverage Docker's layer caching
COPY calculation_engine/go.mod calculation_engine/go.sum ./
RUN go mod download

# Copy the rest of your application's source code
COPY calculation_engine/main.go .

# Build the Go application, creating a static binary
RUN CGO_ENABLED=0 go build -o /app/calculation_engine .

# ---- Final Stage ----
# This stage creates the lean, final image
FROM alpine:3.18

# --- NEW, RELIABLE METHOD TO INSTALL MEGACMD ---
# 1. Install prerequisites. We need wget and ca-certificates to fetch the key.
RUN apk add --no-cache supervisor wget ca-certificates

# 2. Fetch MEGA's public GPG key, which is used to sign their packages, and add it to apk's trusted keys.
RUN wget https://mega.nz/linux/repo/MEGAsync/repo_mega.asc -O /etc/apk/keys/repo_mega.asc

# 3. Add the official MEGA package repository to Alpine's list of sources.
#    The '!testing' tag is an important part of their repository definition.
RUN echo "https://mega.nz/linux/repo/Alpine_3.18 !testing" >> /etc/apk/repositories

# 4. Update the package index to fetch the list of packages from the new repository,
#    then install megacmd. apk will now know where to find it.
RUN apk update && apk add megacmd
# --- END OF NEW METHOD ---

# Create a non-root user for security
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
ENV HOME=/home/appuser
WORKDIR /app

# Copy the compiled application binary from the builder stage
COPY --from=builder /app/calculation_engine /app/calculation_engine

# Copy your other server assets
COPY server9/ /app/server9/

# Copy the new supervisor config and the reliable session manager script
COPY supervisord.conf /etc/supervisor/conf.d/supervisord.conf
COPY scripts/mega-session-manager.sh /usr/local/bin/mega-session-manager.sh

# Make binaries and scripts executable
RUN chmod +x /app/calculation_engine \
    && chmod +x /usr/local/bin/mega-session-manager.sh

# Create required directories and set permissions
RUN mkdir -p /var/log/supervisor /home/appuser/.megaCmd \
    && chown -R appuser:appgroup /home/appuser /app /var/log/supervisor

# Expose the port the web server will run on
EXPOSE 8080

# Switch to the non-root user
USER appuser

# The command to run when the container starts. Supervisor will manage all our processes.
CMD ["/usr/bin/supervisord", "-c", "/etc/supervisor/conf.d/supervisord.conf"]