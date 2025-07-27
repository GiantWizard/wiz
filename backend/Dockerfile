# STAGE 1: Rust/C++ Builder for Metrics Generator and Export Engine
# This stage compiles the data producer and its uploader.
FROM rust:1.87.0-slim-bookworm AS rust-cpp-builder
LABEL stage=rust-cpp-builder

# Install C++ compiler and ALL required build dependencies for Rust crates
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        g++ \
        make \
        pkg-config \
        libssl-dev && \
    rm -rf /var/lib/apt/lists/*

# Build the C++ export engine first
WORKDIR /app/export_engine_build
COPY server9/export.cpp .
RUN g++ -std=c++11 -O2 -Wall -o /app/export_engine export.cpp

# Build the Rust metrics generator
WORKDIR /app/metrics_generator
COPY server9/Cargo.toml server9/Cargo.lock* ./
RUN mkdir -p src && echo "fn main(){}" > src/main.rs
RUN cargo fetch
COPY server9/src ./src
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
#
# ---- CHANGE #1: Use the same OS version as the builder ----
FROM debian:bookworm-slim AS final
# -----------------------------------------------------------

#
# ---- CHANGE #2: Install the required libssl3 runtime library ----
RUN apt-get update && apt-get install -y --no-install-recommends \
    supervisor \
    wget \
    ca-certificates \
    libssl3 && \
    rm -rf /var/lib/apt/lists/*
# -----------------------------------------------------------------

# Use the local vendor method for MEGA CMD
COPY vendor/megacmd_*.deb /tmp/megacmd.deb
# --- Note: Ensure your megacmd.deb is compatible with Debian 12 (Bookworm) ---
RUN apt-get update && apt-get install -y --no-install-recommends /tmp/megacmd.deb && \
    rm /tmp/megacmd.deb && \
    rm -rf /var/lib/apt/lists/*

# Create a non-root user for security. Supervisor will handle user switching.
RUN addgroup --system appgroup && adduser --system --ingroup appgroup appuser
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

# Create required directories and set ownership.
RUN mkdir -p /var/log/supervisor /home/appuser/.megaCmd /app/metrics \
    && chown -R appuser:appgroup /home/appuser /app \
    && chown -R root:root /var/log/supervisor

# Expose the port for the Go web server
EXPOSE 8080

# ---- CHANGE #3: Remove USER instruction. Let supervisord manage users. ----
# USER appuser
# -------------------------------------------------------------------------

# The final command to start the process manager, which runs everything else as root,
# but launches the individual programs as 'appuser' as defined in supervisord.conf.
CMD ["/usr/bin/supervisord", "-c", "/etc/supervisor/conf.d/supervisord.conf", "-n"]
