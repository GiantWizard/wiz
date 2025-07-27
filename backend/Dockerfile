# STAGE 1: Rust/C++ Builder for Metrics Generator and Export Engine
# This stage compiles the data producer and its uploader.
FROM rust:1.80-slim-bookworm AS rust-cpp-builder
LABEL stage=rust-cpp-builder

# Install C++ compiler and build tools
RUN apt-get update && \
    apt-get install -y --no-install-recommends g++ make && \
    rm -rf /var/lib/apt/lists/*

# Build the C++ export engine first
WORKDIR /app/export_engine_build
COPY server9/export.cpp .
RUN g++ -std=c++11 -O2 -Wall -o /app/export_engine export.cpp

# Build the Rust metrics generator
WORKDIR /app/metrics_generator
COPY server9/Cargo.toml server9/Cargo.lock* ./
# Create a dummy src to fetch dependencies without the full source
RUN mkdir -p src && echo "fn main(){}" > src/main.rs
RUN cargo fetch
# Now copy the actual source and build the release binary
COPY server9/src ./src
# The name 'timestamp_generator' comes from your Cargo.toml
RUN cargo build --release && mv /app/metrics_generator/target/release/timestamp_generator /app/metrics_generator_binary


# STAGE 2: Go Builder for Calculation Engine
# This stage compiles the data consumer.
FROM golang:1.23-alpine AS go-builder
LABEL stage=go-builder

WORKDIR /src
COPY calculation_engine/go.mod calculation_engine/go.sum ./
RUN go mod download
RUN go mod verify

COPY calculation_engine/main.go .
RUN CGO_ENABLED=0 go build -o /app/calculation_engine .


# STAGE 3: Final Runtime Image
# This stage assembles the final, lean image from the build artifacts.
FROM debian:11-slim

# Install only the necessary runtime dependencies (Supervisor and MEGA CMD).
# Using the local vendor method for MEGA CMD is more reliable than downloading during build.
RUN apt-get update && apt-get install -y --no-install-recommends \
    supervisor \
    wget \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY vendor/megacmd_*.deb /tmp/megacmd.deb
RUN apt-get update && apt-get install -y --no-install-recommends /tmp/megacmd.deb && \
    rm /tmp/megacmd.deb && \
    rm -rf /var/lib/apt/lists/*

# Create a non-root user for security
RUN addgroup --system appgroup && adduser --system --ingroup appgroup appuser
ENV HOME=/home/appuser
WORKDIR /app

# --- Copy all compiled binaries from the previous stages ---
COPY --from=go-builder --chown=appuser:appgroup /app/calculation_engine /app/calculation_engine
COPY --from=rust-cpp-builder --chown=appuser:appgroup /app/metrics_generator_binary /app/metrics_generator
COPY --from=rust-cpp-builder --chown=appuser:appgroup /app/export_engine /app/export_engine

# Copy the supervisor config and the session manager script
COPY supervisord.conf /etc/supervisor/conf.d/supervisord.conf
COPY scripts/mega-session-manager.sh /usr/local/bin/mega-session-manager.sh

# Make all binaries and scripts executable
RUN chmod +x /app/calculation_engine \
    && chmod +x /app/metrics_generator \
    && chmod +x /app/export_engine \
    && chmod +x /usr/local/bin/mega-session-manager.sh

# Create required directories and set ownership for the non-root user
RUN mkdir -p /var/log/supervisor /home/appuser/.megaCmd /app/metrics \
    && chown -R appuser:appgroup /home/appuser /app /var/log/supervisor

# Expose the port for the Go web server
EXPOSE 8080

# The final command to start the process manager, which runs everything else.
CMD ["/usr/bin/supervisord", "-c", "/etc/supervisor/conf.d/supervisord.conf"]
