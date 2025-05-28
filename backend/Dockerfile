# STAGE 1: Go Builder for Calculation Engine
FROM golang:1.23-alpine AS go-builder
LABEL stage=go-builder
WORKDIR /app/calculation_engine

# Copy only necessary files for Go module download first
# Assuming these files are in calculation_engine/ relative to Dockerfile context
COPY calculation_engine/go.mod calculation_engine/go.sum ./
RUN go mod download && go mod verify

# Copy the rest of the Go source code
COPY calculation_engine/ ./

# Build the Go application
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -trimpath -o /app/calculation_service .

# STAGE 2: Rust/C++ Builder for Metrics Generator and Export Engine
FROM rust:1.78-slim-bookworm AS rust-cpp-builder
LABEL stage=rust-cpp-builder

# Install C++ compiler, make, curl, ca-certificates, pkg-config, libssl-dev (for Rust reqwest)
RUN apt-get update && apt-get install -y --no-install-recommends \
    g++ make curl ca-certificates pkg-config libssl-dev \
    && rm -rf /var/lib/apt/lists/*

# Build Rust metrics_generator
# Assuming these files are in server9/ relative to Dockerfile context
WORKDIR /app/metrics_generator
COPY server9/Cargo.toml server9/Cargo.lock* ./
RUN mkdir -p src && echo "fn main(){println!(\"Dummy main for cargo fetch\");}" > src/main.rs
RUN cargo fetch
COPY server9/src ./src
RUN cargo build --release

# Build C++ export_engine
WORKDIR /app/export_engine_build
COPY server9/export.cpp .
RUN g++ -std=c++11 -O2 -Wall -o /app/export_engine export.cpp

# Install megacmd in this builder stage so we can copy its binaries easily
RUN curl -fsSL https://mega.nz/linux/repo/Debian_12/amd64/megacmd-Debian_12_amd64.deb -o /tmp/megacmd.deb \
    && apt-get update \
    && apt-get install -y --no-install-recommends /tmp/megacmd.deb \
    && rm /tmp/megacmd.deb \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# STAGE 3: Final Runtime Image
FROM debian:bookworm-slim AS final

# Install runtime dependencies: supervisor, ca-certificates, libfuse2 (often a mega dep), libssl3
RUN apt-get update && apt-get install -y --no-install-recommends \
    supervisor ca-certificates libfuse2 libssl3 \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user and group
RUN groupadd --gid 999 appgroup && useradd --uid 999 --gid appgroup --shell /bin/bash --create-home appuser

# Create directories and set permissions
RUN mkdir -p /app /data/metrics_archive /var/log/supervisor \
    && chown -R appuser:appgroup /app /data \
    && chown -R root:root /var/log/supervisor # Supervisor (as root) will write its main logs here

WORKDIR /app

# Copy compiled Go binary and its dependencies
COPY --from=go-builder --chown=appuser:appgroup /app/calculation_service /app/calculation_service
COPY --chown=appuser:appgroup calculation_engine/dependencies /app/dependencies

# Copy compiled Rust binary
COPY --from=rust-cpp-builder --chown=appuser:appgroup /app/metrics_generator/target/release/timestamp_generator /app/metrics_generator
# Copy compiled C++ binary
COPY --from=rust-cpp-builder --chown=appuser:appgroup /app/export_engine /app/export_engine

# Copy megacmd binaries from the rust-cpp-builder stage
COPY --from=rust-cpp-builder --chown=root:root /usr/bin/mega-* /usr/local/bin/
RUN chmod +x /usr/local/bin/mega-*

# Copy supervisor configuration file
# This assumes 'supervisor.conf' (the INI file) is at the root of your Docker build context
# (e.g., next to this Dockerfile)
COPY supervisor.conf /etc/supervisor/supervisord.conf

EXPOSE 8081

# USER appuser # <<-- COMMENTED OUT/REMOVED: supervisord will start as root.
              # Individual programs are run as 'appuser' via supervisor.conf.

# CMD for supervisord
CMD ["/usr/bin/supervisord", "-c", "/etc/supervisor/supervisord.conf", "-n"]