# ---- Build Stage ----
# Use an official Go image to build the application
FROM golang:1.22-alpine AS builder

WORKDIR /src

# Copy Go source files
COPY main.go go.mod go.sum ./

# Build the Go application binary
# CGO_ENABLED=0 is important for creating a static binary on Alpine
# -o /app/go-app specifies the output path and name for the binary
RUN CGO_ENABLED=0 go build -o /app/go-app .

# ---- Final Stage ----
# Use a lean base image for the final container
FROM alpine:3.18

# Install necessary dependencies: supervisor for process management,
# and megacmd for interacting with MEGA.
RUN apk add --no-cache supervisor megacmd

# Create the application directory and a non-root user for security
RUN addgroup -S appgroup && adduser -S appuser -G appgroup
ENV HOME=/home/appuser
WORKDIR /app

# Copy the built Go binary from the builder stage
COPY --from=builder /app/go-app /app/go-app

# Copy all configuration and script files into the container
COPY supervisord.conf /etc/supervisor/conf.d/supervisord.conf
COPY mega-session-manager.sh /usr/local/bin/mega-session-manager.sh
# Add any other application binaries you have
COPY calculation_service /app/calculation_service
COPY metrics_generator /app/metrics_generator

# Make scripts and application binaries executable
RUN chmod +x /usr/local/bin/mega-session-manager.sh \
    && chmod +x /app/go-app \
    && chmod +x /app/calculation_service \
    && chmod +x /app/metrics_generator

# Create log directories for supervisor
RUN mkdir -p /var/log/supervisor
# Create the .megaCmd directory and give the appuser ownership
RUN mkdir -p /home/appuser/.megaCmd && chown -R appuser:appgroup /home/appuser /app

# Switch to the non-root user
USER appuser

# Define the command to run when the container starts.
# supervisord will read the config file and manage all our processes.
CMD ["/usr/bin/supervisord", "-c", "/etc/supervisor/conf.d/supervisord.conf"]